package nativehost

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFrames(t *testing.T) {
	for _, data := range [][]byte{[]byte(`{"protocol":1}`), bytes.Repeat([]byte{'a'}, MaxFrame)} {
		var b bytes.Buffer
		if e := WriteFrame(&b, data); e != nil {
			t.Fatal(e)
		}
		read, e := ReadFrame(&b)
		if e != nil || !bytes.Equal(read, data) {
			t.Fatal(e)
		}
	}
	for _, size := range []uint32{0, MaxFrame + 1, 0xffffffff} {
		var h [4]byte
		binary.LittleEndian.PutUint32(h[:], size)
		if _, e := ReadFrame(bytes.NewReader(h[:])); e == nil {
			t.Fatalf("accepted size %d", size)
		}
	}
	if _, e := ReadFrame(bytes.NewReader([]byte{3, 0, 0, 0, 'x'})); e != io.ErrUnexpectedEOF {
		t.Fatal(e)
	}
}
func TestPersistentRelayAndOrigin(t *testing.T) {
	dir := t.TempDir()
	count := 0
	server, e := Start(dir, func(raw json.RawMessage) json.RawMessage {
		count++
		var r map[string]any
		if e := json.Unmarshal(raw, &r); e != nil {
			t.Fatal(e)
		}
		r["status"] = "ok"
		b, _ := json.Marshal(r)
		return b
	})
	if e != nil {
		t.Fatal(e)
	}
	defer server.Close()
	var input bytes.Buffer
	for _, id := range []string{"one", "two", "three"} {
		b, _ := json.Marshal(map[string]any{"protocol": 1, "requestId": id, "action": "configure"})
		_ = WriteFrame(&input, b)
	}
	var out bytes.Buffer
	if e = ServeStdio(dir, []string{"chrome-extension://" + ChromiumID + "/"}, &input, &out); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"one", "two", "three"} {
		b, e := ReadFrame(&out)
		if e != nil {
			t.Fatal(e)
		}
		var r map[string]any
		_ = json.Unmarshal(b, &r)
		if r["requestId"] != id || r["status"] != "ok" {
			t.Fatal(string(b))
		}
	}
	if count != 3 {
		t.Fatal(count)
	}
	for _, origin := range []string{"chrome-extension://attacker/", "chrome-extension://obnlebeedcbbchadakacilopfdaifgjh/"} {
		out.Reset()
		if e = ServeStdio(dir, []string{origin}, bytes.NewReader(nil), &out); e != nil {
			t.Fatal(e)
		}
		b, _ := ReadFrame(&out)
		if !bytes.Contains(b, []byte("unauthorized")) || count != 3 {
			t.Fatalf("unauthorized origin %s reached the vault: %s", origin, b)
		}
	}
	if _, e = Start(dir, func(r json.RawMessage) json.RawMessage { return r }); e == nil {
		t.Fatal("allowed second instance")
	}
}
func TestEndpointPermissionsAndReconnect(t *testing.T) {
	dir := t.TempDir()
	handler := func(r json.RawMessage) json.RawMessage { return r }
	server, e := Start(dir, handler)
	if e != nil {
		t.Fatal(e)
	}
	client := NewRelayClient(dir)
	defer client.Close()
	req := []byte(`{"protocol":1,"requestId":"1"}`)
	if got := client.Send(req); !bytes.Equal(got, req) {
		t.Fatal(string(got))
	}
	_ = server.Close()
	server, e = Start(dir, handler)
	if e != nil {
		t.Fatal(e)
	}
	defer server.Close()
	if got := client.Send(req); !bytes.Equal(got, req) {
		t.Fatal(string(got))
	}
	ep := filepath.Join(dir, "browser-endpoint.json")
	if runtime.GOOS == "windows" {
		return
	} // Windows uses ACLs rather than chmod permission bits.
	if e = os.Chmod(ep, 0644); e != nil {
		t.Fatal(e)
	}
	if got := client.Send(req); !bytes.Contains(got, []byte("app_unavailable")) {
		t.Fatal(string(got))
	}
}
func BenchmarkPersistentRelay(b *testing.B) {
	dir := b.TempDir()
	server, e := Start(dir, func(r json.RawMessage) json.RawMessage { return r })
	if e != nil {
		b.Fatal(e)
	}
	defer server.Close()
	c := NewRelayClient(dir)
	defer c.Close()
	r := []byte(`{"protocol":1,"action":"status","requestId":"1"}`)
	b.ResetTimer()
	for b.Loop() {
		_ = c.Send(r)
	}
}

func TestOversizedResponsePreservesRequestCorrelation(t *testing.T) {
	dir := t.TempDir()
	server, e := Start(dir, func(raw json.RawMessage) json.RawMessage {
		return bytes.Repeat([]byte{'x'}, MaxFrame+1)
	})
	if e != nil {
		t.Fatal(e)
	}
	defer server.Close()
	client := NewRelayClient(dir)
	defer client.Close()
	response := client.Send([]byte(`{"protocol":1,"action":"fetch","requestId":"large-entry-request"}`))
	var got struct {
		RequestID string `json:"requestId"`
		Status    string `json:"status"`
		Code      string `json:"code"`
	}
	if e = json.Unmarshal(response, &got); e != nil {
		t.Fatal(e)
	}
	if got.RequestID != "large-entry-request" || got.Status != "error" || got.Code != "response_too_large" {
		t.Fatalf("oversized response cannot be matched to its request: %s", response)
	}
}

func TestRelayRejectsUnsupportedProtocolBeforeConnecting(t *testing.T) {
	// No desktop endpoint exists: protocol errors must still be correlated
	// immediately, rather than timing out or becoming app_unavailable.
	client := NewRelayClient(t.TempDir())
	defer client.Close()
	for _, test := range []struct {
		name, request, code string
	}{
		{"newer protocol", `{"protocol":2,"requestId":"compatibility","action":"lock"}`, "protocol_mismatch"},
		{"invalid protocol type", `{"protocol":"1","requestId":"compatibility","action":"lock"}`, "invalid_request"},
		{"missing protocol", `{"requestId":"compatibility","action":"lock"}`, "invalid_request"},
		{"supported protocol without product versions", `{"protocol":1,"requestId":"compatibility","action":"configure"}`, "app_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := client.Send([]byte(test.request))
			var got struct {
				Protocol  int    `json:"protocol"`
				RequestID string `json:"requestId"`
				Status    string `json:"status"`
				Code      string `json:"code"`
			}
			if err := json.Unmarshal(response, &got); err != nil {
				t.Fatal(err)
			}
			if got.Protocol != Protocol || got.RequestID != "compatibility" || got.Status != "error" || got.Code != test.code {
				t.Fatalf("wrong protocol response: %s", response)
			}
		})
	}
}
