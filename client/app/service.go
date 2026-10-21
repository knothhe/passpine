// Package app owns the one active vault session shared by the desktop and browser.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/knothhe/passpine/client/deviceunlock"
	"github.com/knothhe/passpine/client/nativehost"
	"github.com/knothhe/passpine/client/vault"
)

type preferences struct {
	Current  string      `json:"current"`
	Vaults   []VaultInfo `json:"vaults"`
	Settings Settings    `json:"settings"`
}
type Service struct {
	device           deviceunlock.Provider
	deviceMu         sync.Mutex
	deviceCancel     context.CancelFunc
	onChange         func()
	onGitChange      func()
	entriesVersion   uint64
	cloneCancel      context.CancelFunc
	mu               sync.Mutex
	directory        string
	preview          bool
	prefs            preferences
	active           *vault.Vault
	server           *nativehost.Server
	onOpen           func()
	onLock           func()
	clipboard        func(ClipboardWrite) error
	done             chan struct{}
	closed           bool
	activity         []Activity
	activityPrunedAt time.Time
	browser          browserState
	gitError         string
	gitStatusError   string
	gitData          Data
	gitVault         *vault.Vault
	gitRefreshAt     time.Time
	gitOperation     bool
	gitEpoch         uint64
	lastSync         time.Time
	gitSyncPending   bool
	gitSyncQueued    bool
	gitRetryAt       time.Time
	lastActivity     atomic.Int64
	cancellation     atomic.Uint64
	cancelVault      atomic.Pointer[vault.Vault]
	cachedEntries    []vault.Entry
	cachedAt         time.Time
}

