package nativehost

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Remove only manifests whose complete identity still points to this helper,
// including browsers that have since been removed from the machine.
func Unregister(executable string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return err
	}
	return unregisterTargets(executable, platformRegistrationTargets(home), unregisterPlatformHost)
}

func unregisterTargets(executable string, targets []registrationTarget, detach func(registrationTarget, string) error) error {
	var failures []error
	for _, target := range targets {
		file := filepath.Join(target.directory, Name+".json")
		info, err := os.Lstat(file)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if !info.Mode().IsRegular() || info.Size() > 64<<10 {
			failures = append(failures, fmt.Errorf("unsafe manifest: %s", file))
			continue
		}
		if err := registrationParentsSafe(target.directory); err != nil {
			failures = append(failures, err)
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if !manifestMatches(data, manifestFor(executable, target)) {
			continue
		}
		if err = detach(target, file); err == nil {
			err = os.Remove(file)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func registrationParentsSafe(directory string) error {
	for {
		info, err := os.Lstat(directory)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe registration directory: %s", directory)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return nil
		}
		directory = parent
	}
}
