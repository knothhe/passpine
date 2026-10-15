package app

import (
	"github.com/knothhe/passpine-neo/client/deviceunlock"
	"github.com/knothhe/passpine-neo/client/nativehost"
)

// Request is shared by the native UI and the browser native-messaging boundary.
// The browser boundary separately restricts which actions and arguments it accepts.
type Request struct {
	RequestID        string    `json:"requestId,omitempty"`
	Protocol         int       `json:"protocol,omitempty"`
	Action           string    `json:"action"`
	Interactive      bool      `json:"interactive,omitempty"`
	VaultID          string    `json:"vaultId,omitempty"`
	Entry            string    `json:"entry,omitempty"`
	Contents         string    `json:"contents,omitempty"`
	Revision         string    `json:"revision,omitempty"`
	Destination      string    `json:"destination,omitempty"`
	Mode             string    `json:"mode,omitempty"`
	Directory        string    `json:"directory,omitempty"`
	KeyFile          string    `json:"keyFile,omitempty"`
	Passphrase       string    `json:"passphrase,omitempty"`
	Name             string    `json:"name,omitempty"`
	Email            string    `json:"email,omitempty"`
	Query            string    `json:"query,omitempty"`
	ID               string    `json:"id,omitempty"`
	Kind             string    `json:"kind,omitempty"`
	Field            string    `json:"field,omitempty"`
	URL              string    `json:"url,omitempty"`
	Length           int       `json:"length,omitempty"`
	Lowercase        bool      `json:"lowercase,omitempty"`
	Uppercase        bool      `json:"uppercase,omitempty"`
	Numbers          bool      `json:"numbers,omitempty"`
	Symbols          bool      `json:"symbols,omitempty"`
	ExcludeAmbiguous bool      `json:"excludeAmbiguous,omitempty"`
	Settings         *Settings `json:"settings,omitempty"`
}

type Response struct {
	RequestID string `json:"requestId,omitempty"`
	Protocol  int    `json:"protocol"`
	Status    string `json:"status"`
	Data      Data   `json:"data"`
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
}
type Entry struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Folder string `json:"folder"`
}
type VaultInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Directory      string `json:"directory"`
	NameCustomized bool   `json:"nameCustomized,omitempty"`
}
type RecoveryItem struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	CreatedAt string `json:"createdAt"`
}
type Activity struct {
	Time    string `json:"time"`
	Action  string `json:"action"`
	Source  string `json:"source"`
	Outcome string `json:"outcome"`
}
type Settings struct {
	CloseToTray           bool   `json:"closeToTray"`
	Theme                 string `json:"theme"`
	Language              string `json:"language"`
	IdleMinutes           int    `json:"idleMinutes"`
	MaxSessionMinutes     int    `json:"maxSessionMinutes"`
	ClipboardSeconds      int    `json:"clipboardSeconds"`
	BrowseWhileLocked     bool   `json:"browseWhileLocked"`
	DeviceUnlock          bool   `json:"deviceUnlock"`
	AutoSync              bool   `json:"autoSync"`
	SyncMinutes           int    `json:"syncMinutes"`
	RecordActivity        bool   `json:"recordActivity"`
	ActivityMaxEntries    int    `json:"activityMaxEntries"`
	ActivityRetentionDays int    `json:"activityRetentionDays"`
	GlobalShortcut        string `json:"globalShortcut"`
}
type Data struct {
	EntriesVersion         uint64                           `json:"entriesVersion"`
	Destination            string                           `json:"destination,omitempty"`
	OriginalEntry          string                           `json:"originalEntry,omitempty"`
	Configured             bool                             `json:"configured"`
	Locked                 bool                             `json:"locked"`
	Preview                bool                             `json:"preview"`
	VaultID                string                           `json:"vaultId"`
	VaultName              string                           `json:"vaultName"`
	Directory              string                           `json:"directory,omitempty"`
	Vaults                 []VaultInfo                      `json:"vaults"`
	Entries                []Entry                          `json:"entries"`
	Capabilities           []string                         `json:"capabilities"`
	Settings               Settings                         `json:"settings"`
	Entry                  string                           `json:"entry,omitempty"`
	Contents               string                           `json:"contents,omitempty"`
	Revision               string                           `json:"revision,omitempty"`
	Password               string                           `json:"password,omitempty"`
	Username               string                           `json:"username,omitempty"`
	URL                    string                           `json:"url,omitempty"`
	Notes                  string                           `json:"notes,omitempty"`
	OTP                    string                           `json:"otp,omitempty"`
	OTPSeconds             int                              `json:"otpSeconds,omitempty"`
	Recovery               []RecoveryItem                   `json:"recovery,omitempty"`
	TrashID                string                           `json:"trashId,omitempty"`
	ID                     string                           `json:"id,omitempty"`
	Fingerprint            string                           `json:"fingerprint,omitempty"`
	GitRemote              string                           `json:"gitRemote,omitempty"`
	GitUpstream            string                           `json:"gitUpstream,omitempty"`
	GitBranch              string                           `json:"gitBranch,omitempty"`
	GitChanges             int                              `json:"gitChanges"`
	GitAvailable           bool                             `json:"gitAvailable"`
	GitAhead               int                              `json:"gitAhead"`
	GitBehind              int                              `json:"gitBehind"`
	GitConflict            bool                             `json:"gitConflict"`
	GitSyncing             bool                             `json:"gitSyncing"`
	GitRefreshing          bool                             `json:"gitRefreshing"`
	GitLastResult          string                           `json:"gitLastResult,omitempty"`
	GitLastSync            string                           `json:"gitLastSync,omitempty"`
	GitStatusAt            string                           `json:"gitStatusAt,omitempty"`
	GitSummary             string                           `json:"gitSummary,omitempty"`
	GitError               string                           `json:"gitError,omitempty"`
	GitInitialized         bool                             `json:"gitInitialized"`
	BrowserInstalled       bool                             `json:"browserInstalled"`
	BrowserHelperAvailable bool                             `json:"browserHelperAvailable"`
	BrowserRegistrations   []nativehost.BrowserRegistration `json:"browserRegistrations,omitempty"`
	DeviceUnlockAvailable  bool                             `json:"deviceUnlockAvailable"`
	DeviceUnlockEnrolled   bool                             `json:"deviceUnlockEnrolled"`
	DeviceUnlockNotice     string                           `json:"deviceUnlockNotice,omitempty"`
	Activity               []Activity                       `json:"activity,omitempty"`
}
type Options struct {
	DataDir      string
	Preview      bool
	DeviceUnlock deviceunlock.Provider
}
