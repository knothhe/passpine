package update

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const helperFlag = "--passpine-update-helper"
const ackFlag = "--passpine-update-ack"

var startupAcknowledgement string

type installPlan struct {
	CurrentVersion, FeedURL, PublicKey, DataDir, Executable, InstallRoot string
	ProtectedPaths                                                       []string
	ParentPID                                                            int
	Arguments                                                            []string
	Envelope                                                             json.RawMessage
}

// Install validates both the signed archive and every staged file again, then
// starts an isolated helper. A nil result means the caller may close its UI and
// service normally. The helper waits for this process to exit before changing
// any installed files; calling Download or Check can never install a release.
func (m *Manager) Install(ctx context.Context) error {
	if !m.begin() {
		return ErrBusy
	}
	defer m.end()
	if m.readyDir == "" || m.manifest == nil {
		return ErrUnavailable
	}
	m.publish(func(s *Snapshot) { s.State = StateVerifying; s.ErrorCode = ""; s.Progress = 0 })
	if e := ctx.Err(); e != nil {
		return m.fail(ErrorInstall, e)
	}
	if e := m.verifyPending(); e != nil {
		return m.fail(ErrorVerification, e)
	}
	if e := m.validateInstallRoot(); e != nil {
		return m.fail(ErrorInstall, e)
	}
	plan := installPlan{CurrentVersion: m.cfg.CurrentVersion, FeedURL: m.cfg.FeedURL, PublicKey: m.cfg.PublicKey, DataDir: m.cfg.DataDir, Executable: m.cfg.Executable, InstallRoot: m.cfg.InstallRoot, ProtectedPaths: m.protectedPaths(), ParentPID: os.Getpid(), Arguments: append([]string(nil), os.Args[1:]...), Envelope: m.envelope}
	data, e := json.Marshal(plan)
	if e != nil {
		return m.fail(ErrorInstall, e)
	}
	token := make([]byte, 12)
	if _, e = rand.Read(token); e != nil {
		return m.fail(ErrorInstall, e)
	}
	id := hex.EncodeToString(token)
	planName := filepath.Join(m.cacheDir(), "install-"+id+".json")
	if e = writeAtomic(planName, data, 0o600); e != nil {
		return m.fail(ErrorStorage, e)
	}
	helper := filepath.Join(m.cacheDir(), "helper-"+id)
	if runtime.GOOS == "windows" {
		helper += ".exe"
	}
	if e = copyFile(m.cfg.Executable, helper, 0o700); e != nil {
		os.Remove(planName)
		return m.fail(ErrorStorage, e)
	}
	command := exec.Command(helper, helperFlag, planName)
	detachHelper(command)
	if e = command.Start(); e != nil {
		os.Remove(planName)
		os.Remove(helper)
		return m.fail(ErrorInstall, e)
	}
	_ = command.Process.Release()
	m.publish(func(s *Snapshot) { s.State = StateInstalling; s.Progress = 1; s.ErrorCode = "" })
	return nil
}
func (m *Manager) verifyPending() error {
	manifest, asset, e := m.parseEnvelope(m.envelope)
	if e != nil {
		return e
	}
	if manifest.Version != m.manifest.Version {
		return errors.New("pending release changed")
	}
	if e = verifyFile(m.readyArchive, asset.Full.SHA256, asset.Full.Size); e != nil {
		return e
	}
	return verifyTree(m.readyDir, asset.Files)
}
func (m *Manager) validateInstallRoot() error {
	if m.cfg.OS == "darwin" && filepath.Ext(m.cfg.InstallRoot) != ".app" {
		return errors.New("macOS update requires an installed application bundle")
	}
	if m.cfg.Executable != filepath.Join(m.cfg.InstallRoot, filepath.FromSlash(mainExecutable(m.cfg.OS))) {
		return errors.New("update executable is outside the expected installation layout")
	}
	st, e := os.Lstat(m.cfg.InstallRoot)
	if e != nil {
		return e
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return errors.New("installation root is not a real directory")
	}
	if protects(m.cfg.InstallRoot, m.cfg.DataDir) {
		return errors.New("application data is inside installation root")
	}
	for _, p := range m.protectedPaths() {
		absolute, e := filepath.Abs(p)
		if e != nil {
			return e
		}
		if protects(m.cfg.InstallRoot, absolute) {
			return errors.New("protected data is inside installation root")
		}
	}
	return nil
}
func mainExecutable(platform string) string {
	if platform == "darwin" {
		return "Contents/MacOS/passpine"
	}
	if platform == "windows" {
		return "passpine.exe"
	}
	return "passpine"
}

