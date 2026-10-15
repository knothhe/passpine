package vault

import (
	"bytes"
	"crypto"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

func cryptoConfig() *packet.Config {
	return &packet.Config{DefaultCipher: packet.CipherAES256, DefaultHash: crypto.SHA256, Algorithm: packet.PubKeyAlgoEdDSA, Curve: packet.Curve25519}
}

func parseKeys(data []byte) (openpgp.EntityList, error) {
	if len(data) == 0 || len(data) > maxFileSize {
		return nil, errors.New("invalid key file")
	}
	var all openpgp.EntityList
	if bytes.Contains(data, []byte("-----BEGIN PGP ")) {
		for len(data) > 0 {
			start := bytes.Index(data, []byte("-----BEGIN PGP "))
			if start < 0 {
				break
			}
			data = data[start:]
			end := bytes.Index(data, []byte("-----END PGP "))
			if end < 0 {
				return nil, errors.New("incomplete armored key")
			}
			tail := bytes.Index(data[end+5:], []byte("-----"))
			if tail < 0 {
				return nil, errors.New("incomplete armored key")
			}
			end += 5 + tail + 5
			keys, e := openpgp.ReadArmoredKeyRing(bytes.NewReader(data[:end]))
			if e != nil {
				return nil, fmt.Errorf("read OpenPGP keys: %w", e)
			}
			all = append(all, keys...)
			data = data[end:]
		}
	} else {
		keys, e := openpgp.ReadKeyRing(bytes.NewReader(data))
		if e != nil {
			return nil, fmt.Errorf("read OpenPGP keys: %w", e)
		}
		all = keys
	}
	if len(all) == 0 {
		return nil, errors.New("no OpenPGP keys found")
	}
	// A legacy export can include both public and secret packets for one identity.
	unique := openpgp.EntityList{}
	indices := map[string]int{}
	for _, entity := range all {
		fp := fingerprint(entity)
		if i, ok := indices[fp]; ok {
			if hasSecret(entity) {
				unique[i] = entity
			}
			continue
		}
		indices[fp] = len(unique)
		unique = append(unique, entity)
	}
	return unique, nil
}

func fingerprint(e *openpgp.Entity) string {
	return strings.ToUpper(hex.EncodeToString(e.PrimaryKey.Fingerprint))
}

func hasSecret(e *openpgp.Entity) bool {
	if e.PrivateKey != nil && !e.PrivateKey.Dummy() {
		return true
	}
	for _, s := range e.Subkeys {
		if s.PrivateKey != nil && !s.PrivateKey.Dummy() {
			return true
		}
	}
	return false
}

func identities(keys openpgp.EntityList) []Identity {
	out := make([]Identity, 0, len(keys))
	for _, e := range keys {
		names := []string{}
		for name := range e.Identities {
			names = append(names, name)
		}
		sort.Strings(names)
		out = append(out, Identity{fingerprint(e), names, hasSecret(e)})
	}
	return out
}

func decryptKeys(keys openpgp.EntityList, passphrase string, requireProtected bool) error {
	if passphrase == "" {
		return ErrPassphrase
	}
	pass := []byte(passphrase)
	defer wipe(pass)
	found := false
	for _, e := range keys {
		if !hasSecret(e) {
			continue
		}
		found = true
		if requireProtected {
			if e.PrivateKey != nil && !e.PrivateKey.Dummy() && !e.PrivateKey.Encrypted {
				return errors.New("private key is not passphrase protected")
			}
			for _, s := range e.Subkeys {
				if s.PrivateKey != nil && !s.PrivateKey.Dummy() && !s.PrivateKey.Encrypted {
					return errors.New("private subkey is not passphrase protected")
				}
			}
		}
		if e.DecryptPrivateKeys(pass) != nil {
			return ErrPassphrase
		}
	}
	if !found {
		return errors.New("no usable private key found")
	}
	return nil
}

func protectedKeyData(keys openpgp.EntityList, passphrase string) ([]byte, error) {
	var out bytes.Buffer
	w, e := armor.Encode(&out, openpgp.PrivateKeyType, nil)
	if e != nil {
		return nil, e
	}
	pass := []byte(passphrase)
	defer wipe(pass)
	for _, key := range keys {
		if hasSecret(key) {
			if e = key.EncryptPrivateKeys(pass, cryptoConfig()); e != nil {
				return nil, e
			}
			e = serializeKeyPackets(w, key)
		} else {
			e = key.Serialize(w)
		}
		if e != nil {
			return nil, e
		}
	}
	if e = w.Close(); e != nil {
		return nil, e
	}
	return out.Bytes(), nil
}

// Serialize existing signatures unchanged and retain public-only subkeys. Some
// transferable GnuPG exports have private encryption subkeys but no private
// signing subkey; Entity.SerializePrivateWithoutSigning assumes every subkey
// has a PrivateKey and would panic for that otherwise valid mixed keyring.
func serializeKeyPackets(w io.Writer, key *openpgp.Entity) error {
	type serializable interface{ Serialize(io.Writer) error }
	packets := []serializable{}
	if key.PrivateKey != nil {
		packets = append(packets, key.PrivateKey)
	} else {
		packets = append(packets, key.PrimaryKey)
	}
	for _, signature := range key.Revocations {
		packets = append(packets, signature)
	}
	for _, signature := range key.Signatures {
		packets = append(packets, signature)
	}
	names := make([]string, 0, len(key.Identities))
	for name := range key.Identities {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		identity := key.Identities[name]
		packets = append(packets, identity.UserId)
		for _, signature := range identity.Signatures {
			packets = append(packets, signature)
		}
	}
	for _, subkey := range key.Subkeys {
		if subkey.PrivateKey != nil {
			packets = append(packets, subkey.PrivateKey)
		} else {
			packets = append(packets, subkey.PublicKey)
		}
		for _, signature := range subkey.Revocations {
			packets = append(packets, signature)
		}
		packets = append(packets, subkey.Sig)
	}
	for _, packet := range packets {
		if e := packet.Serialize(w); e != nil {
			return e
		}
	}
	return nil
}

func encrypt(raw []byte, keys []*openpgp.Entity) ([]byte, error) {
	if len(keys) == 0 {
		return nil, errors.New("no recipients configured")
	}
	if len(raw) > maxPlaintextSize {
		return nil, errors.New("entry exceeds size limit")
	}
	var encrypted bytes.Buffer
	w, e := openpgp.Encrypt(&encrypted, keys, nil, &openpgp.FileHints{IsBinary: true}, cryptoConfig())
	if e != nil {
		return nil, fmt.Errorf("encrypt entry: %w", e)
	}
	if _, e = w.Write(raw); e != nil {
		return nil, e
	}
	if e = w.Close(); e != nil {
		return nil, e
	}
	return encrypted.Bytes(), nil
}

func decrypt(data []byte, keys openpgp.EntityList) ([]byte, error) {
	var r io.Reader = bytes.NewReader(data)
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("-----BEGIN PGP MESSAGE-----")) {
		a, e := armor.Decode(r)
		if e != nil {
			return nil, e
		}
		r = a.Body
	}
	message, e := openpgp.ReadMessage(r, keys, nil, cryptoConfig())
	if e != nil {
		return nil, fmt.Errorf("decrypt entry: %w", e)
	}
	if !message.IsEncrypted {
		return nil, errors.New("entry is not encrypted")
	}
	plain, e := io.ReadAll(io.LimitReader(message.UnverifiedBody, maxPlaintextSize+1))
	// Reading through EOF is essential: it verifies the MDC / authenticated data.
	if e != nil {
		wipe(plain)
		return nil, fmt.Errorf("verify encrypted entry: %w", e)
	}
	if len(plain) > maxPlaintextSize {
		wipe(plain)
		return nil, errors.New("entry exceeds size limit")
	}
	if message.IsSigned && message.SignatureError != nil {
		wipe(plain)
		return nil, fmt.Errorf("verify entry signature: %w", message.SignatureError)
	}
	return plain, nil
}

