package app

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserMetadataKeepsCustomLabelsAndHidesAutomaticDirectoryPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, test := range []struct {
		name, directory, customName, wantBrowser string
		customized                               bool
	}{
		{"home directory", filepath.Join(home, ".password-store"), "", ".password-store", false},
		{"outside home", filepath.Join(t.TempDir(), "company", "private-vault"), "", "private-vault", false},
		{"explicit label", filepath.Join(home, "private-vault"), "Team / Personal", "Team / Personal", true},
		{"explicit path label", filepath.Join(home, "private-vault"), "~/My custom label", "~/My custom label", true},
		{"legacy custom label", filepath.Join(home, "private-vault"), "Old custom label", "Old custom label", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			label := test.customName
			if label == "" {
				label = DisplayDirectory(test.directory)
			}
			s := &Service{preview: true, prefs: preferences{
				Current: "fixture-vault", Settings: DefaultSettings(),
				Vaults: []VaultInfo{{ID: "fixture-vault", Name: label, Directory: test.directory, NameCustomized: test.customized}},
			}}
			for _, action := range []string{"configure", "status", "list"} {
				request, _ := json.Marshal(Request{Protocol: 1, Action: action})
				encoded := s.handleBrowser(request)
				var response Response
				if err := json.Unmarshal(encoded, &response); err != nil {
					t.Fatal(err)
				}
				if response.Status != "ok" || response.Data.VaultName != test.wantBrowser || response.Data.Directory != "" || len(response.Data.Vaults) != 0 || strings.Contains(string(encoded), test.directory) {
					t.Fatalf("%s exposed a local directory or changed a custom label: %s", action, encoded)
				}
			}
			// Browser output sanitization must not alter either the desktop's
			// snapshot or the persisted preference used to render its label.
			if snapshot := s.snapshot(); snapshot.VaultName != label || snapshot.Directory != test.directory || s.prefs.Vaults[0].Name != label {
				t.Fatalf("browser metadata changed the desktop directory label: %+v", snapshot)
			}
		})
	}
}
