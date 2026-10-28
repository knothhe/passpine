package app

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/knothhe/passpine/client/cloudsync"
)

// ConfigSyncOptions controls this device's synchronization only. These options
// are never included in the encrypted preferences document.
type ConfigSyncOptions struct {
	Mode            string             `json:"mode"`
	IntervalMinutes int                `json:"intervalMinutes"`
	SyncOnChange    bool               `json:"syncOnChange"`
	SyncOnStart     bool               `json:"syncOnStart"`
	NotifyErrors    bool               `json:"notifyErrors"`
	Desktop         bool               `json:"desktop"`
	Browser         bool               `json:"browser"`
	DesktopFields   SyncFieldSelection `json:"desktopFields"`
	BrowserFields   SyncFieldSelection `json:"browserFields"`
}

type SyncFieldSelection struct {
	Appearance bool `json:"appearance"`
	FontScale  bool `json:"fontScale"`
	Language   bool `json:"language"`
}

func DefaultConfigSyncOptions() ConfigSyncOptions {
	fields := SyncFieldSelection{Appearance: true, FontScale: true, Language: true}
	return ConfigSyncOptions{Mode: "automatic", IntervalMinutes: 5, SyncOnChange: true, SyncOnStart: true, NotifyErrors: true, Desktop: true, Browser: true, DesktopFields: fields, BrowserFields: fields}
}

// Missing fields in an older local file preserve the behavior from before
// selective synchronization and scheduling modes were introduced.
func (options *ConfigSyncOptions) UnmarshalJSON(data []byte) error {
	type plain ConfigSyncOptions
	decoded := plain(DefaultConfigSyncOptions())
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*options = ConfigSyncOptions(decoded)
	return nil
}

func validConfigSyncOptions(options ConfigSyncOptions) bool {
	if options.Mode != "automatic" && options.Mode != "manual" {
		return false
	}
	switch options.IntervalMinutes {
	case 1, 5, 15, 30, 60:
		return true
	}
	return false
}

// The service mutex must be held after initialization.
func (s *Service) configSyncOptions() ConfigSyncOptions {
	if options := s.configSync.meta.Options; options != nil {
		return *options
	}
	return DefaultConfigSyncOptions()
}

// This action shares the profile lane, but never starts SDK, network or native
// credential work, even when no vault or connection has been configured.
func (s *Service) saveConfigSyncOptions(r Request, source string) Response {
	s.updateRequests.RLock()
	defer s.updateRequests.RUnlock()
	s.configSyncMu.Lock()
	defer s.configSyncMu.Unlock()
	s.mu.Lock()
	var err error
	switch {
	case s.closed || s.updatePaused.Load():
		err = &cloudsync.Error{Code: "cancelled"}
	case s.preview || s.configSync.invalid:
		err = &cloudsync.Error{Code: "unavailable"}
	case r.SyncOptions == nil || !validConfigSyncOptions(*r.SyncOptions):
		err = &cloudsync.Error{Code: "invalid_sync_options"}
	default:
		options := *r.SyncOptions
		previous := s.configSyncOptions()
		next := s.configSync.meta
		next.Options = &options
		if s.saveSyncMetadata(next) != nil {
			err = &cloudsync.Error{Code: "local_storage"}
		} else {
			s.configSync.meta = next
			for _, key := range allConfigSyncKeys() {
				if configSyncKeyEnabled(options, key) && !configSyncKeyEnabled(previous, key) {
					s.configSync.dirty = true
				}
			}
			// A local options write must not clear a conflict, unblock denied
			// credentials or shorten a failed operation's retry delay.
			if s.configSync.status.ErrorCode == "" {
				if previous.IntervalMinutes != options.IntervalMinutes || previous.SyncOnChange && !options.SyncOnChange || previous.Mode != options.Mode {
					last, parseErr := time.Parse(time.RFC3339, next.LastSyncedAt)
					if parseErr != nil {
						last = time.Now()
					}
					s.configSync.next = last.Add(time.Duration(options.IntervalMinutes) * time.Minute)
				}
				if options.Mode == "automatic" && options.SyncOnChange && s.configSync.dirty && s.configSync.status.Ready {
					s.scheduleConfigSyncChange()
				}
			}
		}
	}
	s.logConfigSync(r.Action, source, err)
	out := s.syncResponseLocked(r, err)
	notify := s.onSettingsChange
	s.mu.Unlock()
	if err == nil && notify != nil {
		go notify()
	}
	return out
}

func (s *Service) markConfigSyncDirty() {
	s.configSync.dirty = true
	options := s.configSyncOptions()
	if options.Mode == "automatic" && options.SyncOnChange && s.configSync.status.Ready && s.configSync.status.ErrorCode == "" {
		s.scheduleConfigSyncChange()
	}
}

