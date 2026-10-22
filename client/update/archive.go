package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const maxArchiveEntries = 16384
const maxArchiveMetadata = 32 << 20

func packageName(asset *Asset) string {
	switch asset.Format {
	case "tar-gzip":
		return "package.tar.gz"
	case "nsis":
		return "package.exe"
	default:
		return "package.zip"
	}
}

// Extraction only produces the signed application tree. Platform installers'
// scripts, registration and user-data handling are deliberately never used.
func (m *Manager) extract(ctx context.Context, archive, stage string) error {
	switch m.asset.Format {
	case "": // Previously published schema-1 update ZIPs remain readable.
		return m.extractZIP(ctx, archive, stage, false)
	case "app-zip":
		return m.extractZIP(ctx, archive, stage, true)
	case "tar-gzip":
		return m.extractTarGzip(ctx, archive, stage)
	case "nsis":
		return extractNSIS(ctx, archive, stage)
	}
	return errors.New("unsupported installer format")
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func writeArchiveFile(ctx context.Context, stage string, file File, in io.Reader) error {
	target, err := securePath(stage, file.Path)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, os.FileMode(file.Mode))
	if err != nil {
		return err
	}
	err = out.Chmod(os.FileMode(file.Mode))
	var n int64
	if err == nil {
		n, err = io.Copy(out, io.LimitReader(contextReader{ctx, in}, file.Size+1))
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil && n != file.Size {
		err = errors.New("archive member size mismatch")
	}
	return err
}

func (m *Manager) archiveFiles() (map[string]File, map[string]bool) {
	files := make(map[string]File, len(m.asset.Files))
	directories := map[string]bool{}
	for _, file := range m.asset.Files {
		files[file.Path] = file
		for directory := path.Dir(file.Path); directory != "."; directory = path.Dir(directory) {
			directories[directory] = true
		}
	}
	return files, directories
}

func (m *Manager) extractedAll(present map[string]bool) error {
	for i, file := range m.asset.Files {
		if !present[file.Path] {
			return errors.New("full package omits signed file")
		}
		m.publish(func(s *Snapshot) { s.Progress = float64(i+1) / float64(len(m.asset.Files)) })
	}
	return nil
}

func (m *Manager) extractZIP(ctx context.Context, archive, stage string, appBundle bool) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	expected, directories := m.archiveFiles()
	seen, present := map[string]bool{}, map[string]bool{}
	if len(reader.File) > maxArchiveEntries {
		return errors.New("too many archive entries")
	}
	for _, entry := range reader.File {
		if err = ctx.Err(); err != nil {
			return err
		}
		name := entry.Name
		isDirectory := entry.FileInfo().IsDir()
		if isDirectory {
			name = strings.TrimSuffix(name, "/")
		}
		folded := strings.ToLower(name)
		if !safePath(name) || seen[folded] || entry.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe or duplicate archive member")
		}
		seen[folded] = true
		if appBundle {
			if name == "Passpine.app" && isDirectory && entry.UncompressedSize64 == 0 {
				continue
			}
			if !strings.HasPrefix(name, "Passpine.app/") {
				return errors.New("archive member is outside the application bundle")
			}
			name = strings.TrimPrefix(name, "Passpine.app/")
		}
		if isDirectory {
			if !directories[name] || entry.UncompressedSize64 != 0 {
				return errors.New("unexpected archive directory")
			}
			continue
		}
		file, ok := expected[name]
		if !ok || present[name] || !entry.Mode().IsRegular() || entry.UncompressedSize64 != uint64(file.Size) {
			return fmt.Errorf("invalid archive member %q", entry.Name)
		}
		present[name] = true
		in, err := entry.Open()
		if err != nil {
			return err
		}
		err = writeArchiveFile(ctx, stage, file, in)
		closeErr := in.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
	}
	return m.extractedAll(present)
}

// Linux packages include installer metadata for manual installation. None of it
// is copied into the staged application tree or executed by automatic updates.
func linuxMetadataLimit(name string) int64 {
	switch name {
	case "install.sh":
		return 1 << 20
	case "version.txt":
		return 1024
	case "app-icon-monochrome.png":
		return 16 << 20
	}
	return 0
}

func (m *Manager) extractTarGzip(ctx context.Context, archive, stage string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed := bufio.NewReader(contextReader{ctx, file})
	gz, err := gzip.NewReader(compressed)
	if err != nil {
		return err
	}
	defer gz.Close()
	gz.Multistream(false)
	// Count tar headers, extended metadata, discarded installer metadata and
	// trailing padding as well as installed files, so none can form a gzip bomb.
	expanded := &io.LimitedReader{R: contextReader{ctx, gz}, N: maxExpandedSize + maxArchiveMetadata + 1}
	reader := tar.NewReader(expanded)
	expected, _ := m.archiveFiles()
	seen, present := map[string]bool{}, map[string]bool{}
	for entries := 0; ; entries++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if entries >= maxArchiveEntries || expanded.N <= 0 {
			return errors.New("too many archive entries or expanded archive too large")
		}
		name := header.Name
		folded := strings.ToLower(name)
		if !safePath(name) || seen[folded] || (header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA) || !header.FileInfo().Mode().IsRegular() {
			return errors.New("unsafe or duplicate tar member")
		}
		seen[folded] = true
		if header.Size < 0 || header.Size > maxFileSize {
			return errors.New("tar member size is out of bounds")
		}
		if file, ok := expected[name]; ok {
			if header.Size != file.Size {
				return errors.New("tar member size differs from signed file")
			}
			if err = writeArchiveFile(ctx, stage, file, reader); err != nil {
				return err
			}
			present[name] = true
		} else {
			limit := linuxMetadataLimit(name)
			if limit == 0 || header.Size > limit {
				return errors.New("unexpected or oversized installer metadata")
			}
			if _, err = io.Copy(io.Discard, reader); err != nil {
				return err
			}
		}
	}
	// tar stops after its end marker. Drain only zero padding, both validating
	// the gzip checksum/trailer and rejecting hidden appended archives/data.
	buffer := make([]byte, 32<<10)
	for {
		n, err := expanded.Read(buffer)
		for _, b := range buffer[:n] {
			if b != 0 {
				return errors.New("unexpected data after tar end marker")
			}
		}
		if expanded.N <= 0 {
			return errors.New("expanded archive too large")
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	if _, err = compressed.Peek(1); err != io.EOF {
		return errors.New("unexpected data after gzip stream")
	}
	return m.extractedAll(present)
}
