package deviceunlock

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"github.com/knothhe/passpine/client/i18n"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/sys/windows"
)

const (
	helperArgument    = "--device-unlock-helper"
	credPackGeneric   = 0x4
	credUIGeneric     = 0x1
	credUICurrentUser = 0x200
)

var (
	credentialUI      = windows.NewLazySystemDLL("credui.dll")
	promptCredentials = credentialUI.NewProc("CredUIPromptForWindowsCredentialsW")
	packCredentials   = credentialUI.NewProc("CredPackAuthenticationBufferW")
	unpackCredentials = credentialUI.NewProc("CredUnPackAuthenticationBufferW")
	logonUser         = windows.NewLazySystemDLL("advapi32.dll").NewProc("LogonUserW")
)

func configureProvider(p *systemProvider, executable string) {
	p.binary, p.args = executable, []string{helperArgument}
}

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

// RunHelper runs before desktop/single-instance initialization. CommandContext
// kills this process on lock, vault switch, shutdown or authentication timeout.
func RunHelper() bool {
	if len(os.Args) != 2 || os.Args[1] != helperArgument {
		return false
	}
	runWindowsHelper(os.Stdin, os.Stdout, authenticateWindows)
	return true
}

type windowsAuthenticator func(string, string) ([]uint16, error)

func runWindowsHelper(input io.Reader, output io.Writer, authenticate windowsAuthenticator) {
	result := response{}
	data, err := io.ReadAll(io.LimitReader(input, (1<<20)+1))
	defer clear(data)
	var req request
	if err == nil && len(data) <= 1<<20 {
		err = json.Unmarshal(data, &req)
	} else if err == nil {
		err = errors.New("request too large")
	}
	defer clear(req.Secret)
	defer clear(req.Envelope)
	if err == nil {
		result, err = handleWindowsRequest(req, authenticate)
	}
	if err != nil {
		clear(result.Secret)
		result = response{Error: "系统验证未完成，请使用私钥口令"}
	}
	defer clear(result.Secret)
	encoded, err := json.Marshal(result)
	if err == nil {
		defer clear(encoded)
		_, _ = output.Write(encoded)
	}
}

func windowsAvailable() bool {
	for _, proc := range []*windows.LazyProc{promptCredentials, packCredentials, unpackCredentials, logonUser} {
		if proc.Find() != nil {
			return false
		}
	}
	_, err := windows.GetCurrentProcessToken().GetTokenUser()
	return err == nil
}

// The password is also required as DPAPI entropy: a process running as the same
// user cannot silently decrypt the envelope by bypassing our credential dialog.
// A login password change requires enrollment again with the private key phrase.
type windowsEnvelope struct {
	Version    int    `json:"version"`
	Platform   string `json:"platform"`
	Salt       []byte `json:"salt"`
	Ciphertext []byte `json:"ciphertext"`
}

func handleWindowsRequest(req request, authenticate windowsAuthenticator) (response, error) {
	if req.Action == "status" {
		return response{Available: windowsAvailable()}, nil
	}
	if req.Account == "" || (req.Action != "enroll" && req.Action != "unlock") {
		return response{}, errors.New("invalid authentication request")
	}
	var envelope windowsEnvelope
	if req.Action == "enroll" {
		envelope.Version, envelope.Platform = 1, "windows-dpapi-password-v1"
		if len(req.Secret) == 0 || len(req.Secret) > 1<<16 {
			return response{}, errors.New("invalid secret")
		}
		envelope.Salt = make([]byte, 32)
		if _, err := rand.Read(envelope.Salt); err != nil {
			return response{}, err
		}
	} else if err := json.Unmarshal(req.Envelope, &envelope); err != nil {
		return response{}, err
	} else if envelope.Version != 1 || envelope.Platform != "windows-dpapi-password-v1" || len(envelope.Salt) != 32 || len(envelope.Ciphertext) == 0 || len(envelope.Ciphertext) > 1<<18 {
		return response{}, errors.New("invalid device envelope")
	}
	password, err := authenticate(req.Language, req.Action)
	defer clear(password)
	if err != nil {
		return response{}, err
	}
	if len(password) == 0 || password[0] == 0 {
		return response{}, errors.New("empty login password")
	}
	entropy, err := passwordEntropy(password, req.Account, envelope.Salt)
	if err != nil {
		return response{}, err
	}
	defer clear(entropy)
	if req.Action == "unlock" {
		secret, err := protectWindows(envelope.Ciphertext, entropy, false)
		return response{Secret: secret}, err
	}
	envelope.Ciphertext, err = protectWindows(req.Secret, entropy, true)
	if err != nil {
		return response{}, err
	}
	// Verify before handing the envelope to the service for persistence.
	secret, err := protectWindows(envelope.Ciphertext, entropy, false)
	defer clear(secret)
	if err != nil || !bytes.Equal(secret, req.Secret) {
		return response{}, errors.New("device protection verification failed")
	}
	encoded, err := json.Marshal(envelope)
	return response{Envelope: encoded}, err
}

