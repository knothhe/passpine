package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/knothhe/passpine/client/cloudsync"
)

func transferFixture(t *testing.T) (ConfigSyncOptions, cloudsync.Config, []byte) {
	t.Helper()
	options := DefaultConfigSyncOptions()
	options.Mode, options.IntervalMinutes = "manual", 30
	options.SyncOnChange, options.SyncOnStart, options.NotifyErrors = false, false, false
	options.BrowserFields = SyncFieldSelection{Language: true}
	config := cloudsync.Config{Endpoint: "https://account.r2.cloudflarestorage.com", Region: "auto", Bucket: "personal-settings", Prefix: "passpine/偏好", PathStyle: true}
	encoded, err := EncodeConfigSyncTransfer(options, &config)
	if err != nil {
		t.Fatal(err)
	}
	return options, config, encoded
}

func TestConfigSyncTransferRoundTripPreservesExplicitChoicesAndNormalizesTarget(t *testing.T) {
	options, config, encoded := transferFixture(t)
	decoded, target, err := DecodeConfigSyncTransfer(encoded)
	if err != nil || decoded != options || target == nil || *target != config {
		t.Fatalf("transfer round trip changed choices or target: %+v %+v %v", decoded, target, err)
	}
	config.Endpoint, config.Region, config.Prefix = "  https://storage.example.test/  ", "", "/passpine/"
	before := config
	encoded, err = EncodeConfigSyncTransfer(options, &config)
	if err != nil || config != before {
		t.Fatal("export mutated its connection input", err)
	}
	_, target, err = DecodeConfigSyncTransfer(encoded)
	if err != nil || target.Endpoint != "https://storage.example.test" || target.Region != "auto" || target.Prefix != "passpine" {
		t.Fatal("export did not normalize portable target values", err)
	}
	// All false field choices remain intentional rather than inheriting defaults.
	options.Desktop, options.Browser = false, false
	options.DesktopFields, options.BrowserFields = SyncFieldSelection{}, SyncFieldSelection{}
	encoded, err = EncodeConfigSyncTransfer(options, nil)
	if err != nil {
		t.Fatal(err)
	}
	decoded, target, err = DecodeConfigSyncTransfer(encoded)
	if err != nil || decoded != options || target != nil || bytes.Contains(encoded, []byte(`"connection"`)) {
		t.Fatal("options-only transfer changed explicit false choices or invented a connection", err)
	}
}

func TestConfigSyncTransferExportsOnlyPortableSchema(t *testing.T) {
	_, _, encoded := transferFixture(t)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 4 || string(fields["kind"]) != `"passpine-config-sync"` || string(fields["version"]) != "1" || fields["options"] == nil || fields["connection"] == nil {
		t.Fatal("export is missing its identity or contains unexpected top-level fields")
	}
	var connection map[string]json.RawMessage
	if err := json.Unmarshal(fields["connection"], &connection); err != nil {
		t.Fatal(err)
	}
	if len(connection) != 5 {
		t.Fatal("connection export contains state beyond its five public target fields")
	}
	for _, key := range []string{"endpoint", "region", "bucket", "prefix", "pathStyle"} {
		if _, exists := connection[key]; !exists {
			t.Errorf("connection export omitted %s", key)
		}
	}
	for _, private := range []string{"secrets", "accessKeyId", "secretAccessKey", "sessionToken", "passphrase", "secretRef", "baseline", "accountId", "clientId", "connectionId", "lastSyncedAt"} {
		if bytes.Contains(encoded, []byte(`"`+private+`"`)) {
			t.Errorf("export included private field %s", private)
		}
	}
}

func TestConfigSyncTransferRejectsUnknownFieldsAtEveryLevel(t *testing.T) {
	_, _, encoded := transferFixture(t)
	for _, location := range []string{"root", "options", "desktopFields", "browserFields", "connection"} {
		t.Run(location, func(t *testing.T) {
			var root map[string]any
			if err := json.Unmarshal(encoded, &root); err != nil {
				t.Fatal(err)
			}
			selected := root
			if location == "desktopFields" || location == "browserFields" {
				selected = root["options"].(map[string]any)[location].(map[string]any)
			} else if location != "root" {
				selected = root[location].(map[string]any)
			}
			selected["secretAccessKey"] = "private-import-material"
			data, err := json.Marshal(root)
			if err != nil {
				t.Fatal(err)
			}
			assertInvalidSyncTransfer(t, data)
		})
	}
}