// A transferable key may include a newer public encryption subkey while only
// retaining an older private subkey. Verify the actual ciphertext before any
// write can replace an entry or a trash operation can remove its source.
func encryptRecoverable(raw []byte, recipients []*openpgp.Entity, keys openpgp.EntityList) ([]byte, error) {
	data, e := encrypt(raw, recipients)
	if e != nil {
		return nil, e
	}
	verified, e := decrypt(data, keys)
	defer wipe(verified)
	if e != nil {
		return nil, errors.New("the selected recipients cannot be decrypted with the available private keys")
	}
	if !bytes.Equal(raw, verified) {
		return nil, errors.New("encrypted entry failed its recovery verification")
	}
	return data, nil
}

func resolveRecipients(names []string, keys openpgp.EntityList) ([]*openpgp.Entity, error) {
	out := []*openpgp.Entity{}
	seen := map[string]bool{}
	for _, name := range names {
		lookup := strings.TrimPrefix(strings.ToUpper(name), "0X")
		var matched *openpgp.Entity
		for _, e := range keys {
			fp := fingerprint(e)
			matches := lookup == fp || (len(lookup) == 16 && strings.HasSuffix(fp, lookup))
			for _, s := range e.Subkeys {
				sub := strings.ToUpper(hex.EncodeToString(s.PublicKey.Fingerprint))
				matches = matches || lookup == sub || (len(lookup) == 16 && strings.HasSuffix(sub, lookup))
			}
			for id, identity := range e.Identities {
				matches = matches || id == name || strings.EqualFold(identity.UserId.Email, name)
			}
			if matches {
				if matched != nil && fingerprint(matched) != fp {
					return nil, errors.New("ambiguous recipient")
				}
				matched = e
			}
		}
		if matched == nil {
			return nil, fmt.Errorf("missing public key for recipient %s", name)
		}
		if _, ok := matched.EncryptionKey(time.Now()); !ok {
			return nil, fmt.Errorf("recipient %s has no current encryption key", name)
		}
		fp := fingerprint(matched)
		if !seen[fp] {
			seen[fp] = true
			out = append(out, matched)
		}
	}
	return out, nil
}
