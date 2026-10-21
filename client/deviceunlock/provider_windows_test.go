package deviceunlock

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestMain(m *testing.M) {
	if os.Getenv("PASSPINE_TEST_BLOCK_AUTH") == "1" && len(os.Args) == 2 && os.Args[1] == helperArgument {
		runWindowsHelper(os.Stdin, os.Stdout, func(string, string) ([]uint16, error) {
			// Signal readiness before blocking so the parent cancels a running
			// authentication, rather than a process that has not started yet.
			_ = os.WriteFile(os.Getenv("PASSPINE_TEST_AUTH_STARTED"), nil, 0600)
			for {
				time.Sleep(time.Second)
			}
		})
		return
	}
	if RunHelper() {
		return
	}
	os.Exit(m.Run())
}

func fixturePassword(string, string) ([]uint16, error) {
	value, _ := windows.UTF16FromString("fictional-login-password-密码")
	return value[:len(value)-1], nil
}

func enrollWindowsFixture(t *testing.T) request {
	t.Helper()
	result, err := handleWindowsRequest(request{Action: "enroll", Account: "fixture-vault", Secret: []byte("fictional-private-key-passphrase")}, fixturePassword)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(result.Envelope, []byte("fictional")) {
		t.Fatal("envelope contains a secret")
	}
	return request{Action: "unlock", Account: "fixture-vault", Envelope: result.Envelope}
}

func TestWindowsPasswordProtectedRoundTripAndVaultBinding(t *testing.T) {
	req := enrollWindowsFixture(t)
	calls := 0
	authenticate := func(language, action string) ([]uint16, error) {
		calls++
		if language != "zh-CN" || action != "unlock" {
			t.Fatal("authentication request lost its locale or action")
		}
		return fixturePassword(language, action)
	}
	req.Language = "zh-CN"
	result, err := handleWindowsRequest(req, authenticate)
	defer clear(result.Secret)
	if err != nil || string(result.Secret) != "fictional-private-key-passphrase" || calls != 1 {
		t.Fatal("round trip failed", err)
	}
	req.Account = "other-vault"
	result, err = handleWindowsRequest(req, authenticate)
	if err == nil || len(result.Secret) != 0 {
		t.Fatal("another vault decrypted the envelope")
	}
}

func TestWindowsPasswordChangeCancellationAndTamperingFailClosed(t *testing.T) {
	req := enrollWindowsFixture(t)
	for _, test := range []struct {
		name string
		auth windowsAuthenticator
	}{
		{"changed password", func(string, string) ([]uint16, error) { return []uint16{'w', 'r', 'o', 'n', 'g'}, nil }},
		{"cancelled", func(string, string) ([]uint16, error) { return nil, context.Canceled }},
		{"empty password", func(string, string) ([]uint16, error) { return []uint16{0}, nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := handleWindowsRequest(req, test.auth)
			if err == nil || len(result.Secret) != 0 {
				t.Fatal("failed authentication returned a secret")
			}
		})
	}
	var envelope windowsEnvelope
	if err := json.Unmarshal(req.Envelope, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Ciphertext[len(envelope.Ciphertext)-1] ^= 1
	req.Envelope, _ = json.Marshal(envelope)
	result, err := handleWindowsRequest(req, fixturePassword)
	if err == nil || len(result.Secret) != 0 {
		t.Fatal("tampered envelope decrypted")
	}
}

func TestWindowsRejectsInvalidRequestsBeforePrompting(t *testing.T) {
	for _, req := range []request{
		{Action: "delete", Account: "vault"},
		{Action: "enroll", Secret: []byte("secret")},
		{Action: "enroll", Account: "vault"},
		{Action: "unlock", Account: "vault", Envelope: []byte(`{}`)},
		{Action: "unlock", Account: "vault", Envelope: []byte(`{"version":1,"platform":"macos"}`)},
	} {
		if _, err := handleWindowsRequest(req, func(string, string) ([]uint16, error) {
			t.Fatal("invalid request prompted for credentials")
			return nil, nil
		}); err == nil {
			t.Fatal("accepted invalid request")
		}
	}
	for _, input := range []string{"invalid-json", strings.Repeat("x", (1<<20)+1)} {
		var output bytes.Buffer
		runWindowsHelper(strings.NewReader(input), &output, fixturePassword)
		var result response
		if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.Error == "" || len(result.Secret) != 0 {
			t.Fatal("invalid helper error response", err)
		}
	}
}

func TestWindowsHelperAvailabilityAndCancellation(t *testing.T) {
	p := New()
	if !p.Available() {
		t.Fatal("Windows credential APIs were not detected")
	}
	started := t.TempDir() + "/started"
	t.Setenv("PASSPINE_TEST_BLOCK_AUTH", "1")
	t.Setenv("PASSPINE_TEST_AUTH_STARTED", started)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := p.Enroll(ctx, "fixture-vault", []byte("fixture-private-key-passphrase"))
		done <- err
	}()
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
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled helper succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not terminate authentication helper")
	}
}