func (s *Service) scheduleConfigSyncChange() {
	next := time.Now().Add(2 * time.Second)
	if s.configSync.next.After(next) {
		s.configSync.next = next
	}
}

var desktopConfigSyncKeys = []string{"desktop.theme", "desktop.themeStyle", "desktop.fontScale", "desktop.language"}
var browserConfigSyncKeys = []string{"browser.theme", "browser.themeStyle", "browser.fontScale", "browser.language"}

func allConfigSyncKeys() []string {
	return append(append([]string(nil), desktopConfigSyncKeys...), browserConfigSyncKeys...)
}

func configSyncKeyEnabled(options ConfigSyncOptions, key string) bool {
	fields := options.DesktopFields
	if strings.HasPrefix(key, "browser.") {
		if !options.Browser {
			return false
		}
		fields = options.BrowserFields
	} else if strings.HasPrefix(key, "desktop.") {
		if !options.Desktop {
			return false
		}
	} else {
		return true
	}
	switch key[strings.IndexByte(key, '.')+1:] {
	case "theme", "themeStyle":
		return fields.Appearance
	case "fontScale":
		return fields.FontScale
	case "language":
		return fields.Language
	}
	return true
}

func selectedBrowserSyncFields(options ConfigSyncOptions) []string {
	fields := []string{}
	for _, key := range browserConfigSyncKeys {
		if configSyncKeyEnabled(options, key) {
			fields = append(fields, strings.TrimPrefix(key, "browser."))
		}
	}
	return fields
}

func selectBrowserSyncPreferences(preferences *BrowserSyncPreferences, options ConfigSyncOptions) *BrowserSyncPreferences {
	selected := cloneBrowserSync(preferences)
	if selected == nil {
		return nil
	}
	if !configSyncKeyEnabled(options, "browser.theme") {
		selected.Theme, selected.ThemeStyle = nil, nil
	}
	if !configSyncKeyEnabled(options, "browser.fontScale") {
		selected.FontScale = nil
	}
	if !configSyncKeyEnabled(options, "browser.language") {
		selected.Language = nil
	}
	if selected.Theme == nil && selected.ThemeStyle == nil && selected.FontScale == nil && selected.Language == nil {
		return nil
	}
	return selected
}

func browserSyncPatchSelected(p *BrowserSyncPreferences, options ConfigSyncOptions) bool {
	return len(selectedBrowserSyncFields(options)) != 0 &&
		(p.Theme == nil || configSyncKeyEnabled(options, "browser.theme")) &&
		(p.ThemeStyle == nil || configSyncKeyEnabled(options, "browser.themeStyle")) &&
		(p.FontScale == nil || configSyncKeyEnabled(options, "browser.fontScale")) &&
		(p.Language == nil || configSyncKeyEnabled(options, "browser.language"))
}

func configSyncKeyPending(previous syncSecret, key string) bool {
	if strings.HasPrefix(key, "desktop.") && previous.PendingDesktop || strings.HasPrefix(key, "browser.") && previous.PendingBrowser {
		return true
	}
	for _, pending := range previous.PendingKeys {
		if pending == key {
			return true
		}
	}
	return false
}

func configSyncPolicy(previous syncSecret, options ConfigSyncOptions) cloudsync.MergePolicy {
	var policy cloudsync.MergePolicy
	for _, key := range allConfigSyncKeys() {
		if !configSyncKeyEnabled(options, key) {
			policy.IgnoredKeys = append(policy.IgnoredKeys, key)
		} else if configSyncKeyPending(previous, key) {
			policy.InitialKeys = append(policy.InitialKeys, key)
		}
	}
	return policy
}

func advanceConfigSyncBaseline(previous syncSecret, merged cloudsync.Values, options ConfigSyncOptions) syncSecret {
	next := previous
	next.Baseline = copySyncValues(merged)
	next.PendingDesktop, next.PendingBrowser = false, false
	next.PendingKeys = nil
	for _, key := range allConfigSyncKeys() {
		if configSyncKeyEnabled(options, key) {
			continue
		}
		if configSyncKeyPending(previous, key) || previous.Baseline == nil {
			next.PendingKeys = append(next.PendingKeys, key)
		}
		// Excluded values were not applied locally; retain their previous
		// baseline so enabling them later can merge real intervening edits.
		delete(next.Baseline, key)
		if value, ok := previous.Baseline[key]; ok {
			next.Baseline[key] = append([]byte(nil), value...)
		}
	}
	return next
}
