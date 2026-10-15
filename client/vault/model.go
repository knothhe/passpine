// Package vault implements a pass-compatible OpenPGP password store in Go.
// Secret material is never logged or written to temporary plaintext files.
package vault

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	ErrLocked        = errors.New("vault is locked")
	ErrCancelled     = errors.New("session changed; operation cancelled")
	ErrConflict      = errors.New("entry changed since it was opened")
	ErrExists        = errors.New("destination already exists")
	ErrPath          = errors.New("invalid or unsafe path")
	ErrPassphrase    = errors.New("incorrect passphrase or unusable private key")
	ErrNotConfigured = errors.New("vault has not been configured")
	ErrBusy          = errors.New("vault is busy in another process")
)

type Config struct {
	Directory         string        `json:"directory"`
	KeyFile           string        `json:"keyFile"`
	RecoveryDirectory string        `json:"recoveryDirectory"`
	IdleTimeout       time.Duration `json:"-"`
	MaximumSession    time.Duration `json:"-"`
}

type Identity struct {
	Fingerprint string   `json:"fingerprint"`
	Names       []string `json:"names"`
	HasSecret   bool     `json:"hasSecret"`
}

type Entry struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Folder   string    `json:"folder"`
	Modified time.Time `json:"modified"`
}

type Field struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Document struct {
	Path     string  `json:"path"`
	Raw      string  `json:"raw"`
	Password string  `json:"password"`
	Username string  `json:"username"`
	URL      string  `json:"url"`
	Notes    string  `json:"notes"`
	Fields   []Field `json:"fields"`
	Revision string  `json:"revision"`
	OTP      string  `json:"otp,omitempty"`
}

type Status struct {
	Configured       bool   `json:"configured"`
	Unlocked         bool   `json:"unlocked"`
	Directory        string `json:"directory"`
	RemainingSeconds int    `json:"remainingSeconds"`
}

// ValidatePath accepts portable relative entry names, without their .gpg suffix.
func ValidatePath(name string) error {
	if name == "" || len(name) > 1024 || strings.HasSuffix(name, ".gpg") {
		return ErrPath
	}
	for _, c := range name {
		if unicode.IsControl(c) || c == '\\' || c == ':' || c == '\u2028' || c == '\u2029' {
			return ErrPath
		}
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasPrefix(part, "-") || strings.TrimSpace(part) != part || strings.HasSuffix(part, ".") || len(part) > 240 {
			return ErrPath
		}
		stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || (len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9') {
			return ErrPath
		}
	}
	return nil
}