func DefaultSettings() Settings {
	return Settings{Theme: "system", ThemeStyle: ThemeStyleMonochrome, FontScale: DefaultFontScale, Language: "system", IdleMinutes: 1, MaxSessionMinutes: 60, ClipboardSeconds: 30, CloseToTray: true, DockIcon: DockIconShow, TrayClickAction: TrayClickQuickPanel, BrowseWhileLocked: true, DeviceUnlock: true, AutoSync: false, SyncMinutes: 0, RecordActivity: true, ActivityMaxEntries: 1000, ActivityRetentionDays: 7, GlobalShortcut: DefaultGlobalShortcut}
}
func New(o Options) (*Service, error) {
	var err error
	if o.Preview && o.DataDir == "" {
		o.DataDir, err = os.MkdirTemp("", "passpine-preview-")
		if err != nil {
			return nil, err
		}
	}
	if o.DataDir == "" {
		o.DataDir, err = nativehost.DataDir()
		if err != nil {
			return nil, err
		}
	}
	dir, err := filepath.Abs(o.DataDir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if info, e := os.Lstat(dir); e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("unsafe application directory")
	}
	if err = os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	s := &Service{directory: dir, preview: o.Preview, done: make(chan struct{}), prefs: preferences{Vaults: []VaultInfo{}, Settings: DefaultSettings()}}
	initialized := false
	defer func() {
		if !initialized {
			_ = s.Close()
		}
	}()
	if !o.Preview {
		s.device = o.DeviceUnlock
		if s.device == nil {
			s.device = deviceunlock.New()
		}
	}
	p := filepath.Join(dir, "preferences.json")
	if b, e := readPrivate(p, 1<<20); e == nil {
		if e = json.Unmarshal(b, &s.prefs); e != nil {
			return nil, fmt.Errorf("settings are invalid: %w", e)
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}

	s.prefs.Settings.ThemeStyle = NormalizeThemeStyle(s.prefs.Settings.ThemeStyle)
	normalizeLifecycleSettings(&s.prefs.Settings, DefaultSettings())
	normalizeTypographySettings(&s.prefs.Settings, DefaultSettings())
	if e := validateSettings(s.prefs.Settings); e != nil {
		s.prefs.Settings = DefaultSettings()
	}
	shortcut, shortcutErr := NormalizeGlobalShortcut(s.prefs.Settings.GlobalShortcut)
	if shortcutErr != nil {
		shortcut = DefaultGlobalShortcut
	}
	s.prefs.Settings.GlobalShortcut = shortcut
	if s.migrateVaultNames() {
		if err = s.savePreferences(); err != nil {
			return nil, err
		}
	}
	for _, v := range s.prefs.Vaults {
		if v.ID == s.prefs.Current {
			s.active, err = s.newVault(v.Directory)
			if err != nil {
				return nil, err
			}
			break
		}
	}
	if o.Preview && s.active == nil {
		if err = s.createPreview(); err != nil {
			return nil, err
		}
	}
	s.cancelVault.Store(s.active)
	if b, e := readPrivate(filepath.Join(dir, "activity.json"), 32<<20); e == nil {
		var records []Activity
		if json.Unmarshal(b, &records) == nil {
			s.activity = records
		}
	}
	if err = s.pruneActivity(time.Now()); err != nil {
		return nil, err
	}
	// Publishing the endpoint allows browser handlers to run immediately.
	// Complete all session and activity initialization before accepting them.
	if !o.Preview {
		s.server, err = nativehost.Start(dir, s.handleBrowser)
		if err != nil {
			return nil, err
		}
	}
	initialized = true
	go s.watch()
	return s, nil
}
func (s *Service) DataDirectory() string { return s.directory }
func (s *Service) IsLocked() bool {
	current := s.cancelVault.Load()
	return current == nil || !current.Status().Unlocked
}
func (s *Service) Touch() {
	s.lastActivity.Store(time.Now().UnixNano())
	if current := s.cancelVault.Load(); current != nil {
		current.Activity()
	}
}
func (s *Service) SetOnChange(f func())    { s.mu.Lock(); s.onChange = f; s.mu.Unlock() }
func (s *Service) SetOnGitChange(f func()) { s.mu.Lock(); s.onGitChange = f; s.mu.Unlock() }
func (s *Service) SetOnOpen(f func())      { s.mu.Lock(); s.onOpen = f; s.mu.Unlock() }
func (s *Service) SetOnLock(f func())      { s.mu.Lock(); s.onLock = f; s.mu.Unlock() }
func (s *Service) SetClipboard(f func(ClipboardWrite) error) {
	s.mu.Lock()
	s.clipboard = f
	s.mu.Unlock()
}
func (s *Service) Close() error {
	s.cancellation.Add(1)
	s.cancelDeviceAuthentication()
	if current := s.cancelVault.Load(); current != nil {
		current.Close()
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.done)
	if s.cloneCancel != nil {
		s.cloneCancel()
	}
	if s.active != nil {
		s.active.Close()
	}
	server := s.server
	s.mu.Unlock()
	if server != nil {
		return server.Close()
	}
	return nil
}
func (s *Service) lock() {
	if s.active != nil {
		s.active.Lock()
	}
	if s.onLock != nil {
		go s.onLock()
	}
}
func (s *Service) watch() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	wasUnlocked := false
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			if s.closed {
				s.mu.Unlock()
				return
			}
			unlocked := s.active != nil && s.active.Status().Unlocked
			if wasUnlocked && !unlocked && s.onLock != nil {
				go s.onLock()
			}
			wasUnlocked = unlocked
			if time.Since(s.activityPrunedAt) >= time.Minute {
				_ = s.pruneActivity(time.Now())
			}
			s.ensureGitRefreshLocked(false)
			syncNow := !s.gitSyncQueued && s.autoSyncDueLocked(time.Now())
			syncVaultID := s.prefs.Current
			if syncNow {
				s.gitSyncQueued = true
			}
			s.mu.Unlock()
			if syncNow {
				go s.runAutomaticGitSync(syncVaultID)
			}
		}
	}
}
func idFor(dir string) string { h := sha256.Sum256([]byte(dir)); return hex.EncodeToString(h[:16]) }
func (s *Service) newVault(directory string) (*vault.Vault, error) {
	dir, e := filepath.Abs(directory)
	if e != nil {
		return nil, e
	}
	id := idFor(dir)
	return vault.New(vault.Config{Directory: dir, KeyFile: filepath.Join(s.directory, "keys", id, "secret.pgp"), RecoveryDirectory: filepath.Join(s.directory, "keys", id, "recovery"), IdleTimeout: time.Duration(s.prefs.Settings.IdleMinutes) * time.Minute, MaximumSession: time.Duration(s.prefs.Settings.MaxSessionMinutes) * time.Minute})
}
func (s *Service) remember(dir, name string) error {
	id := idFor(dir)
	name = strings.TrimSpace(name)
	found := false
	for i, v := range s.prefs.Vaults {
		if v.ID == id {
			if name != "" {
				s.prefs.Vaults[i].Name = name
				s.prefs.Vaults[i].NameCustomized = true
			}
			found = true
		}
	}
	if !found {
		customized := name != ""
		if name == "" {
			name = DisplayDirectory(dir)
		}
		s.prefs.Vaults = append(s.prefs.Vaults, VaultInfo{ID: id, Name: name, Directory: dir, NameCustomized: customized})
	}
	s.prefs.Current = id
	return s.savePreferences()
}
func (s *Service) savePreferences() error {
	b, e := json.MarshalIndent(s.prefs, "", "  ")
	if e != nil {
		return e
	}
	return writePrivate(filepath.Join(s.directory, "preferences.json"), b, false)
}
func (s *Service) snapshot() Data { return s.snapshotWithEntries(true) }
func (s *Service) snapshotWithEntries(includeEntries bool) Data {
	d := Data{EntriesVersion: s.entriesVersion, Configured: s.active != nil, Locked: true, Preview: s.preview, Vaults: append([]VaultInfo{}, s.prefs.Vaults...), Entries: []Entry{}, Settings: s.prefs.Settings, Capabilities: []string{"rename", "revision", "trash", "generate", "copy", "copy-generated", "otp", "interactive_fetch", "interactive_copy"}, VaultID: s.prefs.Current}
	d.DeviceUnlockAvailable = !s.preview && s.device != nil && s.device.Available()
	d.DeviceUnlockEnrolled = s.deviceEnrolled()
	for _, v := range s.prefs.Vaults {
		if v.ID == s.prefs.Current {
			d.Directory = v.Directory
			d.VaultName = v.Name
		}
	}
	if s.active != nil {
		st := s.active.Status()
		d.Locked = !st.Unlocked
		d.Configured = st.Configured
		if includeEntries && (st.Unlocked || s.prefs.Settings.BrowseWhileLocked) {
			if time.Since(s.cachedAt) > 2*time.Second {
				list, e := s.active.ListMetadata()
				if e == nil {
					if !sameEntries(s.cachedEntries, list) {
						s.entriesVersion++
					}
					s.cachedEntries = list
					s.cachedAt = time.Now()
				}
			}
			for _, v := range s.cachedEntries {
				d.Entries = append(d.Entries, Entry{Path: v.Path, Name: v.Name, Folder: v.Folder})
			}
			sort.SliceStable(d.Entries, func(i, j int) bool {
				return d.Entries[i].Path < d.Entries[j].Path
			})
		}
	}
	s.browserSnapshot(&d)
	s.ensureGitRefreshLocked(false)
	copyGitData(&d, s.gitData)
	d.GitError = strings.TrimSpace(strings.Join([]string{s.gitError, s.gitStatusError}, " "))
	d.EntriesVersion = s.entriesVersion
	return d
}
func (s *Service) Execute(r Request) Response { return s.execute(r, "desktop") }
func (s *Service) execute(r Request, source string) Response {
	return s.executeWithBrowserLease(r, source, nil)
}

