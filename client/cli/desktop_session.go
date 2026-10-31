package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/internal/clisession"
	"github.com/knothhe/passpine/client/nativehost"
	"github.com/knothhe/passpine/client/vault"
)

// Both backends implement the same operations. Terminal interaction, editors,
// clipboard handling and Git commits remain in the command process.
type vaultOperations interface {
	List() ([]vault.Entry, error)
	Read(string) (vault.Document, error)
	Save(string, string, string) (vault.Document, error)
	Trash(string, string) (vault.RecoveryRecord, error)
	Move(string, string, string) error
}

type desktopVault struct {
	client       *nativehost.RelayClient
	directory    string
	session      string
	needsUnlock  bool
	deviceUnlock bool
}

// Only the initial, read-only handshake may choose the independent backend.
// Existing installations without a CLI endpoint continue working offline.
func (c *command) tryDesktop(store *app.CLIVault) (*desktopVault, error) {
	remote := &desktopVault{
		client:    nativehost.NewRelayClientWithFrameLimit(clisession.Directory(c.dataDir), clisession.MaxFrame),
		directory: store.Info.Directory,
	}
	out, err := remote.send(clisession.Request{Action: "status"})
	if err != nil {
		// A locked matching desktop provides an unlock challenge. Authenticate
		// there so the desktop and subsequent CLI commands share the new session.
		if errors.Is(err, vault.ErrLocked) && out.CanUnlock && len(out.Session) == 64 {
			remote.session = out.Session
			remote.needsUnlock = true
			remote.deviceUnlock = out.DeviceUnlock
			return remote, nil
		}
		remote.client.Close()
		if errors.Is(err, vault.ErrLocked) || out.Code == "invalid_request" || out.Code == "cli_protocol_mismatch" {
			// A replaced executable may still be talking to an older desktop
			// process. Do not silently recreate an independent unlock session.
			return nil, c.fail("桌面客户端尚不支持共享解锁，请重启更新后的客户端再重试。")
		}
		// These responses explicitly mean no unlocked matching desktop vault
		// was selected. Transport errors after selection never reach this path.
		switch out.Code {
		case "app_unavailable", "vault_changed", "not_configured", "unsupported_action", "protocol_mismatch", "transport_error":
			return nil, nil
		}
		return nil, err
	}
	if out.Session == "" {
		remote.client.Close()
		return nil, clisession.ErrOperation
	}
	remote.session = out.Session
	return remote, nil
}

func (c *command) unlockDesktop(remote *desktopVault, name string) error {
	if !remote.needsUnlock {
		return nil
	}
	if remote.deviceUnlock {
		if err := remote.Unlock("device", ""); err == nil {
			return nil
		} else if !errors.Is(err, clisession.ErrAuthentication) && !errors.Is(err, vault.ErrPassphrase) {
			return err
		}
		fmt.Fprintln(c.stderr, c.t("系统解锁未完成，请改用私钥口令。"))
	}
	return c.unlockPassphrase(name, func(passphrase string) error {
		return remote.Unlock("passphrase", passphrase)
	})
}

func (v *desktopVault) Unlock(mode, passphrase string) error {
	secret := []byte(passphrase)
	defer clear(secret)
	out, err := v.send(clisession.Request{Action: "unlock", Mode: mode, Passphrase: secret})
	if err != nil {
		return err
	}
	if len(out.Session) != 64 {
		return clisession.ErrOperation
	}
	v.session = out.Session
	v.needsUnlock = false
	v.deviceUnlock = false
	return nil
}

func (v *desktopVault) send(request clisession.Request) (clisession.Response, error) {
	request.Protocol = clisession.Protocol
	request.PayloadVersion = clisession.PayloadVersion
	request.Directory = v.directory
	request.Session = v.session
	// JSON strings normalize invalid UTF-8. Reject filesystem metadata before
	// encoding so an entry or directory can never silently name another path.
	if !utf8.ValidString(request.Directory) || !utf8.ValidString(request.Path) || !utf8.ValidString(request.Destination) {
		return clisession.Response{}, vault.ErrPath
	}
	b, err := json.Marshal(request)
	if err != nil {
		return clisession.Response{}, clisession.ErrOperation
	}
	defer clear(b)
	response := v.client.Send(b)
	defer clear(response)
	var out clisession.Response
	if json.Unmarshal(response, &out) != nil || out.Protocol != clisession.Protocol {
		if out.Document != nil {
			clear(out.Document.Raw)
		}
		return clisession.Response{}, clisession.ErrOperation
	}
	if out.Status != "ok" {
		if out.Document != nil {
			clear(out.Document.Raw)
			out.Document = nil
		}
		// Losing a mutation's response has an uncertain outcome. Report it;
		// neither reconnect nor unlock may replay the request automatically.
		if out.Code == "transport_error" {
			return out, clisession.ErrUnavailable
		}
		return out, clisession.Error(out.Code)
	}
	return out, nil
}

func (v *desktopVault) List() ([]vault.Entry, error) {
	out, err := v.send(clisession.Request{Action: "list"})
	return out.Entries, err
}

func (v *desktopVault) document(request clisession.Request) (vault.Document, error) {
	out, err := v.send(request)
	if out.Document != nil {
		defer clear(out.Document.Raw)
	}
	if err != nil {
		return vault.Document{}, err
	}
	if out.Document == nil || out.Document.Path != request.Path {
		return vault.Document{}, clisession.ErrOperation
	}
	return vault.ParseDocument(out.Document.Path, string(out.Document.Raw), out.Document.Revision), nil
}

func (v *desktopVault) Read(name string) (vault.Document, error) {
	return v.document(clisession.Request{Action: "read", Path: name})
}

func (v *desktopVault) Save(name, raw, revision string) (vault.Document, error) {
	contents := []byte(raw)
	defer clear(contents)
	return v.document(clisession.Request{Action: "save", Path: name, Contents: contents, Revision: revision})
}

func (v *desktopVault) Trash(name, revision string) (vault.RecoveryRecord, error) {
	out, err := v.send(clisession.Request{Action: "trash", Path: name, Revision: revision})
	return vault.RecoveryRecord{ID: out.RecoveryID}, err
}

func (v *desktopVault) Move(from, to, revision string) error {
	_, err := v.send(clisession.Request{Action: "move", Path: from, Destination: to, Revision: revision})
	return err
}