func ParseDocument(name, raw, revision string) Document {
	d := Document{Path: name, Raw: raw, Revision: revision, Fields: []Field{}}
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	d.Password = lines[0]
	notes := []string{}
	for _, line := range lines[1:] {
		key, value, ok := strings.Cut(line, ":")
		valid := ok && key != "" && strings.IndexFunc(key, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' }) < 0
		if !valid {
			notes = append(notes, line)
			continue
		}
		value = strings.TrimPrefix(value, " ")
		d.Fields = append(d.Fields, Field{Key: key, Value: value})
		switch strings.ToLower(key) {
		case "note", "notes":
			notes = append(notes, value)
		case "user", "username", "login", "email":
			if d.Username == "" {
				d.Username = value
			}
		case "url", "website", "site":
			if d.URL == "" {
				d.URL = value
			}
		case "otp", "totp", "otpauth":
			if d.OTP == "" {
				d.OTP = value
			}
		}
	}
	d.Notes = strings.TrimSpace(strings.Join(notes, "\n"))
	return d
}

func Search(entries []Entry, query string) []Entry {
	query = strings.ToLower(strings.TrimSpace(query))
	type match struct {
		e     Entry
		score int
	}
	matches := []match{}
	for _, e := range entries {
		name := []rune(strings.ToLower(e.Path))
		cursor, score := 0, 0
		found := true
		for _, c := range query {
			i := cursor
			for i < len(name) && name[i] != c {
				i++
			}
			if i == len(name) {
				found = false
				break
			}
			score += i - cursor
			cursor = i + 1
		}
		if found {
			if query != "" && strings.HasPrefix(strings.ToLower(path.Base(e.Path)), query) {
				score -= 100
			}
			matches = append(matches, match{e, score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score == matches[j].score {
			return matches[i].e.Path < matches[j].e.Path
		}
		return matches[i].score < matches[j].score
	})
	out := make([]Entry, len(matches))
	for i, m := range matches {
		out[i] = m.e
	}
	return out
}

type PasswordOptions struct {
	Length           int  `json:"length"`
	Lowercase        bool `json:"lowercase"`
	Uppercase        bool `json:"uppercase"`
	Numbers          bool `json:"numbers"`
	Symbols          bool `json:"symbols"`
	ExcludeAmbiguous bool `json:"excludeAmbiguous"`
}

func DefaultPasswordOptions() PasswordOptions {
	return PasswordOptions{Length: 24, Lowercase: true, Uppercase: true, Numbers: true, Symbols: true, ExcludeAmbiguous: true}
}

func GeneratePassword(o PasswordOptions) (string, error) {
	if o.Length < 4 || o.Length > 256 {
		return "", errors.New("password length must be between 4 and 256")
	}
	groups := []string{}
	for _, g := range []struct {
		enabled bool
		chars   string
	}{{o.Lowercase, "abcdefghijklmnopqrstuvwxyz"}, {o.Uppercase, "ABCDEFGHIJKLMNOPQRSTUVWXYZ"}, {o.Numbers, "0123456789"}, {o.Symbols, "!@#$%^&*()-_=+[]{};:,.?/"}} {
		if !g.enabled {
			continue
		}
		chars := g.chars
		if o.ExcludeAmbiguous {
			chars = strings.Map(func(r rune) rune {
				if strings.ContainsRune("Il1O0o", r) {
					return -1
				}
				return r
			}, chars)
		}
		groups = append(groups, chars)
	}
	if len(groups) == 0 {
		return "", errors.New("select at least one character group")
	}
	choose := func(n int) (int, error) {
		v, e := rand.Int(rand.Reader, big.NewInt(int64(n)))
		if e != nil {
			return 0, e
		}
		return int(v.Int64()), nil
	}
	alphabet := strings.Join(groups, "")
	out := make([]byte, o.Length)
	for i := range out {
		g := alphabet
		if i < len(groups) {
			g = groups[i]
		}
		j, e := choose(len(g))
		if e != nil {
			return "", e
		}
		out[i] = g[j]
	}
	for i := len(out) - 1; i > 0; i-- {
		j, e := choose(i + 1)
		if e != nil {
			return "", e
		}
		out[i], out[j] = out[j], out[i]
	}
	return string(out), nil
}

type OTPCode struct {
	Code      string `json:"code"`
	Remaining int    `json:"remaining"`
	Period    int    `json:"period"`
}

// TOTP implements RFC 6238, including SHA-256/512 and otpauth:// URIs.
func TOTP(secret string, now time.Time) (OTPCode, error) {
	digits, period, algorithm := 6, 30, "SHA1"
	if strings.HasPrefix(strings.ToLower(secret), "otpauth://") {
		u, e := url.Parse(secret)
		if e != nil || u.Host != "totp" {
			return OTPCode{}, errors.New("only TOTP otpauth URIs are supported")
		}
		q := u.Query()
		secret = q.Get("secret")
		if q.Has("digits") {
			digits, e = strconv.Atoi(q.Get("digits"))
			if e != nil {
				return OTPCode{}, errors.New("invalid OTP digits")
			}
		}
		if q.Has("period") {
			period, e = strconv.Atoi(q.Get("period"))
			if e != nil {
				return OTPCode{}, errors.New("invalid OTP period")
			}
		}
		if q.Has("algorithm") {
			algorithm = strings.ToUpper(q.Get("algorithm"))
		}
	}
	if (digits != 6 && digits != 8) || period < 1 || period > 300 || now.Unix() < 0 {
		return OTPCode{}, errors.New("invalid OTP parameters")
	}
	var newHash func() hash.Hash
	switch algorithm {
	case "SHA1":
		newHash = sha1.New
	case "SHA256":
		newHash = sha256.New
	case "SHA512":
		newHash = sha512.New
	default:
		return OTPCode{}, errors.New("unsupported OTP algorithm")
	}
	secret = strings.ToUpper(strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""), "="))
	key, e := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if e != nil || len(key) == 0 {
		return OTPCode{}, errors.New("invalid OTP secret")
	}
	defer wipe(key)
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(now.Unix()/int64(period)))
	mac := hmac.New(newHash, key)
	_, _ = mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := int(sum[len(sum)-1] & 15)
	number := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	mod := uint32(1000000)
	if digits == 8 {
		mod = 100000000
	}
	return OTPCode{Code: fmt.Sprintf("%0*d", digits, number%mod), Remaining: period - int(now.Unix()%int64(period)), Period: period}, nil
}

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