// A locked interactive browser operation receives a new response lease only
// after its own authentication succeeds, before reading any credential.
type browserResponseLease struct {
	epoch   uint64
	session func(func()) error
}

func (s *Service) executeWithBrowserLease(r Request, source string, browserLease *browserResponseLease) Response {
	if r.Action == "touch" {
		s.lastActivity.Store(time.Now().UnixNano())
		return Response{RequestID: r.RequestID, Protocol: 1, Status: "ok"}
	}
	// Metadata polling is not user activity. Desktop edits also call Touch,
	// while browser credential actions are recorded here before taking mu.
	switch r.Action {
	case "fetch", "copy", "save", "delete", "draft", "generate", "copy-generated", "recovery", "restore", "purge", "unlock", "lock", "switch", "create", "open", "settings", "export", "verify-backup":
		s.lastActivity.Store(time.Now().UnixNano())
	}
	if r.Action == "clone" {
		return s.executeClone(r)
	}
	if r.Action == "git-status" || r.Action == "git-init" || r.Action == "git-sync" || r.Action == "git-remote" {
		return s.executeGit(r, source)
	}

	if r.Action == "lock" || r.Action == "switch" || r.Action == "create" || r.Action == "open" {
		s.cancellation.Add(1)
		s.cancelDeviceAuthentication()
		if current := s.cancelVault.Load(); current != nil {
			current.Lock()
		}
	}
	epoch := s.cancellation.Load()
	if browserLease != nil && (r.Interactive || r.Action == "copy-generated") {
		epoch = browserLease.epoch
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Response{Protocol: 1, Status: "error", Code: "app_unavailable", Message: "应用已关闭"}
	}
	// Explicit metadata refresh detects external pass/Git edits, including same-name content updates.
	if r.Action == "configure" || r.Action == "list" {
		s.cachedAt = time.Time{}
	}
	// Check session and vault identity before any operation can use a stale selection.
	var err error
	var extra Data
	if r.VaultID != "" && r.VaultID != s.prefs.Current {
		err = errors.New("vault_changed")
	} else if epoch != s.cancellation.Load() {
		err = vault.ErrCancelled
	} else {
		if source == "browser" && r.Interactive && (r.Action == "fetch" || r.Action == "copy") && s.active != nil && !s.active.Status().Unlocked {
			err = s.unlockForBrowser(epoch)
			if err == nil && browserLease != nil {
				browserLease.session = s.ClipboardLease()
			}
		}
		if err == nil {
			extra, err = s.dispatch(r, source, epoch)
		}
	}
	if err == nil && source == "desktop" && r.Passphrase != "" && (r.Action == "create" || r.Action == "open" || r.Action == "unlock") {
		extra.DeviceUnlockNotice = s.enrollDevice(r.Passphrase, epoch)
	}
	switch r.Action {
	case "save", "delete", "restore", "switch", "create", "open", "git-sync", "unlock":
		s.entriesVersion++
		s.cachedAt = time.Time{}
	}
	if epoch != s.cancellation.Load() {
		if s.active != nil {
			s.active.Lock()
		}
		extra = Data{}
		err = vault.ErrCancelled
	}
	out := Response{RequestID: r.RequestID, Protocol: 1, Status: "ok", Data: s.snapshotWithEntries(source != "browser" || r.Action == "configure" || r.Action == "list")}
	if out.Data.Locked && needsUnlock(r.Action) {
		extra = Data{}
		if err == nil {
			err = vault.ErrLocked
		}
	}
	if err == nil && s.active != nil {
		switch r.Action {
		case "fetch", "copy", "save", "delete", "draft", "generate", "recovery", "restore":
			s.active.Activity()
		}
	}
	mergeData(&out.Data, extra)
	if err != nil {
		out.Status = "error"
		out.Code, out.Message = classify(err)
	}
	if r.Action != "configure" && r.Action != "status" && r.Action != "list" && r.Action != "activity" {
		s.log(r.Action, source, out.Status)
	}
	if source == "browser" && err == nil && s.onChange != nil {
		switch r.Action {
		case "save", "delete":
			callback := s.onChange
			go callback()
		}
	}
	// Logging and snapshot I/O may outlast a concurrent lock request. Never
	// return the already-built credential response after its session expires.
	if needsUnlock(r.Action) && (epoch != s.cancellation.Load() || (out.Status == "ok" && (s.active == nil || !s.active.Status().Unlocked))) {
		mergeData(&out.Data, Data{})
		out.Data.Locked = true
		if !s.prefs.Settings.BrowseWhileLocked {
			out.Data.Entries = []Entry{}
		}
		out.Status = "error"
		out.Code, out.Message = classify(vault.ErrCancelled)
	}
	return out
}
func mergeData(out *Data, v Data) {
	out.Entry = v.Entry
	out.Destination = v.Destination
	out.OriginalEntry = v.OriginalEntry
	out.Contents = v.Contents
	out.Revision = v.Revision
	out.Password = v.Password
	out.Username = v.Username
	out.URL = v.URL
	out.Notes = v.Notes
	out.OTP = v.OTP
	out.OTPSeconds = v.OTPSeconds
	out.Recovery = v.Recovery
	out.ID = v.ID
	out.TrashID = v.TrashID
	out.Fingerprint = v.Fingerprint
	out.DeviceUnlockNotice = v.DeviceUnlockNotice
	out.Activity = v.Activity
	if v.Entries != nil {
		out.Entries = v.Entries
	}
}
func dataDocument(d vault.Document) Data {
	out := Data{Entry: d.Path, Contents: d.Raw, Revision: d.Revision, Password: d.Password, Username: d.Username, URL: d.URL, Notes: d.Notes}
	if d.OTP != "" {
		if code, e := vault.TOTP(d.OTP, time.Now()); e == nil {
			out.OTP = code.Code
			out.OTPSeconds = code.Remaining
		}
	}
	return out
}
func (s *Service) requireVault() error {
	if s.active == nil {
		return vault.ErrNotConfigured
	}
	return nil
}
func (s *Service) dispatch(r Request, source string, epoch uint64) (Data, error) {
	switch r.Action {
	case "configure", "status":
		return Data{}, nil
	case "list":
		d := s.snapshot()
		if r.Query != "" {
			filtered := []Entry{}
			for _, entry := range d.Entries {
				if strings.Contains(strings.ToLower(entry.Path), strings.ToLower(r.Query)) {
					filtered = append(filtered, entry)
				}
			}
			d.Entries = filtered
		}
		return d, nil
	case "lock":
		s.lock()
		return Data{}, nil
	case "unlock":
		if source == "browser" {
			if r.Interactive {
				return Data{}, s.unlockForBrowser(epoch)
			}
			if s.onOpen != nil {
				go s.onOpen()
			}
			if s.active == nil {
				return Data{}, vault.ErrNotConfigured
			}
			if !s.active.Status().Unlocked {
				return Data{}, vault.ErrLocked
			}
			return Data{}, nil
		}
		if e := s.requireVault(); e != nil {
			return Data{}, e
		}
		if r.Mode == "device" {
			return Data{}, s.unlockDevice(epoch)
		}
		return Data{}, s.active.Unlock(r.Passphrase)
	case "create", "open":
		return s.setup(r)
	case "switch":
		if s.cloneCancel != nil {
			s.cloneCancel()
		}
		for _, v := range s.prefs.Vaults {
			if v.ID == r.ID {
				next, e := s.newVault(v.Directory)
				if e != nil {
					return Data{}, e
				}
				if s.active != nil {
					s.active.Close()
				}
				s.active = next
				s.cancelVault.Store(next)
				s.prefs.Current = v.ID
				return Data{}, s.savePreferences()
			}
		}
		return Data{}, errors.New("unknown vault")
	case "vault-rename":
		return Data{}, s.renameVault(r.ID, r.Name)
	case "generate":
		o := vault.PasswordOptions{Length: r.Length, Lowercase: r.Lowercase, Uppercase: r.Uppercase, Numbers: r.Numbers, Symbols: r.Symbols, ExcludeAmbiguous: r.ExcludeAmbiguous}
		if o.Length == 0 {
			o = vault.DefaultPasswordOptions()
		}
		password, e := vault.GeneratePassword(o)
		return Data{Password: password}, e
	case "copy-generated":
		if r.Contents == "" || len(r.Contents) > 128 {
			return Data{}, errors.New("invalid generated password")
		}
		if s.clipboard == nil {
			return Data{}, errors.New("clipboard unavailable")
		}
		return Data{}, s.clipboard(ClipboardWrite{
			Text: r.Contents, Seconds: s.prefs.Settings.ClipboardSeconds,
			Sensitive: true, Lease: s.generatedClipboardLease(epoch),
		})
	case "settings":
		if r.Settings == nil {
			return Data{}, nil
		}
		settings := *r.Settings
		normalizeLifecycleSettings(&settings, s.prefs.Settings)
		normalizeTypographySettings(&settings, s.prefs.Settings)
		// Older clients omit this preference; preserve the user's current style.
		if settings.ThemeStyle == "" {
			settings.ThemeStyle = NormalizeThemeStyle(s.prefs.Settings.ThemeStyle)
		}
		shortcut, err := NormalizeGlobalShortcut(settings.GlobalShortcut)
		if err != nil {
			return Data{}, err
		}
		settings.GlobalShortcut = shortcut
		if e := validateSettings(settings); e != nil {
			return Data{}, e
		}
		previous := s.prefs.Settings
		if previous.DeviceUnlock && !settings.DeviceUnlock {
			if e := s.removeDeviceEnvelopes(); e != nil {
				return Data{}, e
			}
		}
		s.prefs.Settings = settings
		if e := s.savePreferences(); e != nil {
			s.prefs.Settings = previous
			return Data{}, e
		}
		if s.active != nil {
			s.active.SetTimeouts(time.Duration(settings.IdleMinutes)*time.Minute, time.Duration(settings.MaxSessionMinutes)*time.Minute)
		}
		return Data{}, s.pruneActivity(time.Now())
	case "browser-install":
		return s.installBrowser(r.ID)
	case "activity":
		if e := s.pruneActivity(time.Now()); e != nil {
			return Data{}, e
		}
		return Data{Activity: append([]Activity{}, s.activity...)}, nil
	}
	if e := s.requireVault(); e != nil {
		return Data{}, e
	}
	switch r.Action {
	case "fetch":
		if r.Kind == "draft" {
			rec, e := s.active.ReadRecovery(r.ID)
			if e != nil {
				return Data{}, e
			}
			d := dataDocument(vault.ParseDocument(rec.Path, rec.Raw, rec.Revision))
			d.ID = rec.ID
			d.Destination = rec.Destination
			d.OriginalEntry = rec.Path
			return d, nil
		}
		doc, e := s.active.Read(r.Entry)
		return dataDocument(doc), e
	case "copy":
		doc, e := s.active.Read(r.Entry)
		if e != nil {
			return Data{}, e
		}
		if r.Revision != "" && r.Revision != doc.Revision {
			return Data{}, vault.ErrConflict
		}
		d := dataDocument(doc)
		var text string
		switch r.Field {
		case "password", "":
			text = d.Password
		case "username":
			text = d.Username
		case "otp":
			text = d.OTP
		case "raw":
			if source == "browser" {
				return Data{}, errors.New("invalid field")
			}
			text = d.Contents
		default:
			return Data{}, errors.New("invalid field")
		}
		if text == "" {
			return Data{}, errors.New("字段为空")
		}
		if s.clipboard == nil {
			return Data{}, errors.New("clipboard unavailable")
		}
		return Data{}, s.clipboard(ClipboardWrite{
			Text: text, Seconds: s.prefs.Settings.ClipboardSeconds,
			Sensitive: r.Field != "username", Lease: s.ClipboardLease(),
		})
	case "save":
		if r.Mode == "create" && r.Revision != "" {
			return Data{}, vault.ErrConflict
		}
		if r.Mode == "edit" && r.Revision == "" {
			return Data{}, vault.ErrConflict
		}
		var doc vault.Document
		var e error
		if r.Destination != "" && r.Destination != r.Entry {
			if r.Revision == "" {
				return Data{}, vault.ErrConflict
			}
			doc, e = s.active.SaveRename(r.Entry, r.Destination, r.Contents, r.Revision)
		} else {
			doc, e = s.active.Save(r.Entry, r.Contents, r.Revision)
		}
		if e == nil {
			if r.ID != "" {
				_ = s.active.RemoveRecovery(r.ID)
			}
			s.autoSync()
		}
		return dataDocument(doc), e
	case "delete":
		rec, e := s.active.Trash(r.Entry, r.Revision)
		if e == nil {
			s.autoSync()
		}
		return Data{TrashID: rec.ID}, e
	case "draft":
		rec, e := s.active.SaveDraftRecord(r.ID, r.Entry, r.Destination, r.Contents, r.Revision)
		return Data{ID: rec.ID}, e
	case "recovery":
		records, e := s.active.Recovery(r.Kind)
		d := Data{Recovery: []RecoveryItem{}}
		for _, rec := range records {
			d.Recovery = append(d.Recovery, RecoveryItem{ID: rec.ID, Kind: rec.Kind, Path: rec.Path, CreatedAt: rec.CreatedAt.Format(time.RFC3339)})
		}
		return d, e
	case "restore":
		destination := r.Destination
		if destination == "" {
			record, e := s.active.ReadRecovery(r.ID)
			if e != nil {
				return Data{}, e
			}
			destination = record.Path
		}
		doc, e := s.active.Restore(r.ID, destination)
		if e == nil {
			s.autoSync()
		}
		return dataDocument(doc), e
	case "purge":
		return Data{}, s.active.RemoveRecovery(r.ID)
	case "export":
		b, e := s.active.ExportSecret()
		if e != nil {
			return Data{}, e
		}
		defer clear(b)
		if r.Destination == "" {
			return Data{}, errors.New("请选择恢复私钥的保存路径")
		}
		return Data{}, writePrivate(r.Destination, b, true)
	case "verify-backup":
		b, e := readPrivate(r.KeyFile, 16<<20)
		if e != nil {
			return Data{}, e
		}
		defer clear(b)
		return Data{}, s.active.VerifyBackup(b, r.Passphrase)
	}
	return Data{}, errors.New("unsupported action")
}
func (s *Service) setup(r Request) (Data, error) {
	if r.Directory == "" {
		return Data{}, errors.New("请选择密码库目录")
	}
	dir, e := filepath.Abs(r.Directory)
	if e != nil {
		return Data{}, e
	}
	next, e := s.newVault(dir)
	if e != nil {
		return Data{}, e
	}
	prior := s.cancelVault.Swap(next)
	defer func() {
		if s.active != next {
			s.cancelVault.Store(prior)
		}
	}()
	var fingerprint string
	if r.Action == "create" {
		identity, e := next.Create(r.Name, r.Email, r.Passphrase)
		if e != nil {
			return Data{}, e
		}
		fingerprint = identity.Fingerprint
	} else {
		if r.Mode == "key-file" || r.KeyFile != "" || r.PublicKeyFile != "" {
			if strings.TrimSpace(r.KeyFile) == "" {
				return Data{}, errors.New("请先选择私钥文件。")
			}
			b, e := readPrivate(r.KeyFile, 16<<20)
			if e != nil {
				return Data{}, errors.New("无法读取私钥文件，请检查文件路径和访问权限。")
			}
			defer clear(b)
			var public []byte
			if r.PublicKeyFile != "" {
				public, e = readPrivate(r.PublicKeyFile, 16<<20)
				if e != nil || len(public) == 0 {
					return Data{}, errors.New("无法读取公钥文件，请检查文件路径和访问权限。")
				}
				defer clear(public)
			}
			ids, e := next.ImportKeyFiles(b, public, r.Passphrase)
			if e != nil {
				return Data{}, e
			}
			for _, identity := range ids {
				if identity.HasSecret {
					fingerprint = identity.Fingerprint
					break
				}
			}
		} else {
			if next.Status().Configured && r.Mode != "system-gpg" {
				if e = next.Unlock(r.Passphrase); e != nil {
					return Data{}, e
				}
			} else {
				if s.preview {
					return Data{}, errors.New("演示模式不读取系统 GPG 私钥")
				}
				ids, e := next.ImportSystemGPG(r.Passphrase)
				if e != nil {
					return Data{}, e
				}
				for _, identity := range ids {
					if identity.HasSecret {
						fingerprint = identity.Fingerprint
						break
					}
				}
			}
		}
	}
	if s.active != nil {
		s.active.Close()
	}
	s.active = next
	s.cancelVault.Store(next)
	if e = s.remember(dir, r.Name); e != nil {
		return Data{}, e
	}
	return Data{Fingerprint: fingerprint}, nil
}
func (s *Service) autoSync() {
	if !s.prefs.Settings.AutoSync {
		return
	}
	s.resetGitStateLocked()
	s.gitSyncPending = true
}
func validateSettings(v Settings) error {
	if !ValidFontScale(v.FontScale) {
		return errors.New("invalid text size")
	}
	if v.DockIcon != DockIconShow && v.DockIcon != DockIconHide && v.DockIcon != DockIconFollowWindow {
		return errors.New("invalid dock icon behavior")
	}
	if v.TrayClickAction != TrayClickQuickPanel && v.TrayClickAction != TrayClickMainWindow {
		return errors.New("invalid tray click action")
	}
	if v.IdleMinutes < 1 || v.IdleMinutes > 240 || v.MaxSessionMinutes < 1 || v.MaxSessionMinutes > 1440 || v.ClipboardSeconds < 5 || v.ClipboardSeconds > 300 || v.SyncMinutes < 0 || v.SyncMinutes > 1440 {
		return errors.New("设置数值超出范围")
	}
	if v.Theme != "system" && v.Theme != "light" && v.Theme != "dark" {
		return errors.New("invalid theme")
	}
	if v.ThemeStyle != ThemeStyleOnevra && v.ThemeStyle != ThemeStyleMonochrome {
		return errors.New("invalid theme style")
	}
	if v.Language != "zh-CN" && v.Language != "en" && v.Language != "system" {
		return errors.New("invalid language")
	}
	if v.ActivityMaxEntries < 1 || v.ActivityMaxEntries > 100000 || v.ActivityRetentionDays < 1 || v.ActivityRetentionDays > 3650 {
		return errors.New("操作记录保留数量或天数超出范围")
	}
	return nil
}
func classify(e error) (string, string) {
	var authentication *browserUnlockError
	if errors.As(e, &authentication) {
		return authentication.code, authentication.message
	}
	switch {
	case errors.Is(e, vault.ErrCancelled):
		return "locked", "会话已失效，请重新解锁"
	case errors.Is(e, vault.ErrBusy):
		return "vault_busy", "密码库正在同步，请稍后重试"
	case errors.Is(e, vault.ErrLocked):
		return "locked", "密码库已锁定，请在客户端解锁"
	case errors.Is(e, vault.ErrNotConfigured):
		return "not_configured", "请先在客户端创建或打开密码库"
	case errors.Is(e, vault.ErrConflict):
		return "revision_conflict", "条目已发生变化，请重新打开后编辑"
	case errors.Is(e, vault.ErrExists):
		return "already_exists", "目标位置已存在条目"
	case errors.Is(e, vault.ErrPath):
		return "invalid_path", "名称或目录不安全"
	case errors.Is(e, vault.ErrPassphrase):
		return "invalid_passphrase", "私钥口令错误或私钥不可用"
	case e.Error() == "vault_changed":
		return "vault_changed", "密码库已切换，请刷新列表"
	default:
		return "operation_failed", e.Error()
	}
}
func readPrivate(path string, max int64) ([]byte, error) {
	info, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() || info.Size() > max {
		return nil, errors.New("invalid file")
	}
	return os.ReadFile(path)
}
func writePrivate(path string, data []byte, noReplace bool) error {
	if !filepath.IsAbs(path) {
		return errors.New("absolute path required")
	}
	if info, e := os.Lstat(path); e == nil {
		if noReplace {
			return os.ErrExist
		}
		if !info.Mode().IsRegular() {
			return errors.New("unsafe file")
		}
	}
	file, e := os.CreateTemp(filepath.Dir(path), ".passpine-*")
	if e != nil {
		return e
	}
	name := file.Name()
	defer os.Remove(name)
	if e = restrictPrivateFile(file); e == nil {
		_, e = file.Write(data)
	}
	if e == nil {
		e = file.Sync()
	}
	ce := file.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if noReplace {
		if e = os.Link(name, path); e != nil {
			return e
		}
		return nil
	}
	return os.Rename(name, path)
}
func (s *Service) handleBrowser(raw json.RawMessage) json.RawMessage {
	lease := browserResponseLease{epoch: s.cancellation.Load()}
	var r Request
	if len(raw) > nativehost.MaxFrame || json.Unmarshal(raw, &r) != nil || r.Protocol != 1 {
		return encode(Response{RequestID: r.RequestID, Protocol: 1, Status: "error", Code: "invalid_request"})
	}
	switch r.Action {
	case "configure", "status", "list", "unlock", "lock", "generate":
	case "touch":
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return encode(Response{RequestID: r.RequestID, Protocol: 1, Status: "error", Code: "invalid_request"})
		}
		for key := range fields {
			if key != "protocol" && key != "action" && key != "requestId" {
				return encode(Response{RequestID: r.RequestID, Protocol: 1, Status: "error", Code: "invalid_request"})
			}
		}
		s.lastActivity.Store(time.Now().UnixNano())
		return encode(map[string]any{"requestId": r.RequestID, "protocol": 1, "status": "ok", "data": map[string]any{}})
	case "copy-generated":
		if !validGeneratedCopyRequest(r) {
			return encode(Response{RequestID: r.RequestID, Protocol: 1, Status: "error", Code: "invalid_request"})
		}
	case "fetch", "copy", "save", "delete":
		if r.VaultID == "" {
			return encode(Response{RequestID: r.RequestID, Protocol: 1, Status: "error", Code: "vault_changed"})
		}
	default:
		return encode(Response{RequestID: r.RequestID, Protocol: 1, Status: "error", Code: "unsupported_action"})
	}
	if r.Kind != "" || r.Directory != "" || r.KeyFile != "" || r.PublicKeyFile != "" || r.Passphrase != "" || r.Settings != nil || r.ID != "" {
		return encode(Response{RequestID: r.RequestID, Protocol: 1, Status: "error", Code: "invalid_request"})
	}
	if r.Interactive && r.Action != "fetch" && r.Action != "copy" && r.Action != "unlock" {
		return encode(Response{RequestID: r.RequestID, Protocol: 1, Status: "error", Code: "invalid_request"})
	}
	if needsUnlock(r.Action) {
		lease.session = s.ClipboardLease()
	}
	out := s.executeWithBrowserLease(r, "browser", &lease)
	out.Data.VaultName = browserVaultName(out.Data)
	out.Data.Directory = ""
	out.Data.Vaults = nil
	out.Data.BrowserRegistrations = nil
	// Git remotes and filesystem errors can contain local checkout paths.
	// The extension does not need the desktop's repository diagnostics.
	copyGitData(&out.Data, Data{})
	out.Data.GitError = ""
	out.RequestID = r.RequestID
	// A browser response is never allowed to include preferences or a local path.
	encoded := encodeBrowserResponse(out, lease.session)
	if r.Action != "configure" && r.Action != "list" {
		var lean map[string]any
		if json.Unmarshal(encoded, &lean) == nil {
			if data, ok := lean["data"].(map[string]any); ok {
				delete(data, "entries")
				delete(data, "vaults")
				delete(data, "directory")
				delete(data, "capabilities")
				delete(data, "settings")
			}
			encoded = encode(lean)
		}
	}
	return encoded
}

