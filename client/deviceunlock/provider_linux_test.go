package deviceunlock

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("PASSPINE_TEST_BLOCK_AUTH") == "1" && len(os.Args) == 2 && os.Args[1] == helperArgument {
		runLinuxHelper(os.Stdin, os.Stdout, func(ctx context.Context, _, _ string) ([]byte, error) {
			child := exec.Command("sleep", "120")
			if err := child.Start(); err != nil {
				return nil, err
			}
			_ = os.WriteFile(os.Getenv("PASSPINE_TEST_AUTH_STARTED"), []byte(strconv.Itoa(child.Process.Pid)), 0600)
			<-ctx.Done()
			return nil, ctx.Err()
		})
		return
	}
	if RunHelper() {
		return
	}
	os.Exit(m.Run())
}

func linuxFixturePassword(context.Context, string, string) ([]byte, error) {
	return []byte("fictional-login-password-密码"), nil
}

func linuxFixtureEnvelope(t *testing.T) request {
	t.Helper()
	result, err := handleLinuxRequest(context.Background(), request{Action: "enroll", Account: "fixture-vault", Secret: []byte("fictional-vault-passphrase")}, linuxFixturePassword)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(result.Envelope, []byte("fictional")) {
		t.Fatal("envelope exposes plaintext")
	}
	return request{Action: "unlock", Account: "fixture-vault", Envelope: result.Envelope}
}

func TestLinuxPasswordProtectionAndAccountBinding(t *testing.T) {
	req := linuxFixtureEnvelope(t)
	req.Language = "zh-CN"
	calls := 0
	authenticate := func(ctx context.Context, language, action string) ([]byte, error) {
		calls++
		if language != "zh-CN" || action != "unlock" {
			t.Fatal("authentication lost locale or action")
		}
		return linuxFixturePassword(ctx, language, action)
	}
	for i := 0; i < 2; i++ {
		result, err := handleLinuxRequest(context.Background(), req, authenticate)
		if err != nil || string(result.Secret) != "fictional-vault-passphrase" {
			t.Fatal("unlock failed", err)
		}
		clear(result.Secret)
	}
	if calls != 2 {
		t.Fatal("unlock bypassed authentication")
	}
	req.Account = "another-vault"
	result, err := handleLinuxRequest(context.Background(), req, linuxFixturePassword)
	if err == nil || len(result.Secret) != 0 {
		t.Fatal("envelope accepted for another vault")
	}
	req.Account = "fixture-vault"
	result, err = handleLinuxRequest(context.Background(), req, func(context.Context, string, string) ([]byte, error) { return []byte("wrong-login-password"), nil })
	if err == nil || len(result.Secret) != 0 {
		t.Fatal("wrong password decrypted envelope")
	}
	result, err = handleLinuxRequest(context.Background(), req, func(context.Context, string, string) ([]byte, error) {
		return nil, errors.New("PAM rejected authentication")
	})
	if err == nil || len(result.Secret) != 0 {
		t.Fatal("failed authentication returned secret")
	}
	var envelope linuxEnvelope
	if err = json.Unmarshal(req.Envelope, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Ciphertext[0] ^= 1
	req.Envelope, _ = json.Marshal(envelope)
	result, err = handleLinuxRequest(context.Background(), req, linuxFixturePassword)
	if err == nil || len(result.Secret) != 0 {
		t.Fatal("tampered ciphertext accepted")
	}
}

func TestLinuxInvalidRequestsNeverPrompt(t *testing.T) {
	prompt := func(context.Context, string, string) ([]byte, error) {
		t.Fatal("invalid request opened authentication")
		return nil, nil
	}
	for _, req := range []request{
		{Action: "delete", Account: "fixture"},
		{Action: "unlock"},
		{Action: "enroll", Account: "fixture"},
		{Action: "unlock", Account: "fixture", Envelope: []byte(`{"version":1,"platform":"windows-dpapi-password-v1"}`)},
	} {
		if _, err := handleLinuxRequest(context.Background(), req, prompt); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := handleLinuxRequest(ctx, request{Action: "enroll", Account: "fixture", Secret: []byte("fake")}, prompt); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled request not rejected", err)
	}
}

func TestLinuxPinentryProtocol(t *testing.T) {
	password, err := parsePinentryPassword([]byte("OK hello\nOK\nD abc%25+%0A%E5%AF%86%E7%A0%81\nOK\nOK closing\n"))
	if err != nil || string(password) != "abc%+\n密码" {
		t.Fatal("password encoding changed", err)
	}
	clear(password)
	for _, raw := range []string{"OK\n", "D secret\nERR 123 cancelled\n", "D secret\nD duplicate\n", "D %00\n", "D %zz\n", "D %\n", "D " + strings.Repeat("a", 513) + "\n"} {
		result, err := parsePinentryPassword([]byte(raw))
		if err == nil || len(result) != 0 {
			t.Fatal("invalid or cancelled password accepted")
		}
	}
}

func TestLinuxHelperNeverExposesSecretsOnFailure(t *testing.T) {
	var output bytes.Buffer
	runLinuxHelper(strings.NewReader(`{"action":"enroll","account":"fixture","secret":"ZmFrZQ=="}`), &output, func(context.Context, string, string) ([]byte, error) {
		return nil, errors.New("private authentication failure detail")
	})
	var result response
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error == "" || len(result.Secret) != 0 || (len(result.Envelope) != 0 && string(result.Envelope) != "null") || bytes.Contains(output.Bytes(), []byte("private authentication")) {
		t.Fatal("helper exposed details or secret")
	}
}

func TestLinuxAuthenticationProcessCancellation(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	started := t.TempDir() + "/started"
	t.Setenv("PASSPINE_TEST_BLOCK_AUTH", "1")
	t.Setenv("PASSPINE_TEST_AUTH_STARTED", started)
	provider := &systemProvider{binary: executable, args: []string{helperArgument}, available: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := provider.Enroll(ctx, "fixture", []byte("fake-secret")); done <- err }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("authentication helper did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	childPID, err := os.ReadFile(started)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		assertProcessStopped(t, string(childPID))
		if err == nil {
			t.Fatal("cancelled authentication succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("authentication helper survived cancellation")
	}
}

func assertProcessStopped(t *testing.T, pid string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile("/proc/" + pid + "/stat")
		if os.IsNotExist(err) {
			return
		}
		// A killed child can briefly remain a zombie until its new parent reaps it.
		if err == nil {
			end := strings.LastIndexByte(string(data), ')')
			if end >= 0 && strings.HasPrefix(string(data[end+1:]), " Z ") {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("authentication dialog child survived cancellation")
}