// RunHelper must run before application argument parsing and opening the vault.
// It consumes only private update arguments, and returns true for helper mode.
func RunHelper(trust ...string) bool {
	if len(os.Args) == 3 && os.Args[1] == helperFlag {
		planName := os.Args[2]
		if len(trust) == 2 {
			_ = runHelper(planName, trust[0], trust[1])
		}
		return true
	}
	for i := 1; i+1 < len(os.Args); i++ {
		if os.Args[i] == ackFlag {
			startupAcknowledgement = os.Args[i+1]
			os.Args = append(os.Args[:i], os.Args[i+2:]...)
			break
		}
	}
	return false
}

// ConfirmStartup acknowledges that the restarted application initialized its
// native UI and service. Without an acknowledgement the helper restores the
// previous binaries and relaunches the previous version.
func ConfirmStartup(dataDir string) {
	acknowledgement := startupAcknowledgement
	startupAcknowledgement = ""
	if acknowledgement == "" || dataDir == "" {
		return
	}
	absolute, err := filepath.Abs(dataDir)
	if err != nil {
		return
	}
	cache := filepath.Join(canonicalPath(absolute), "updates")
	acknowledgement = filepath.Join(canonicalPath(filepath.Dir(acknowledgement)), filepath.Base(acknowledgement))
	if filepath.Dir(acknowledgement) != cache || !ackNamePattern.MatchString(filepath.Base(acknowledgement)) {
		return
	}
	st, err := os.Lstat(cache)
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return
	}
	f, err := os.OpenFile(acknowledgement, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write([]byte("ready"))
	_ = f.Sync()
	_ = f.Close()
}

var ackNamePattern = regexp.MustCompile(`^startup-install-[0-9a-f]{24}\.json\.ok$`)
var installNamePattern = regexp.MustCompile(`^install-([0-9a-f]{24})\.json$`)

