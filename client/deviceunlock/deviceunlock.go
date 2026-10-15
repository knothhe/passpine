// Package deviceunlock bridges macOS authentication and device-bound encryption.
package deviceunlock

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type Provider interface {
	Available() bool
	Enroll(context.Context, string, []byte) ([]byte, error)
	Unlock(context.Context, string, []byte) ([]byte, error)
}

type systemProvider struct {
	binary    string
	available bool
}

type request struct {
	Action   string          `json:"action"`
	Account  string          `json:"account,omitempty"`
	Secret   []byte          `json:"secret,omitempty"`
	Envelope json.RawMessage `json:"envelope,omitempty"`
}
type response struct {
	Available bool            `json:"available"`
	Envelope  json.RawMessage `json:"envelope"`
	Secret    []byte          `json:"secret"`
	Error     string          `json:"error"`
}

func New() Provider {
	p := &systemProvider{}
	if runtime.GOOS != "darwin" {
		return p
	}
	executable, err := os.Executable()
	if err != nil {
		return p
	}
	p.binary = filepath.Join(filepath.Dir(executable), "passpine-device-unlock")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := p.run(ctx, request{Action: "status"})
	p.available = err == nil && result.Available
	return p
}
func (p *systemProvider) Available() bool { return p.available }
func (p *systemProvider) Enroll(ctx context.Context, account string, passphrase []byte) ([]byte, error) {
	result, err := p.run(ctx, request{Action: "enroll", Account: account, Secret: passphrase})
	if err != nil {
		return nil, err
	}
	if len(result.Envelope) == 0 || string(result.Envelope) == "null" {
		return nil, errors.New("系统解锁未返回有效的设备保护数据")
	}
	return result.Envelope, nil
}
func (p *systemProvider) Unlock(ctx context.Context, account string, envelope []byte) ([]byte, error) {
	result, err := p.run(ctx, request{Action: "unlock", Account: account, Envelope: envelope})
	if err != nil {
		return nil, err
	}
	if len(result.Secret) == 0 {
		return nil, errors.New("系统解锁数据无效，请使用私钥口令")
	}
	return result.Secret, nil
}
func (p *systemProvider) run(ctx context.Context, req request) (response, error) {
	if p.binary == "" {
		return response{}, errors.New("此设备不支持系统解锁")
	}
	input, err := json.Marshal(req)
	if err != nil {
		return response{}, err
	}
	defer clear(input)
	cmd := exec.CommandContext(ctx, p.binary)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	var output limitedOutput
	cmd.Stdout = &output
	if err = cmd.Run(); err != nil {
		clear(output.buffer.Bytes())
		return response{}, errors.New("系统验证未完成，请使用私钥口令")
	}
	defer clear(output.buffer.Bytes())
	var result response
	if err = json.Unmarshal(output.buffer.Bytes(), &result); err != nil {
		return response{}, errors.New("系统解锁响应无效")
	}
	if result.Error != "" {
		clear(result.Secret)
		return response{}, errors.New(result.Error)
	}
	return result, nil
}

type limitedOutput struct{ buffer bytes.Buffer }

func (b *limitedOutput) Write(data []byte) (int, error) {
	if len(data) > 1<<20-b.buffer.Len() {
		return 0, errors.New("system unlock response too large")
	}
	return b.buffer.Write(data)
}
