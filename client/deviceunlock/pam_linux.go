package deviceunlock

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

var pamLibrary struct {
	once         sync.Once
	err          error
	start        func(string, string, *pamConversation, *uintptr) int32
	authenticate func(uintptr, int32) int32
	account      func(uintptr, int32) int32
	end          func(uintptr, int32) int32
	getItem      func(uintptr, int32, *unsafe.Pointer) int32
	calloc       func(uintptr, uintptr) unsafe.Pointer
	free         func(unsafe.Pointer)
}

type pamConversation struct{ callback, data uintptr }
type pamMessage struct {
	style int32
	text  uintptr
}
type pamResponse struct {
	text unsafe.Pointer
	code int32
}

func loadPAM() error {
	pamLibrary.once.Do(func() {
		lib, err := purego.Dlopen("libpam.so.0", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			pamLibrary.err = err
			return
		}
		for _, symbol := range []struct {
			target any
			name   string
		}{
			{&pamLibrary.start, "pam_start"},
			{&pamLibrary.authenticate, "pam_authenticate"},
			{&pamLibrary.account, "pam_acct_mgmt"},
			{&pamLibrary.end, "pam_end"},
			{&pamLibrary.getItem, "pam_get_item"},
			{&pamLibrary.calloc, "calloc"},
			{&pamLibrary.free, "free"},
		} {
			address, err := purego.Dlsym(lib, symbol.name)
			if err != nil {
				pamLibrary.err = err
				return
			}
			purego.RegisterFunc(symbol.target, address)
		}
	})
	return pamLibrary.err
}

// PAM takes ownership of the conversation responses. Allocate them in libc,
// never return Go pointers to C, and reject additional password/MFA challenges
// rather than silently reusing a password for a different factor.
func authenticatePAM(username string, password []byte) error {
	if len(password) == 0 {
		return errors.New("empty PAM password")
	}
	if err := loadPAM(); err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	prompted := false
	callback := purego.NewCallback(func(count int32, messages unsafe.Pointer, output *unsafe.Pointer, _ uintptr) int32 {
		const conversationError = 19
		if output == nil {
			return conversationError
		}
		*output = nil
		if count <= 0 || count > 32 || messages == nil {
			return conversationError
		}
		memory := pamLibrary.calloc(uintptr(count), unsafe.Sizeof(pamResponse{}))
		if memory == nil {
			return conversationError
		}
		responses := unsafe.Slice((*pamResponse)(memory), int(count))
		lengths := make([]int, int(count))
		success := false
		defer func() {
			if success {
				return
			}
			for i, response := range responses {
				if response.text != nil {
					clear(unsafe.Slice((*byte)(response.text), lengths[i]))
					pamLibrary.free(response.text)
				}
			}
			pamLibrary.free(memory)
		}()
		for i, message := range unsafe.Slice((**pamMessage)(messages), int(count)) {
			if message == nil {
				return conversationError
			}
			var text []byte
			switch message.style {
			case 1: // PAM_PROMPT_ECHO_OFF
				if prompted {
					return conversationError
				}
				prompted = true
				text = password
			case 2: // PAM_PROMPT_ECHO_ON
				text = []byte(username)
			case 3, 4: // PAM_ERROR_MSG, PAM_TEXT_INFO
				continue
			default:
				return conversationError
			}
			value := pamLibrary.calloc(uintptr(len(text)+1), 1)
			if value == nil {
				return conversationError
			}
			copy(unsafe.Slice((*byte)(value), len(text)), text)
			responses[i].text = value
			lengths[i] = len(text)
		}
		*output = memory
		success = true
		return 0
	})
	conversation := pamConversation{callback: callback}
	var handle uintptr
	status := pamLibrary.start("login", username, &conversation, &handle)
	if status != 0 {
		return fmt.Errorf("PAM start failed: %d", status)
	}
	defer func() { pamLibrary.end(handle, status); runtime.KeepAlive(password) }()
	const disallowNullToken = 1
	status = pamLibrary.authenticate(handle, disallowNullToken)
	if status != 0 {
		return fmt.Errorf("PAM authentication failed: %d", status)
	}
	// No password-less PAM route may create a password-protected enrollment.
	if !prompted {
		return errors.New("PAM did not validate the login password")
	}
	status = pamLibrary.account(handle, 0)
	if status != 0 {
		return fmt.Errorf("PAM account unavailable: %d", status)
	}
	// PAM modules may remap PAM_USER during authentication or account checks.
	// Require the verified identity to remain the actual desktop account.
	var identity unsafe.Pointer
	if pamLibrary.getItem(handle, 2, &identity) != 0 || identity == nil {
		return errors.New("PAM returned no authenticated account")
	}
	for i := 0; i < len(username); i++ {
		if *(*byte)(unsafe.Add(identity, i)) != username[i] {
			return errors.New("PAM authenticated a different account")
		}
	}
	if *(*byte)(unsafe.Add(identity, len(username))) != 0 {
		return errors.New("PAM authenticated a different account")
	}
	return nil
}
