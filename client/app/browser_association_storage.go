package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const browserAssociationFile = "browser-associations.json"

func (s *Service) initBrowserAssociations(legacy json.RawMessage) error {
	encoded, err := readPrivate(filepath.Join(s.directory, browserAssociationFile), 2<<20)
	if err == nil {
		var store browserAssociationStore
		if json.Unmarshal(encoded, &store) != nil {
			return errors.New("website association storage is invalid")
		}
		s.prefs.BrowserAssociations = &store
		s.associationPersisted = append([]byte(nil), encoded...)
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	if len(legacy) == 0 || bytes.Equal(bytes.TrimSpace(legacy), []byte("null")) {
		return nil
	}
	var store browserAssociationStore
	if json.Unmarshal(legacy, &store) != nil {
		return errors.New("legacy website association storage is invalid")
	}
	s.prefs.BrowserAssociations = &store
	if err := s.saveBrowserAssociations(); err != nil {
		return fmt.Errorf("cannot migrate website associations: %w", err)
	}
	return nil
}

// The service lock serializes local browser writes and cloud application. This
// independent file is never rewritten by old clients' preference serializers.
func (s *Service) saveBrowserAssociations() error {
	if s.associationStorageInvalid {
		return &associationError{"local_storage"}
	}
	store := s.prefs.BrowserAssociations
	if !validAssociationStore(store) {
		return &associationError{"invalid_document"}
	}
	var encoded []byte
	var err error
	if store != nil {
		encoded, err = json.MarshalIndent(store, "", "  ")
		if err != nil {
			return err
		}
	}
	if bytes.Equal(encoded, s.associationPersisted) {
		return nil
	}
	path := filepath.Join(s.directory, browserAssociationFile)
	previous, err := readPrivate(path, 2<<20)
	if err != nil && !os.IsNotExist(err) {
		return &associationError{"local_storage"}
	}
	if !bytes.Equal(previous, s.associationPersisted) {
		// An unexpected replacement or removal must not turn into a silent reset.
		return &associationError{"local_storage"}
	}
	if store == nil {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return &associationError{"local_storage"}
		}
	} else if err := writePrivate(path, encoded, false); err != nil {
		return &associationError{"local_storage"}
	}
	s.associationPersisted = append([]byte(nil), encoded...)
	return nil
}
