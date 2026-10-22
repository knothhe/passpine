package deviceunlock

import (
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Exercise the actual C callback ABI and libc-owned reply memory without
// attempting to authenticate against the developer's real PAM account.
func TestPAMConversationAndFailurePaths(t *testing.T) {
	if err := loadPAM(); err != nil {
		t.Skip("PAM not installed:", err)
	}
	getItem := pamLibrary.getItem
	start, auth, account, end := pamLibrary.start, pamLibrary.authenticate, pamLibrary.account, pamLibrary.end
	t.Cleanup(func() {
		pamLibrary.start = start
		pamLibrary.authenticate = auth
		pamLibrary.account = account
		pamLibrary.end = end
		pamLibrary.getItem = getItem
	})
	for _, test := range []struct {
		name                      string
		authStatus, accountStatus int32
		prompt                    bool
		wantErr                   bool
		identity                  string
		identityStatus            int32
	}{
		{name: "success", prompt: true},
		{name: "wrong password", authStatus: 7, prompt: true, wantErr: true},
		{name: "expired account", accountStatus: 12, prompt: true, wantErr: true},
		{name: "passwordless PAM route", wantErr: true},
		{name: "account remapped", prompt: true, identity: "other-user", wantErr: true},
		{name: "missing account", prompt: true, identityStatus: 4, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var callback func(int32, unsafe.Pointer, *unsafe.Pointer, uintptr) int32
			pamLibrary.start = func(service, username string, conversation *pamConversation, handle *uintptr) int32 {
				if service != "login" || username != "fixture-user" {
					t.Fatal("unexpected PAM account or policy")
				}
				purego.RegisterFunc(&callback, conversation.callback)
				*handle = 1
				return 0
			}
			identityName := test.identity
			if identityName == "" {
				identityName = "fixture-user"
			}
			identity := pamLibrary.calloc(uintptr(len(identityName)+1), 1)
			if identity == nil {
				t.Fatal("identity allocation failed")
			}
			defer pamLibrary.free(identity)
			copy(unsafe.Slice((*byte)(identity), len(identityName)), identityName)
			pamLibrary.getItem = func(_ uintptr, item int32, value *unsafe.Pointer) int32 {
				if item != 2 {
					t.Fatal("wrong PAM identity item")
				}
				*value = identity
				return test.identityStatus
			}
			accountCalled, ended := false, false
			pamLibrary.authenticate = func(_ uintptr, flags int32) int32 {
				if flags != 1 {
					t.Fatal("empty login passwords allowed")
				}
				if test.prompt {
					message := pamMessage{style: 1}
					messages := []*pamMessage{&message}
					var response unsafe.Pointer
					status := callback(1, unsafe.Pointer(&messages[0]), &response, 0)
					if status != 0 || response == nil {
						t.Fatal("PAM conversation failed")
					}
					reply := (*pamResponse)(response)
					if got := string(unsafe.Slice((*byte)(reply.text), len("fake-password"))); got != "fake-password" {
						t.Fatal("PAM received incorrect credentials")
					}
					clear(unsafe.Slice((*byte)(reply.text), len("fake-password")))
					pamLibrary.free(reply.text)
					pamLibrary.free(response)
					// Never reuse the password for a second prompt or factor.
					response = nil
					if callback(1, unsafe.Pointer(&messages[0]), &response, 0) == 0 || response != nil {
						t.Fatal("repeated password challenge accepted")
					}
				}
				return test.authStatus
			}
			pamLibrary.account = func(uintptr, int32) int32 { accountCalled = true; return test.accountStatus }
			pamLibrary.end = func(uintptr, int32) int32 { ended = true; return 0 }
			err := authenticatePAM("fixture-user", []byte("fake-password"))
			if (err != nil) != test.wantErr {
				t.Fatalf("unexpected authentication outcome: %v", err)
			}
			if !ended {
				t.Fatal("PAM transaction not closed")
			}
			if test.authStatus != 0 && accountCalled {
				t.Fatal("failed authentication continued to account validation")
			}
		})
	}
}