func TestWindowsClearsPasswordAfterUse(t *testing.T) {
	password, _ := fixturePassword("", "")
	_, err := handleWindowsRequest(request{Action: "enroll", Account: "fixture-vault", Secret: []byte("fixture-secret")}, func(string, string) ([]uint16, error) { return password, nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, char := range password {
		if char != 0 {
			t.Fatal("login password retained in memory")
		}
	}
}

func TestWindowsProviderReportsAuthenticationFailureWithoutSecrets(t *testing.T) {
	input, _ := json.Marshal(request{Action: "enroll", Account: "fixture-vault", Secret: []byte("fixture-secret")})
	var output bytes.Buffer
	runWindowsHelper(bytes.NewReader(input), &output, func(string, string) ([]uint16, error) { return nil, errors.New("sensitive system detail") })
	if bytes.Contains(output.Bytes(), []byte("sensitive")) || bytes.Contains(output.Bytes(), []byte("fixture-secret")) {
		t.Fatal("helper leaked authentication data")
	}
}

func TestWindowsNativeCredentialUnpackingAndAccountCheck(t *testing.T) {
	current, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	other, err := windows.StringToSid("S-1-5-18")
	if err != nil {
		t.Fatal(err)
	}
	if windows.EqualSid(current.User.Sid, other) {
		t.Skip("test requires an interactive user rather than SYSTEM")
	}
	for _, username := range []string{`FIXTURE\user`, "fixture@example.test", `MicrosoftAccount\fixture@example.test`} {
		user, _ := windows.UTF16PtrFromString(username)
		password, _ := windows.UTF16FromString("fictional-password-密码")
		var size uint32
		packCredentials.Call(credPackGeneric, uintptr(unsafe.Pointer(user)), uintptr(unsafe.Pointer(&password[0])), 0, uintptr(unsafe.Pointer(&size)))
		packed := make([]byte, size)
		ok, _, err := packCredentials.Call(credPackGeneric, uintptr(unsafe.Pointer(user)), uintptr(unsafe.Pointer(&password[0])), uintptr(unsafe.Pointer(&packed[0])), uintptr(unsafe.Pointer(&size)))
		clear(password)
		if ok == 0 {
			t.Fatal(err)
		}
		for _, mismatch := range []bool{false, true} {
			var captured *uint16
			secret, err := verifyWindowsCredentials(unsafe.Pointer(&packed[0]), size, current.User.Sid, func(user, domain, password *uint16) (*windows.SID, error) {
				captured = password
				if windows.UTF16PtrToString(password) != "fictional-password-密码" {
					t.Fatal("native unpacking changed password")
				}
				if strings.HasPrefix(username, `MicrosoftAccount\`) {
					if windows.UTF16PtrToString(domain) != "MicrosoftAccount" || windows.UTF16PtrToString(user) != "fixture@example.test" {
						t.Fatal("Microsoft account provider was not preserved")
					}
				} else if strings.Contains(username, "@") {
					if domain != nil || windows.UTF16PtrToString(user) != username {
						t.Fatal("UPN credentials were not preserved")
					}
				} else if windows.UTF16PtrToString(domain) != "FIXTURE" || windows.UTF16PtrToString(user) != "user" {
					t.Fatal("domain credentials were not normalized")
				}
				if mismatch {
					return other, nil
				}
				return current.User.Sid, nil
			})
			if mismatch {
				if err == nil || len(secret) != 0 {
					t.Fatal("another account passed authentication")
				}
			} else if err != nil || windows.UTF16ToString(secret) != "fictional-password-密码" {
				t.Fatal("current user authentication failed", err)
			}
			clear(secret)
			if captured == nil || *captured != 0 {
				t.Fatal("unpacked password was not cleared")
			}
		}
		clear(packed)
	}
}