func validGeneratedCopyRequest(r Request) bool {
	if r.Contents == "" || len(r.Contents) > 128 {
		return false
	}
	// A generated password has no vault identity or credential selection. Keep
	// this action independent of unlock and reject arguments for other actions.
	r.RequestID, r.Protocol, r.Action, r.Contents = "", 0, "", ""
	return r == (Request{})
}

// Keep response serialization inside the generation that read the credential;
// an intervening lock/unlock must not revive a stale credential.
func encodeBrowserResponse(out Response, lease func(func()) error) json.RawMessage {
	if lease == nil || out.Status != "ok" {
		return encode(out)
	}
	var encoded json.RawMessage
	if err := lease(func() { encoded = encode(out) }); err != nil {
		code, message := classify(err)
		return encode(Response{RequestID: out.RequestID, Protocol: 1, Status: "error", Code: code, Message: message, Data: Data{Locked: true}})
	}
	return encoded
}

func encode(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func (s *Service) log(action, source, outcome string) {
	if !s.prefs.Settings.RecordActivity {
		_ = s.pruneActivity(time.Now())
		return
	}
	item := Activity{Time: time.Now().UTC().Format(time.RFC3339), Action: action, Source: source, Outcome: outcome}
	s.activity = append([]Activity{item}, s.activity...)
	count := len(s.activity)
	if err := s.pruneActivity(time.Now()); err == nil && len(s.activity) == count {
		// Pruning already persists its changes; otherwise save the new record.
		_ = s.saveActivity()
	}
}

func needsUnlock(action string) bool {
	switch action {
	case "fetch", "copy", "save", "delete", "draft", "recovery", "restore", "purge", "export", "verify-backup":
		return true
	}
	return false
}

// ClipboardLease validates the session on the UI thread immediately around the
// native write. It never acquires the service mutex from the UI thread.
func (s *Service) ClipboardLease() func(func()) error {
	current := s.cancelVault.Load()
	epoch := s.cancellation.Load()
	if current == nil {
		return func(func()) error { return vault.ErrLocked }
	}
	generation, e := current.SessionGeneration()
	return func(f func()) error {
		if e != nil {
			return e
		}
		if s.cancellation.Load() != epoch || s.cancelVault.Load() != current {
			return vault.ErrCancelled
		}
		return current.WithSession(generation, func() error { f(); return nil })
	}
}

// Generated passwords do not belong to a vault session. Their native writes
// still expire when the service closes or a cancellation request intervenes.
// Reading the channel and epoch never takes the service mutex on the UI thread.
func (s *Service) generatedClipboardLease(epoch uint64) func(func()) error {
	return func(f func()) error {
		select {
		case <-s.done:
			return vault.ErrCancelled
		default:
		}
		if s.cancellation.Load() != epoch {
			return vault.ErrCancelled
		}
		f()
		return nil
	}
}

func sameEntries(a, b []vault.Entry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Path != b[i].Path || !a[i].Modified.Equal(b[i].Modified) {
			return false
		}
	}
	return true
}
