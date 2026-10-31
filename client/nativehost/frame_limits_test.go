package nativehost

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestPrivateRelaySupportsLargeEntriesWithoutChangingBrowserLimit(t *testing.T) {
	request, err := json.Marshal(map[string]any{"protocol": Protocol, "action": "save", "contents": string(bytes.Repeat([]byte{'x'}, MaxFrame+1))})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		limit int
		want  int32
	}{
		{"private endpoint", maxFrameLimit, 1},
		{"browser endpoint", MaxFrame, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			var calls atomic.Int32
			server, err := StartWithFrameLimit(dir, test.limit, func(raw json.RawMessage) json.RawMessage {
				calls.Add(1)
				return bytes.Clone(raw)
			})
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			client := NewRelayClientWithFrameLimit(dir, maxFrameLimit)
			defer client.Close()
			got := client.Send(request)
			if calls.Load() != test.want {
				t.Fatalf("handler calls = %d, want %d", calls.Load(), test.want)
			}
			if test.want == 1 {
				if !bytes.Equal(got, request) {
					t.Fatalf("large private request/response did not round trip: %d bytes", len(got))
				}
			} else if !bytes.Contains(got, []byte("transport_error")) {
				t.Fatalf("browser endpoint accepted oversized request: %s", got)
			}
		})
	}
	var encoded bytes.Buffer
	if err := WriteFrame(&encoded, request); err == nil {
		t.Fatal("default browser frame writer accepted a large entry")
	}
}

func TestPrivateRelayRetainsAuthentication(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	server, err := StartWithFrameLimit(dir, maxFrameLimit, func(raw json.RawMessage) json.RawMessage {
		calls.Add(1)
		return bytes.Clone(raw)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	endpoint, err := readEndpoint(filepath.Join(dir, "browser-endpoint.json"))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp", endpoint.Address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	request, err := json.Marshal(envelope{Token: "wrong-token", Request: json.RawMessage(`{"protocol":1,"action":"save"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err = writeFrame(conn, request, maxFrameLimit); err != nil {
		t.Fatal(err)
	}
	got, err := readFrame(conn, maxFrameLimit)
	if err != nil || !bytes.Contains(got, []byte("unauthorized")) || calls.Load() != 0 {
		t.Fatalf("unauthenticated request reached private handler: %s, %v", got, err)
	}
}

func TestInvalidPrivateFrameLimitRejectedBeforeIO(t *testing.T) {
	for _, limit := range []int{-1, 0, MaxFrame - 1, maxFrameLimit + 1} {
		dir := filepath.Join(t.TempDir(), "endpoint")
		server, err := StartWithFrameLimit(dir, limit, func(raw json.RawMessage) json.RawMessage { return raw })
		if err == nil || server != nil {
			t.Fatalf("accepted invalid frame limit %d", limit)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("invalid frame limit %d created an endpoint directory", limit)
		}
		client := NewRelayClientWithFrameLimit(dir, limit)
		response := client.Send([]byte(`{"protocol":1,"requestId":"invalid-limit"}`))
		client.Close()
		if !bytes.Contains(response, []byte("invalid_request")) {
			t.Fatalf("invalid frame limit %d connected: %s", limit, response)
		}
	}
}

func TestPrivateRelayDoesNotReplayMutationWhenResponseIsLost(t *testing.T) {
	dir := t.TempDir()
	ready := make(chan *Server, 1)
	var calls atomic.Int32
	server, err := StartWithFrameLimit(dir, maxFrameLimit, func(raw json.RawMessage) json.RawMessage {
		calls.Add(1)
		s := <-ready
		ready <- s
		// The operation has completed, but the connection is lost before its
		// acknowledgement. Retrying here would apply the mutation twice.
		s.mu.Lock()
		for conn := range s.connections {
			_ = conn.Close()
		}
		s.mu.Unlock()
		return json.RawMessage(`{"protocol":1,"status":"ok"}`)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ready <- server
	client := NewRelayClientWithFrameLimit(dir, maxFrameLimit)
	defer client.Close()
	response := client.Send([]byte(`{"protocol":1,"action":"save"}`))
	if calls.Load() != 1 || !bytes.Contains(response, []byte("transport_error")) {
		t.Fatalf("lost acknowledgement was replayed: calls=%d response=%s", calls.Load(), response)
	}
}
