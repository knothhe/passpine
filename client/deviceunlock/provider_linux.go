package deviceunlock

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"syscall"
	"time"

	"github.com/knothhe/passpine/client/i18n"
	"golang.org/x/crypto/scrypt"
)

const helperArgument = "--device-unlock-helper"

func configureProvider(p *systemProvider, executable string) {
	p.binary, p.args = executable, []string{helperArgument}
}

func configureCommand(cmd *exec.Cmd) {
	// A cancelled request must also close the pinentry dialog, not leave it
	// running after the authentication helper has been terminated.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
}

func RunHelper() bool {
	if len(os.Args) != 2 || os.Args[1] != helperArgument {
		return false
	}
	runLinuxHelper(os.Stdin, os.Stdout, authenticateLinux)
	return true
}

type linuxAuthenticator func(context.Context, string, string) ([]byte, error)

type linuxEnvelope struct {
	Version    int    `json:"version"`
	Platform   string `json:"platform"`
	Salt       []byte `json:"salt"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

func runLinuxHelper(input io.Reader, output io.Writer, authenticate linuxAuthenticator) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
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
	result := response{}
	if err == nil {
		result, err = handleLinuxRequest(ctx, req, authenticate)
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

func linuxAvailable() bool {
	if os.Getuid() == 0 || (os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "") {
		return false
	}
	if _, err := graphicalPinentry(); err != nil {
		return false
	}
	if err := loadPAM(); err != nil {
		return false
	}
	_, err := os.Stat("/etc/pam.d/login")
	return err == nil
}

// The login password is part of the encryption key, not just a UI gate. A
// process cannot bypass PAM and decrypt the envelope without that password.
// Changing the login password requires enrollment again with the vault phrase.
func handleLinuxRequest(ctx context.Context, req request, authenticate linuxAuthenticator) (response, error) {
	if req.Action == "status" {
		return response{Available: linuxAvailable()}, nil
	}
	if req.Account == "" || (req.Action != "enroll" && req.Action != "unlock") {
		return response{}, errors.New("invalid authentication request")
	}
	var envelope linuxEnvelope
	if req.Action == "enroll" {
		if len(req.Secret) == 0 || len(req.Secret) > 1<<16 {
			return response{}, errors.New("invalid secret")
		}
		envelope = linuxEnvelope{Version: 1, Platform: "linux-pam-password-v1", Salt: make([]byte, 32)}
		if _, err := rand.Read(envelope.Salt); err != nil {
			return response{}, err
		}
	} else {
		if err := json.Unmarshal(req.Envelope, &envelope); err != nil {
			return response{}, err
		}
		if envelope.Version != 1 || envelope.Platform != "linux-pam-password-v1" || len(envelope.Salt) != 32 || len(envelope.Nonce) != 12 || len(envelope.Ciphertext) < 16 || len(envelope.Ciphertext) > 1<<18 {
			return response{}, errors.New("invalid device envelope")
		}
	}
	if err := ctx.Err(); err != nil {
		return response{}, err
	}
	password, err := authenticate(ctx, req.Language, req.Action)
	defer clear(password)
	if err != nil {
		return response{}, err
	}
	if len(password) == 0 {
		return response{}, errors.New("empty login password")
	}
	if err := ctx.Err(); err != nil {
		return response{}, err
	}
	binding, err := linuxBinding(req.Account)
	if err != nil {
		return response{}, err
	}
	salt := append(append([]byte(nil), binding...), envelope.Salt...)
	key, err := scrypt.Key(password, salt, 32768, 8, 1, 32)
	if err != nil {
		return response{}, err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return response{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return response{}, err
	}
	if req.Action == "unlock" {
		secret, err := aead.Open(nil, envelope.Nonce, envelope.Ciphertext, binding)
		return response{Secret: secret}, err
	}
	envelope.Nonce = make([]byte, aead.NonceSize())
	if _, err = rand.Read(envelope.Nonce); err != nil {
		return response{}, err
	}
	envelope.Ciphertext = aead.Seal(nil, envelope.Nonce, req.Secret, binding)
	encoded, err := json.Marshal(envelope)
	return response{Envelope: encoded}, err
}

func linuxBinding(account string) ([]byte, error) {
	machine, err := os.ReadFile("/etc/machine-id")
	if err != nil || len(bytes.TrimSpace(machine)) == 0 {
		return nil, errors.New("missing machine identity")
	}
	binding := sha256.Sum256([]byte(fmt.Sprintf("Passpine/linux-device-unlock/v1\x00%s\x00%d\x00%s", strings.TrimSpace(string(machine)), os.Getuid(), account)))
	return binding[:], nil
}

func graphicalPinentry() (string, error) {
	for _, name := range []string{"pinentry-gnome3", "pinentry-qt", "pinentry-gtk-2"} {
		if binary, err := exec.LookPath(name); err == nil {
			return binary, nil
		}
	}
	return "", errors.New("graphical pinentry is not installed")
}

func authenticateLinux(ctx context.Context, language, action string) ([]byte, error) {
	current, err := user.Current()
	if err != nil || current.Username == "" || current.Uid != fmt.Sprint(os.Getuid()) {
		return nil, errors.New("cannot resolve current account")
	}
	password, err := promptLinuxPassword(ctx, language, action)
	if err != nil {
		return nil, err
	}
	if err = authenticatePAM(current.Username, password); err != nil {
		clear(password)
		return nil, err
	}
	return password, nil
}

func promptLinuxPassword(ctx context.Context, language, action string) ([]byte, error) {
	binary, err := graphicalPinentry()
	if err != nil {
		return nil, err
	}
	message := "输入当前 Linux 账户的登录密码以解锁 Passpine 密码库"
	if action == "enroll" {
		message = "输入当前 Linux 账户的登录密码以启用 Passpine 系统解锁"
	}
	escape := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	input := "SETTITLE Passpine\nSETDESC " + escape.Replace(i18n.Text(language, message)) + "\nSETPROMPT " + escape.Replace(i18n.Text(language, "登录密码")) + "\nGETPIN\nBYE\n"
	cmd := exec.CommandContext(ctx, binary)
	cmd.Stdin = strings.NewReader(input)
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	var output limitedOutput
	cmd.Stdout = &output
	err = cmd.Run()
	defer clear(output.buffer.Bytes())
	if err != nil {
		return nil, err
	}
	return parsePinentryPassword(output.buffer.Bytes())
}

func parsePinentryPassword(output []byte) ([]byte, error) {
	var password []byte
	for _, line := range bytes.Split(output, []byte{'\n'}) {
		if bytes.HasPrefix(line, []byte("ERR")) {
			clear(password)
			return nil, errors.New("password dialog cancelled or failed")
		}
		if !bytes.HasPrefix(line, []byte("D ")) {
			continue
		}
		if password != nil {
			clear(password)
			return nil, errors.New("unexpected password response")
		}
		data := bytes.TrimSuffix(line[2:], []byte{'\r'})
		password = make([]byte, 0, len(data))
		for i := 0; i < len(data); i++ {
			value := data[i]
			if value == '%' {
				if i+2 >= len(data) {
					clear(password)
					return nil, errors.New("invalid password encoding")
				}
				a, b := hexNibble(data[i+1]), hexNibble(data[i+2])
				if a < 0 || b < 0 {
					clear(password)
					return nil, errors.New("invalid password encoding")
				}
				value = byte(a*16 + b)
				i += 2
			}
			if value == 0 || len(password) >= 512 {
				clear(password)
				return nil, errors.New("invalid password length or content")
			}
			password = append(password, value)
		}
	}
	if len(password) == 0 {
		return nil, errors.New("empty password response")
	}
	return password, nil
}

func hexNibble(value byte) int {
	switch {
	case value >= '0' && value <= '9':
		return int(value - '0')
	case value >= 'a' && value <= 'f':
		return int(value-'a') + 10
	case value >= 'A' && value <= 'F':
		return int(value-'A') + 10
	}
	return -1
}