func runHelper(planName, currentVersion, publicKey string) error {
	data, e := os.ReadFile(planName)
	if e != nil || len(data) > maxManifestSize+128<<10 {
		return errors.New("invalid install plan")
	}
	var plan installPlan
	if json.Unmarshal(data, &plan) != nil {
		return errors.New("invalid install plan")
	}
	if currentVersion == "" || publicKey == "" || plan.CurrentVersion != currentVersion || plan.PublicKey != publicKey {
		return errors.New("install plan differs from the trusted running release")
	}
	m, e := New(Config{CurrentVersion: currentVersion, FeedURL: plan.FeedURL, PublicKey: publicKey, DataDir: plan.DataDir, Executable: plan.Executable, InstallRoot: plan.InstallRoot, ProtectedPaths: plan.ProtectedPaths})
	if e != nil {
		return e
	}
	if !m.Snapshot().Configured {
		return errors.New("unconfigured update helper")
	}
	match := installNamePattern.FindStringSubmatch(filepath.Base(planName))
	if filepath.Dir(planName) != m.cacheDir() || match == nil {
		return errors.New("install plan outside update cache")
	}
	helper := filepath.Join(m.cacheDir(), "helper-"+match[1])
	if runtime.GOOS == "windows" {
		helper += ".exe"
	}
	actual, e := os.Executable()
	if e != nil {
		return e
	}
	actual = canonicalPath(actual)
	if actual != helper && !(runtime.GOOS == "windows" && strings.EqualFold(actual, helper)) {
		return errors.New("installation must run from the isolated update helper")
	}
	if _, e = regularFile(helper); e != nil {
		return e
	}
	if plan.ParentPID <= 0 || plan.ParentPID == os.Getpid() {
		return errors.New("invalid update parent process")
	}
	defer os.Remove(planName)
	defer os.Remove(helper)
	result := func(code string) {
		_ = writeAtomic(filepath.Join(m.cacheDir(), "install-result.json"), []byte(`{"errorCode":`+strconv.Quote(code)+`}`), 0o600)
	}
	manifest, asset, e := m.parseEnvelope(plan.Envelope)
	if e != nil {
		result(ErrorVerification)
		return e
	}
	m.manifest, m.asset, m.envelope = manifest, asset, plan.Envelope
	m.readyDir = filepath.Join(m.cacheDir(), "ready")
	m.readyArchive = filepath.Join(m.cacheDir(), packageName(asset))
	if e = m.validateInstallRoot(); e != nil {
		result(ErrorInstall)
		return e
	}
	deadline := time.Now().Add(2 * time.Minute)
	for processAlive(plan.ParentPID) {
		if time.Now().After(deadline) {
			result(ErrorInstall)
			return errors.New("application did not exit")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if e = m.verifyPending(); e != nil {
		result(ErrorVerification)
		_ = restart(plan.Executable, plan.Arguments)
		return e
	}
	transaction, e := m.replaceInstalled()
	if e != nil {
		result(ErrorInstall)
		_ = restart(plan.Executable, plan.Arguments)
		return e
	}
	ack := filepath.Join(m.cacheDir(), "startup-"+filepath.Base(planName)+".ok")
	_ = os.Remove(ack)
	arguments := append(append([]string(nil), plan.Arguments...), ackFlag, ack)
	command := exec.Command(plan.Executable, arguments...)
	detachHelper(command)
	if e = command.Start(); e == nil {
		deadline = time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			if st, se := os.Stat(ack); se == nil && st.Mode().IsRegular() {
				e = nil
				break
			}
			if !processAlive(command.Process.Pid) {
				e = errors.New("updated application exited before initialization")
				break
			}
			e = errors.New("updated application did not initialize")
			time.Sleep(100 * time.Millisecond)
		}
	}
	if e != nil {
		if command.Process != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
		if rollbackErr := transaction.rollback(); rollbackErr != nil {
			result(ErrorInstall)
			return rollbackErr
		}
		result(ErrorInstall)
		_ = restart(plan.Executable, plan.Arguments)
		return e
	}
	_ = command.Process.Release()
	transaction.commit()
	_ = os.Remove(ack)
	_ = os.Remove(filepath.Join(m.cacheDir(), "pending.json"))
	_ = os.Remove(filepath.Join(m.cacheDir(), "install-result.json"))
	return nil
}
func restart(executable string, arguments []string) error {
	command := exec.Command(executable, arguments...)
	detachHelper(command)
	if e := command.Start(); e != nil {
		return e
	}
	return command.Process.Release()
}

type replacement struct {
	original, backup string
	installed        bool
	swapped          bool
}
type installTransaction struct {
	items     []replacement
	temporary string
}

func (t *installTransaction) rollback() error {
	var first error
	for i := len(t.items) - 1; i >= 0; i-- {
		item := t.items[i]
		var e error
		if item.swapped {
			e = swapDirectories(item.backup, item.original)
		} else if item.backup != "" {
			e = replaceFileWithRetry(item.backup, item.original)
		} else if item.installed {
			e = os.Remove(item.original)
		}
		if e != nil && first == nil {
			first = e
		}
	}
	if first == nil {
		_ = os.RemoveAll(t.temporary)
	}
	return first
}
func (t *installTransaction) commit() { _ = os.RemoveAll(t.temporary) }
func replaceFileWithRetry(from, to string) error {
	var e error
	for i := 0; i < 30; i++ {
		if e = replaceFile(from, to); e == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return e
}
func (m *Manager) replaceInstalled() (*installTransaction, error) {
	parent := m.cfg.InstallRoot
	if m.cfg.OS == "darwin" {
		parent = filepath.Dir(parent)
	}
	temp, e := os.MkdirTemp(parent, ".passpine-update-")
	if e != nil {
		return nil, e
	}
	t := &installTransaction{temporary: temp}
	staged := filepath.Join(temp, "new")
	if m.cfg.OS == "darwin" {
		staged = filepath.Join(temp, "new.app")
	}
	if e = os.Mkdir(staged, 0o755); e != nil {
		t.rollback()
		return nil, e
	}
	for _, f := range m.asset.Files {
		source, _ := securePath(m.readyDir, f.Path)
		target, e := securePath(staged, f.Path)
		if e != nil {
			t.rollback()
			return nil, e
		}
		if e = copyFile(source, target, os.FileMode(f.Mode)); e != nil {
			t.rollback()
			return nil, e
		}
	}
	if e = verifyTree(staged, m.asset.Files); e != nil {
		t.rollback()
		return nil, e
	}
	if m.cfg.OS == "darwin" {
		// macOS checks the assembled bundle in addition to the release signature.
		if e = exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", staged).Run(); e != nil {
			t.rollback()
			return nil, e
		}
		original := m.cfg.InstallRoot
		if e = swapDirectories(staged, original); e != nil {
			t.rollback()
			return nil, e
		}
		t.items = append(t.items, replacement{original: original, backup: staged, installed: true, swapped: true})
	} else {
		for i, f := range m.asset.Files {
			original, e := securePath(m.cfg.InstallRoot, f.Path)
			if e != nil {
				t.rollback()
				return nil, e
			}
			backup := ""
			if info, fileErr := regularFile(original); fileErr == nil {
				backup = filepath.Join(temp, "old-"+strconv.Itoa(i))
				if e = os.Link(original, backup); e != nil {
					if info.Size() > maxFileSize {
						t.rollback()
						return nil, errors.New("installed executable is too large to back up")
					}
					if e = copyFile(original, backup, info.Mode().Perm()); e != nil {
						t.rollback()
						return nil, e
					}
				}
			} else if !os.IsNotExist(fileErr) {
				t.rollback()
				return nil, fileErr
			}
			t.items = append(t.items, replacement{original: original, backup: backup})
			if e = replaceFileWithRetry(filepath.Join(staged, filepath.FromSlash(f.Path)), original); e != nil {
				t.rollback()
				return nil, e
			}
			t.items[len(t.items)-1].installed = true
		}
	}
	return t, nil
}
