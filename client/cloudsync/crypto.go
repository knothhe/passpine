package cloudsync

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"io"

	"golang.org/x/crypto/scrypt"
)

const (
	envelopeFormat = "passpine-sync-v1"
	scryptN        = 32768
	scryptR        = 8
	scryptP        = 1
)

type envelope struct {
	Format     string `json:"format"`
	Salt       []byte `json:"salt"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

type document struct {
	Schema int    `json:"schema"`
	Values Values `json:"values"`
}

func aeadFor(passphrase string, salt []byte) (cipher.AEAD, error) {
	key, err := scrypt.Key([]byte(passphrase), salt, scryptN, scryptR, scryptP, 32)
	if err != nil {
		return nil, &Error{Code: "invalid_document"}
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, &Error{Code: "invalid_document"}
	}
	return cipher.NewGCM(block)
}

func seal(values Values, passphrase, identity string) ([]byte, error) {
	if err := validateValues(values); err != nil {
		return nil, err
	}
	plain, err := json.Marshal(document{Schema: 1, Values: values})
	if err != nil {
		return nil, &Error{Code: "invalid_document"}
	}
	defer clear(plain)
	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, &Error{Code: "invalid_document"}
	}
	aead, err := aeadFor(passphrase, salt)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, &Error{Code: "invalid_document"}
	}
	sealed := aead.Seal(nil, nonce, plain, []byte(envelopeFormat+"\x00"+identity))
	return json.Marshal(envelope{Format: envelopeFormat, Salt: salt, Nonce: nonce, Ciphertext: sealed})
}

func openEnvelope(encoded []byte, passphrase, identity string) (Values, error) {
	if len(encoded) > MaxDocumentBytes*2 {
		return nil, &Error{Code: "invalid_document"}
	}
	var env envelope
	dec := json.NewDecoder(bytes.NewReader(encoded))
	dec.DisallowUnknownFields()
	if dec.Decode(&env) != nil || dec.Decode(new(any)) != io.EOF || env.Format != envelopeFormat || len(env.Salt) != 32 || len(env.Nonce) != 12 || len(env.Ciphertext) < 16 || len(env.Ciphertext) > MaxDocumentBytes+1024 {
		return nil, &Error{Code: "invalid_document"}
	}
	aead, err := aeadFor(passphrase, env.Salt)
	if err != nil {
		return nil, err
	}
	plain, err := aead.Open(nil, env.Nonce, env.Ciphertext, []byte(envelopeFormat+"\x00"+identity))
	if err != nil {
		return nil, &Error{Code: "decrypt_failed"}
	}
	defer clear(plain)
	var doc document
	dec = json.NewDecoder(bytes.NewReader(plain))
	dec.DisallowUnknownFields()
	if dec.Decode(&doc) != nil || dec.Decode(new(any)) != io.EOF || doc.Schema != 1 || doc.Values == nil {
		return nil, &Error{Code: "invalid_document"}
	}
	if err := validateValues(doc.Values); err != nil {
		return nil, err
	}
	return doc.Values, nil
}
