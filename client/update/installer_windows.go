//go:build windows

package update

import (
	"bytes"
	"context"
	"debug/pe"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

const nsisExtractProtocol = "passpine-update-extract-v1-a8e19eb9-3dc3-4ccf-8b58-97546f3051f0"

// extractNSIS invokes only the ordinary installer's dedicated extraction mode.
// Its caller verifies the signed feed and complete executable hash first, then
// verifies every extracted file against the signed manifest before installation.
func extractNSIS(ctx context.Context, archive, stage string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateNSISStage(stage); err != nil {
		return err
	}
	if !filepath.IsAbs(archive) || !strings.EqualFold(filepath.Ext(archive), ".exe") {
		return errors.New("invalid installer executable path")
	}
	info, err := os.Lstat(archive)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("installer executable is not a regular file")
	}
	if err := verifyNSISExtractionProtocol(archive); err != nil {
		return err
	}
	// Extraction never needs an interactive installer or an unbounded process.
	extractContext, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(extractContext, archive, "/S", "/PASSPINE_EXTRACT_DIR="+stage)
	command.Dir = stage
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := command.Run(); err != nil {
		if extractContext.Err() != nil {
			return extractContext.Err()
		}
		return fmt.Errorf("installer extraction failed: %w", err)
	}
	return nil
}

func validateNSISStage(stage string) error {
	// Match the NSIS entry point's deliberately narrow path grammar. In
	// particular, device paths and UNC paths must never reach an installer.
	if len(stage) < 4 || len(utf16.Encode([]rune(stage))) >= 240 || !filepath.IsAbs(stage) || filepath.Clean(stage) != stage || len(filepath.VolumeName(stage)) != 2 || stage[1:3] != ":\\" || !isASCIIWindowsDrive(stage[0]) {
		return errors.New("invalid installer staging path")
	}
	for _, part := range strings.Split(stage[3:], "\\") {
		if part == "" || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.ContainsAny(part, ":/*?<>|\"") {
			return errors.New("invalid installer staging path component")
		}
		for _, char := range part {
			if char < 32 {
				return errors.New("invalid installer staging path character")
			}
		}
	}
	current, err := os.Executable()
	if err != nil {
		return err
	}
	if relative, err := filepath.Rel(filepath.Dir(current), stage); err == nil && relative != ".." && !strings.HasPrefix(relative, "..\\") {
		return errors.New("installer staging path overlaps the current installation")
	}
	for parent := stage; ; parent = filepath.Dir(parent) {
		path, err := windows.UTF16PtrFromString(parent)
		if err != nil {
			return err
		}
		attributes, err := windows.GetFileAttributes(path)
		if err != nil {
			return err
		}
		if attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return errors.New("installer staging path contains a reparse point or non-directory")
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("installer staging directory is not empty")
	}
	return nil
}

func isASCIIWindowsDrive(char byte) bool {
	return char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z'
}

// The installer is already authenticated by the signed feed and complete hash.
// The marker prevents accidental execution of legacy NSIS packages, whose
// default behavior is to ignore unknown flags and proceed with installation.
func verifyNSISExtractionProtocol(archive string) error {
	installer, err := pe.Open(archive)
	if err != nil {
		return errors.New("installer has no supported extraction protocol")
	}
	defer installer.Close()
	resources := installer.Section(".rsrc")
	if resources == nil || resources.Size > 4<<20 {
		return errors.New("installer has no supported extraction protocol")
	}
	data, err := resources.Data()
	if err != nil {
		return err
	}
	marker := make([]byte, 0, 2*(len(nsisExtractProtocol)+1))
	for _, char := range nsisExtractProtocol {
		marker = append(marker, byte(char), 0)
	}
	marker = append(marker, 0, 0)
	if !bytes.Contains(data, marker) {
		return errors.New("installer has no supported extraction protocol")
	}
	return nil
}
