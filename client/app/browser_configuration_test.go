package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/knothhe/passpine/client/nativehost"
)

func TestManualBrowserConfigurationPersistsAndIsDesktopOnly(t *testing.T) {
	root := t.TempDir()
	program := filepath.Join(root, "Custom Browser.exe")
	if runtime.GOOS == "darwin" {
		program = filepath.Join(root, "Custom Browser.app", "Contents", "MacOS", "browser")
		if err := os.MkdirAll(filepath.Dir(program), 0700); err != nil {
			t.Fatal(err)
		}
		plist := `<plist version="1.0"><dict><key>CFBundleIdentifier</key><string>test.browser</string><key>CFBundleExecutable</key><string>browser</string></dict></plist>`
		if err := os.WriteFile(filepath.Join(root, "Custom Browser.app", "Contents", "Info.plist"), []byte(plist), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(program, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	path := program
	if runtime.GOOS == "darwin" {
		path = filepath.Join(root, "Custom Browser.app")
	}
	configuration := nativehost.BrowserConfiguration{Name: "本机浏览器", Path: path, Kind: "chromium", Directory: filepath.Join(root, "Profile", "NativeMessagingHosts"), RegistryPrefix: `Software\Passpine Test\Browser`}
	s, err := New(Options{DataDir: filepath.Join(root, "app-data")})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	must(t, s, Request{Action: "browser-save", BrowserConfiguration: &configuration})
	loaded, err := nativehost.LoadBrowserConfigurations(s.directory)
	if err != nil || len(loaded) != 1 || loaded[0].Path != path {
		t.Fatalf("saved configuration: %+v %v", loaded, err)
	}
	for _, request := range []Request{
		{Protocol: 1, Action: "browser-save", BrowserConfiguration: &configuration},
		{Protocol: 1, Action: "browser-remove", ID: loaded[0].ID},
		{Protocol: 1, Action: "status", BrowserConfiguration: &configuration},
	} {
		raw, _ := json.Marshal(request)
		var response Response
		if err := json.Unmarshal(s.handleBrowser(raw), &response); err != nil || response.Status != "error" {
			t.Fatalf("extension could configure browsers: %+v %v", response, err)
		}
	}
	for _, action := range []string{"status", "configure", "list"} {
		raw, _ := json.Marshal(Request{Protocol: 1, Action: action})
		response := s.handleBrowser(raw)
		if strings.Contains(string(response), "browserRegistrations") || strings.Contains(string(response), "browserConfigurationError") || strings.Contains(string(response), "本机浏览器") {
			t.Fatalf("local configuration exposed: %s", response)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(Options{DataDir: filepath.Join(root, "app-data")})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	found := false
	for _, item := range reopened.snapshot().BrowserRegistrations {
		if item.ID == loaded[0].ID {
			found = true
		}
	}
	if !found {
		t.Fatal("restart lost manually configured browser")
	}
	preview := &Service{directory: t.TempDir(), preview: true}
	if _, err := preview.configureBrowser(Request{Action: "browser-save", BrowserConfiguration: &configuration}); err == nil {
		t.Fatal("preview saved configuration")
	}
}