func passwordEntropy(password []uint16, account string, salt []byte) ([]byte, error) {
	raw := make([]byte, len(password)*2)
	defer clear(raw)
	for i, char := range password {
		binary.LittleEndian.PutUint16(raw[i*2:], char)
	}
	binding := sha256.New()
	binding.Write([]byte("Passpine/windows-device-unlock/v1\x00"))
	binding.Write([]byte(account))
	binding.Write(salt)
	return scrypt.Key(raw, binding.Sum(nil), 32768, 8, 1, 32)
}

func protectWindows(data, entropy []byte, encrypt bool) ([]byte, error) {
	if len(data) == 0 || len(entropy) == 0 {
		return nil, errors.New("empty protection data")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	extra := windows.DataBlob{Size: uint32(len(entropy)), Data: &entropy[0]}
	var out windows.DataBlob
	var err error
	if encrypt {
		err = windows.CryptProtectData(&in, nil, &extra, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptUnprotectData(&in, nil, &extra, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	runtime.KeepAlive(data)
	runtime.KeepAlive(entropy)
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	value := unsafe.Slice(out.Data, int(out.Size))
	defer clear(value)
	return append([]byte(nil), value...), nil
}

type credentialUIInfo struct {
	Size    uint32
	Parent  windows.Handle
	Message *uint16
	Caption *uint16
	Banner  windows.Handle
}

func authenticateWindows(language, action string) ([]uint16, error) {
	current, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	name, domain, _, err := current.User.Sid.LookupAccount("")
	if err != nil {
		return nil, err
	}
	username, err := windows.UTF16FromString(domain + `\` + name)
	if err != nil {
		return nil, err
	}
	empty := []uint16{0}
	var size uint32
	_, _, packErr := packCredentials.Call(credPackGeneric, uintptr(unsafe.Pointer(&username[0])), uintptr(unsafe.Pointer(&empty[0])), 0, uintptr(unsafe.Pointer(&size)))
	if !errors.Is(packErr, windows.ERROR_INSUFFICIENT_BUFFER) || size == 0 || size > 1<<20 {
		return nil, errors.New("cannot prepare current account credentials")
	}
	packed := make([]byte, size)
	defer clear(packed)
	ok, _, err := packCredentials.Call(credPackGeneric, uintptr(unsafe.Pointer(&username[0])), uintptr(unsafe.Pointer(&empty[0])), uintptr(unsafe.Pointer(&packed[0])), uintptr(unsafe.Pointer(&size)))
	if ok == 0 {
		return nil, err
	}
	message := "输入当前 Windows 账户的登录密码以解锁 Passpine 密码库"
	if action == "enroll" {
		message = "输入当前 Windows 账户的登录密码以启用 Passpine 系统解锁"
	}
	caption, _ := windows.UTF16PtrFromString("Passpine")
	text, _ := windows.UTF16PtrFromString(i18n.Text(language, message))
	info := credentialUIInfo{Message: text, Caption: caption}
	info.Size = uint32(unsafe.Sizeof(info))
	var authPackage, outSize uint32
	var out unsafe.Pointer
	// GENERIC ensures password credentials (rather than a Hello PIN/certificate).
	// The dialog does not save credentials and only enumerates the current user.
	status, _, _ := promptCredentials.Call(uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&authPackage)), uintptr(unsafe.Pointer(&packed[0])), uintptr(size), uintptr(unsafe.Pointer(&out)), uintptr(unsafe.Pointer(&outSize)), 0, credUIGeneric|credUICurrentUser)
	if out != nil {
		defer windows.CoTaskMemFree(out)
		defer clear(unsafe.Slice((*byte)(out), int(outSize)))
	}
	if status != 0 {
		return nil, syscall.Errno(status)
	}
	if out == nil || outSize == 0 || outSize > 1<<20 {
		return nil, errors.New("invalid authentication buffer")
	}
	return verifyWindowsCredentials(out, outSize, current.User.Sid, logonWindowsAccount)
}

type windowsLogon func(user, domain, password *uint16) (*windows.SID, error)

func verifyWindowsCredentials(buffer unsafe.Pointer, size uint32, current *windows.SID, logon windowsLogon) ([]uint16, error) {
	var userSize, domainSize, passwordSize uint32
	_, _, err := unpackCredentials.Call(0, uintptr(buffer), uintptr(size), 0, uintptr(unsafe.Pointer(&userSize)), 0, uintptr(unsafe.Pointer(&domainSize)), 0, uintptr(unsafe.Pointer(&passwordSize)))
	if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) || userSize == 0 || passwordSize == 0 || userSize > 1<<16 || domainSize > 1<<16 || passwordSize > 1<<16 {
		return nil, errors.New("invalid credential sizes")
	}
	user := make([]uint16, userSize)
	domain := make([]uint16, max(domainSize, 1))
	password := make([]uint16, passwordSize)
	defer clear(password)
	ok, _, err := unpackCredentials.Call(0, uintptr(buffer), uintptr(size), uintptr(unsafe.Pointer(&user[0])), uintptr(unsafe.Pointer(&userSize)), uintptr(unsafe.Pointer(&domain[0])), uintptr(unsafe.Pointer(&domainSize)), uintptr(unsafe.Pointer(&password[0])), uintptr(unsafe.Pointer(&passwordSize)))
	if ok == 0 {
		return nil, err
	}
	if password[0] == 0 {
		return nil, errors.New("empty login password")
	}
	// Normalize DOMAIN\user when the generic provider returns it in the user
	// field. Bare UPN credentials use a nil domain; an explicit identity provider
	// such as MicrosoftAccount must be preserved even when its name contains @.
	userName := windows.UTF16ToString(user)
	domainName := windows.UTF16ToString(domain)
	if before, after, found := strings.Cut(userName, `\`); found {
		domainName, userName = before, after
	}
	userPtr, err := windows.UTF16PtrFromString(userName)
	if err != nil {
		return nil, err
	}
	var domainPtr *uint16
	if domainName != "" {
		domainPtr, err = windows.UTF16PtrFromString(domainName)
		if err != nil {
			return nil, err
		}
	}
	verified, err := logon(userPtr, domainPtr, &password[0])
	if err != nil {
		return nil, err
	}
	if verified == nil || !windows.EqualSid(current, verified) {
		return nil, errors.New("authenticated a different Windows account")
	}
	// Return a canonical, unterminated representation for key derivation.
	for i, char := range password {
		if char == 0 {
			return append([]uint16(nil), password[:i]...), nil
		}
	}
	return nil, errors.New("unterminated login password")
}

func logonWindowsAccount(user, domain, password *uint16) (*windows.SID, error) {
	var token windows.Token
	// INTERACTIVE verifies the password locally, including cached domain logon;
	// NEW_CREDENTIALS must never be used because it defers password validation.
	ok, _, err := logonUser.Call(uintptr(unsafe.Pointer(user)), uintptr(unsafe.Pointer(domain)), uintptr(unsafe.Pointer(password)), 2, 0, uintptr(unsafe.Pointer(&token)))
	if ok == 0 {
		return nil, err
	}
	defer token.Close()
	verified, err := token.GetTokenUser()
	if err != nil {
		return nil, err
	}
	return verified.User.Sid, nil
}
