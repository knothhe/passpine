package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/knothhe/passpine/client/cloudsync"
)

const MaxConfigSyncTransferBytes = 64 << 10

// ErrInvalidConfigSyncTransfer deliberately contains no file contents or
// connection details. The desktop supplies the localized recovery message.
var ErrInvalidConfigSyncTransfer = errors.New("invalid configuration sync transfer")

type configSyncTransfer struct {
	Kind       string            `json:"kind"`
	Version    int               `json:"version"`
	Options    ConfigSyncOptions `json:"options"`
	Connection *cloudsync.Config `json:"connection,omitempty"`
}

// EncodeConfigSyncTransfer creates a portable settings file. Its schema can
// carry only local sync options and a public object-storage target, never
// credentials, authorization state, merge baselines or application settings.
func EncodeConfigSyncTransfer(options ConfigSyncOptions, connection *cloudsync.Config) ([]byte, error) {
	if !validConfigSyncOptions(options) {
		return nil, ErrInvalidConfigSyncTransfer
	}
	var target *cloudsync.Config
	if connection != nil {
		normalized, err := cloudsync.NormalizeConfig(*connection)
		if err != nil || !validTransferConfigStrings(normalized) {
			return nil, ErrInvalidConfigSyncTransfer
		}
		target = &normalized
	}
	data, err := json.MarshalIndent(configSyncTransfer{Kind: "passpine-config-sync", Version: 1, Options: options, Connection: target}, "", "  ")
	if err != nil || len(data)+1 > MaxConfigSyncTransferBytes {
		return nil, ErrInvalidConfigSyncTransfer
	}
	return append(data, '\n'), nil
}

// DecodeConfigSyncTransfer validates the entire file before returning anything
// to the editor. Every v1 option is required, so malformed or partial imports
// cannot silently turn on synchronization through local-file migration defaults.
// An absent connection preserves the current connection draft at the call site.
func DecodeConfigSyncTransfer(data []byte) (ConfigSyncOptions, *cloudsync.Config, error) {
	invalid := func() (ConfigSyncOptions, *cloudsync.Config, error) {
		return ConfigSyncOptions{}, nil, ErrInvalidConfigSyncTransfer
	}
	if len(data) == 0 || len(data) > MaxConfigSyncTransferBytes || !utf8.Valid(data) {
		return invalid()
	}
	root, err := configSyncTransferObject(data, []string{"kind", "version", "options", "connection"}, "kind", "version", "options")
	if err != nil {
		return invalid()
	}
	var kind string
	var version int
	if json.Unmarshal(root["kind"], &kind) != nil || kind != "passpine-config-sync" || json.Unmarshal(root["version"], &version) != nil || version != 1 {
		return invalid()
	}
	optionNames := []string{"mode", "intervalMinutes", "syncOnChange", "syncOnStart", "notifyErrors", "desktop", "browser", "desktopFields", "browserFields"}
	fields, err := configSyncTransferObject(root["options"], optionNames, optionNames...)
	if err != nil {
		return invalid()
	}
	selectionNames := []string{"appearance", "fontScale", "language"}
	for _, name := range []string{"desktopFields", "browserFields"} {
		if _, err = configSyncTransferObject(fields[name], selectionNames, selectionNames...); err != nil {
			return invalid()
		}
	}
	// Avoid ConfigSyncOptions.UnmarshalJSON, whose defaults intentionally serve
	// older local metadata rather than an externally supplied transfer file.
	type exactOptions ConfigSyncOptions
	var options exactOptions
	if json.Unmarshal(root["options"], &options) != nil || !validConfigSyncOptions(ConfigSyncOptions(options)) {
		return invalid()
	}
	var target *cloudsync.Config
	if raw, exists := root["connection"]; exists {
		connectionNames := []string{"endpoint", "region", "bucket", "prefix", "pathStyle"}
		if _, err = configSyncTransferObject(raw, connectionNames, connectionNames...); err != nil {
			return invalid()
		}
		var config cloudsync.Config
		if json.Unmarshal(raw, &config) != nil {
			return invalid()
		}
		normalized, normalizeErr := cloudsync.NormalizeConfig(config)
		if normalizeErr != nil {
			return invalid()
		}
		target = &normalized
	}
	return ConfigSyncOptions(options), target, nil
}

// Parse one schema object with exact names and no duplicate or null values.
// Nested schema objects are checked separately before typed decoding.
func configSyncTransferObject(data []byte, allowed []string, required ...string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, ErrInvalidConfigSyncTransfer
	}
	allowedNames := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		allowedNames[name] = true
	}
	fields := make(map[string]json.RawMessage, len(allowed))
	for decoder.More() {
		token, err := decoder.Token()
		name, isName := token.(string)
		if err != nil || !isName || !allowedNames[name] {
			return nil, ErrInvalidConfigSyncTransfer
		}
		if _, exists := fields[name]; exists {
			return nil, ErrInvalidConfigSyncTransfer
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, ErrInvalidConfigSyncTransfer
		}
		fields[name] = value
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return nil, ErrInvalidConfigSyncTransfer
	}
	var extra json.RawMessage
	if decoder.Decode(&extra) != io.EOF {
		return nil, ErrInvalidConfigSyncTransfer
	}
	for _, name := range required {
		if _, exists := fields[name]; !exists {
			return nil, ErrInvalidConfigSyncTransfer
		}
	}
	return fields, nil
}

func validTransferConfigStrings(config cloudsync.Config) bool {
	return utf8.ValidString(config.Endpoint) && utf8.ValidString(config.Region) && utf8.ValidString(config.Bucket) && utf8.ValidString(config.Prefix)
}