func TestConfigSyncTransferRequiresCompleteTypedOptions(t *testing.T) {
	_, _, encoded := transferFixture(t)
	mutations := map[string]func(map[string]any){
		"missing options": func(root map[string]any) { delete(root, "options") },
		"null options":    func(root map[string]any) { root["options"] = nil },
		"empty options":   func(root map[string]any) { root["options"] = map[string]any{} },
		"mode missing": func(root map[string]any) {
			delete(root["options"].(map[string]any), "mode")
		},
		"unknown mode": func(root map[string]any) { root["options"].(map[string]any)["mode"] = "force" },
		"unsupported interval": func(root map[string]any) {
			root["options"].(map[string]any)["intervalMinutes"] = 2
		},
		"null boolean": func(root map[string]any) { root["options"].(map[string]any)["syncOnStart"] = nil },
		"string boolean": func(root map[string]any) {
			root["options"].(map[string]any)["syncOnStart"] = "false"
		},
		"wrong field casing": func(root map[string]any) {
			fields := root["options"].(map[string]any)
			fields["Mode"] = fields["mode"]
			delete(fields, "mode")
		},
		"null selection": func(root map[string]any) { root["options"].(map[string]any)["desktopFields"] = nil },
		"array selection": func(root map[string]any) {
			root["options"].(map[string]any)["desktopFields"] = []bool{true, false}
		},
		"incomplete selection": func(root map[string]any) {
			delete(root["options"].(map[string]any)["browserFields"].(map[string]any), "appearance")
		},
		"wrong selection type": func(root map[string]any) {
			root["options"].(map[string]any)["browserFields"].(map[string]any)["language"] = 0
		},
		"wrong kind":      func(root map[string]any) { root["kind"] = "passpine-vault" },
		"future version":  func(root map[string]any) { root["version"] = 2 },
		"null connection": func(root map[string]any) { root["connection"] = nil },
		"missing connection field": func(root map[string]any) {
			delete(root["connection"].(map[string]any), "pathStyle")
		},
		"credential endpoint": func(root map[string]any) {
			root["connection"].(map[string]any)["endpoint"] = "https://user:password@storage.example.test"
		},
		"endpoint query": func(root map[string]any) {
			root["connection"].(map[string]any)["endpoint"] = "https://storage.example.test?token=private"
		},
		"plain remote HTTP": func(root map[string]any) {
			root["connection"].(map[string]any)["endpoint"] = "http://storage.example.test"
		},
		"invalid bucket": func(root map[string]any) {
			root["connection"].(map[string]any)["bucket"] = "../private"
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			var root map[string]any
			if err := json.Unmarshal(encoded, &root); err != nil {
				t.Fatal(err)
			}
			mutate(root)
			data, err := json.Marshal(root)
			if err != nil {
				t.Fatal(err)
			}
			assertInvalidSyncTransfer(t, data)
		})
	}
}

func TestConfigSyncTransferRejectsDuplicatesTrailingDataAndOversizedFiles(t *testing.T) {
	_, _, encoded := transferFixture(t)
	for name, data := range map[string][]byte{
		"empty":                nil,
		"root array":           []byte(`[]`),
		"trailing document":    append(append([]byte(nil), encoded...), []byte(`{}`)...),
		"trailing scalar":      append(append([]byte(nil), encoded...), []byte(`false`)...),
		"duplicate kind":       bytes.Replace(encoded, []byte(`"kind":`), []byte(`"kind":"passpine-config-sync","kind":`), 1),
		"duplicate option":     bytes.Replace(encoded, []byte(`"mode":`), []byte(`"mode":"manual","mode":`), 1),
		"duplicate selection":  bytes.Replace(encoded, []byte(`"appearance":`), []byte(`"appearance":true,"appearance":`), 1),
		"duplicate connection": bytes.Replace(encoded, []byte(`"bucket":`), []byte(`"bucket":"other-target","bucket":`), 1),
		"escaped duplicate":    bytes.Replace(encoded, []byte(`"mode":`), []byte(`"mode":"manual","\u006dode":`), 1),
		"invalid UTF-8":        append(append([]byte(nil), encoded...), 0xff),
	} {
		t.Run(name, func(t *testing.T) { assertInvalidSyncTransfer(t, data) })
	}
	padded := append(append([]byte(nil), encoded...), bytes.Repeat([]byte(" "), MaxConfigSyncTransferBytes-len(encoded))...)
	if _, _, err := DecodeConfigSyncTransfer(padded); err != nil {
		t.Fatal("a valid file at the documented size limit was rejected", err)
	}
	assertInvalidSyncTransfer(t, append(padded, ' '))
}

func TestConfigSyncTransferExportRejectsInvalidInputsWithoutPartialOutput(t *testing.T) {
	options, config, _ := transferFixture(t)
	invalidOptions := options
	invalidOptions.Mode = "unknown"
	invalidConnection := config
	invalidConnection.Endpoint = "https://private:credential@storage.example.test"
	for _, input := range []struct {
		options ConfigSyncOptions
		target  *cloudsync.Config
	}{{invalidOptions, &config}, {options, &invalidConnection}} {
		data, err := EncodeConfigSyncTransfer(input.options, input.target)
		if !errors.Is(err, ErrInvalidConfigSyncTransfer) || data != nil || strings.Contains(err.Error(), "credential") {
			t.Fatal("invalid export produced data or exposed connection details", err)
		}
	}
}

func assertInvalidSyncTransfer(t *testing.T, data []byte) {
	t.Helper()
	options, target, err := DecodeConfigSyncTransfer(data)
	if !errors.Is(err, ErrInvalidConfigSyncTransfer) || target != nil || !reflect.DeepEqual(options, ConfigSyncOptions{}) {
		t.Fatalf("invalid transfer produced partial data or an uncontrolled error: %+v %+v %v", options, target, err)
	}
	if err.Error() != ErrInvalidConfigSyncTransfer.Error() {
		t.Fatal("invalid transfer exposed raw file contents in its error")
	}
}
