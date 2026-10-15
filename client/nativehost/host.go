// Package nativehost implements a bounded native-messaging relay. Browser
// processes never own an independent vault session or private key.
package nativehost

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const Name = "com.knothhe.passpine.neo"
const FirefoxID = "passpine-neo@knothhe.com"

// ChromiumID is tied to the checked-in browser manifest public key.
const ChromiumID = "obnlebeedcbbchadakacilopfdaifgjh"
const MaxFrame = 1024 * 1024

// Native authentication may wait up to two minutes for a user to respond.
// Keep the relay alive until the client finishes or cancels that interaction.
const relayTimeout = 130 * time.Second

var ErrAlreadyRunning = errors.New("Passpine Neo is already running")

type Endpoint struct {
	Address string `json:"address"`
	Token   string `json:"token"`
	PID     int    `json:"pid"`
}
type envelope struct {
	Token   string          `json:"token"`
	Request json.RawMessage `json:"request"`
}
type Handler func(json.RawMessage) json.RawMessage

type Server struct {
	listener    net.Listener
	endpoint    string
	token       string
	once        sync.Once
	done        chan struct{}
	mu          sync.Mutex
	connections map[net.Conn]bool
}

func DataDir() (string, error) {
	if d := os.Getenv("PASSPINE_NEO_HOME"); d != "" {
		return filepath.Abs(d)
	}
	base, e := os.UserConfigDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(base, "Passpine Neo"), nil
}
func ReadFrame(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, e := io.ReadFull(r, header[:]); e != nil {
		return nil, e
	}
	size := binary.LittleEndian.Uint32(header[:])
	if size == 0 || size > MaxFrame {
		return nil, errors.New("invalid native message size")
	}
	data := make([]byte, int(size))
	_, e := io.ReadFull(r, data)
	return data, e
}
func WriteFrame(w io.Writer, data []byte) error {
	if len(data) == 0 || len(data) > MaxFrame {
		return errors.New("invalid native message size")
	}
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(len(data)))
	if e := writeAll(w, b[:]); e != nil {
		return e
	}
	return writeAll(w, data)
}
func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, e := w.Write(b)
		if e != nil {
			return e
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}
func failure(code string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"protocol": 1, "status": "error", "code": code})
	return b
}

func requestFailure(request json.RawMessage, code string) json.RawMessage {
	var r struct {
		RequestID string `json:"requestId"`
	}
	_ = json.Unmarshal(request, &r)
	b, _ := json.Marshal(map[string]any{"protocol": 1, "requestId": r.RequestID, "status": "error", "code": code})
	return b
}

func Start(dir string, handler Handler) (*Server, error) {
	if err := privateDirectory(dir); err != nil {
		return nil, err
	}
	endpoint := filepath.Join(dir, "browser-endpoint.json")
	if prior, e := readEndpoint(endpoint); e == nil {
		if c, e := net.DialTimeout("tcp", prior.Address, 200*time.Millisecond); e == nil {
			c.Close()
			return nil, ErrAlreadyRunning
		}
	}
	ln, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return nil, e
	}
	token := make([]byte, 32)
	if _, e = rand.Read(token); e != nil {
		ln.Close()
		return nil, e
	}
	ep := Endpoint{Address: ln.Addr().String(), Token: hex.EncodeToString(token), PID: os.Getpid()}
	b, _ := json.Marshal(ep)
	if e = writePrivate(endpoint, b); e != nil {
		ln.Close()
		return nil, e
	}
	s := &Server{listener: ln, endpoint: endpoint, token: ep.Token, done: make(chan struct{}), connections: map[net.Conn]bool{}}
	go func() {
		defer close(s.done)
		for {
			conn, e := ln.Accept()
			if e != nil {
				return
			}
			s.mu.Lock()
			s.connections[conn] = true
			s.mu.Unlock()
			go s.serve(conn, handler)
		}
	}()
	return s, nil
}
func (s *Server) serve(c net.Conn, h Handler) {
	defer func() { c.Close(); s.mu.Lock(); delete(s.connections, c); s.mu.Unlock() }()
	for {
		_ = c.SetDeadline(time.Now().Add(relayTimeout))
		frame, e := ReadFrame(c)
		if e != nil {
			return
		}
		var v envelope
		valid := json.Unmarshal(frame, &v) == nil && subtle.ConstantTimeCompare([]byte(v.Token), []byte(s.token)) == 1
		clear(frame)
		if !valid {
			_ = WriteFrame(c, failure("unauthorized"))
			return
		}
		// Time spent idle before reading this request must not consume the
		// authentication window for the newly received operation.
		_ = c.SetDeadline(time.Now().Add(relayTimeout))
		response := h(v.Request)
		if len(response) > MaxFrame {
			clear(response)
			response = requestFailure(v.Request, "response_too_large")
		}
		e = WriteFrame(c, response)
		clear(v.Request)
		clear(response)
		if e != nil {
			return
		}
	}
}

func (s *Server) Close() error {
	var err error
	s.once.Do(func() {
		err = s.listener.Close()
		<-s.done
		s.mu.Lock()
		for c := range s.connections {
			_ = c.Close()
		}
		s.mu.Unlock()
		if e, x := readEndpoint(s.endpoint); x == nil && e.Token == s.token {
			_ = os.Remove(s.endpoint)
		}
	})
	return err
}
func readEndpoint(path string) (Endpoint, error) {
	var ep Endpoint
	info, e := os.Lstat(path)
	if e != nil {
		return ep, e
	}
	if !info.Mode().IsRegular() || info.Size() > 4096 || !ownedPrivate(info) {
		return ep, errors.New("unsafe endpoint permissions")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return ep, e
	}
	defer clear(b)
	if e = json.Unmarshal(b, &ep); e != nil {
		return ep, e
	}
	host, port, e := net.SplitHostPort(ep.Address)
	if e != nil || host != "127.0.0.1" || port == "" || len(ep.Token) != 64 {
		return ep, errors.New("invalid endpoint")
	}
	if _, e = hex.DecodeString(ep.Token); e != nil {
		return ep, e
	}
	return ep, nil
}

// RelayClient keeps the authenticated loopback connection alive across browser
// requests. Failed mutations are never replayed: the next request reconnects.
type RelayClient struct {
	lastUsed time.Time
	dir      string
	conn     net.Conn
	endpoint Endpoint
}

func NewRelayClient(dir string) *RelayClient { return &RelayClient{dir: dir} }
func (c *RelayClient) Close() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}
func (c *RelayClient) Send(request []byte) json.RawMessage {
	var r struct {
		Protocol  int    `json:"protocol"`
		RequestID string `json:"requestId"`
	}
	if json.Unmarshal(request, &r) != nil || r.Protocol != 1 {
		return failure("invalid_request")
	}
	fail := func(code string) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"protocol": 1, "requestId": r.RequestID, "status": "error", "code": code})
		return b
	}
	if time.Since(c.lastUsed) > 60*time.Second {
		c.Close()
	}
	ep, e := readEndpoint(filepath.Join(c.dir, "browser-endpoint.json"))
	if e != nil {
		c.Close()
		return fail("app_unavailable")
	}
	if ep.Token != c.endpoint.Token || ep.Address != c.endpoint.Address {
		c.Close()
	}
	if c.conn == nil {
		c.conn, e = net.DialTimeout("tcp", ep.Address, 2*time.Second)
		if e != nil {
			return fail("app_unavailable")
		}
		c.endpoint = ep
	}
	_ = c.conn.SetDeadline(time.Now().Add(relayTimeout))
	b, _ := json.Marshal(envelope{Token: ep.Token, Request: request})
	e = WriteFrame(c.conn, b)
	clear(b)
	if e != nil {
		c.Close()
		return fail("transport_error")
	}
	response, e := ReadFrame(c.conn)
	if e != nil {
		c.Close()
		return fail("transport_error")
	}
	c.lastUsed = time.Now()
	return response
}
func Relay(dir string, request []byte) json.RawMessage {
	c := NewRelayClient(dir)
	defer c.Close()
	return c.Send(request)
}

func Authorized(args []string) bool {
	if len(args) > 0 && args[0] == "chrome-extension://"+ChromiumID+"/" {
		return true
	}
	return len(args) > 1 && args[1] == FirefoxID
}
func ServeStdio(dir string, args []string, r io.Reader, w io.Writer) error {
	if !Authorized(args) {
		return WriteFrame(w, failure("unauthorized"))
	}
	client := NewRelayClient(dir)
	defer client.Close()
	for {
		b, e := ReadFrame(r)
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return WriteFrame(w, failure("invalid_request"))
		}
		response := client.Send(b)
		clear(b)
		e = WriteFrame(w, response)
		clear(response)
		if e != nil {
			return e
		}
	}
}
func privateDirectory(dir string) error {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	info, e := os.Lstat(dir)
	if e != nil {
		return e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("unsafe data directory")
	}
	return os.Chmod(dir, 0700)
}
func writePrivate(path string, data []byte) error {
	if info, e := os.Lstat(path); e == nil && (!info.Mode().IsRegular() || !ownedPrivate(info)) {
		return errors.New("unsafe existing file")
	}
	file, e := os.CreateTemp(filepath.Dir(path), ".passpine-*")
	if e != nil {
		return e
	}
	name := file.Name()
	defer os.Remove(name)
	if e = file.Chmod(0600); e == nil {
		_, e = file.Write(data)
	}
	if e == nil {
		e = file.Sync()
	}
	ce := file.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(name, path)
}
func Register(executable string) ([]string, error) {
	path, e := filepath.Abs(executable)
	if e != nil {
		return nil, e
	}
	i, e := os.Stat(path)
	if e != nil || !i.Mode().IsRegular() {
		return nil, errors.New("native helper executable is missing")
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return nil, e
	}
	var dirs []string
	switch runtime.GOOS {
	case "darwin":
		for _, v := range []string{"Google/Chrome", "Google/Chrome Beta", "Chromium", "Microsoft Edge", "BraveSoftware/Brave-Browser", "Mozilla"} {
			dirs = append(dirs, filepath.Join(home, "Library/Application Support", v, "NativeMessagingHosts"))
		}
	case "linux":
		for _, v := range []string{".config/google-chrome/NativeMessagingHosts", ".config/chromium/NativeMessagingHosts", ".config/microsoft-edge/NativeMessagingHosts", ".config/BraveSoftware/Brave-Browser/NativeMessagingHosts", ".mozilla/native-messaging-hosts"} {
			dirs = append(dirs, filepath.Join(home, v))
		}
	default:
		return nil, errors.New("automatic host registration is supported on macOS and Linux")
	}
	installed := []string{}
	for _, dir := range dirs {
		m := map[string]any{"name": Name, "description": "Passpine Neo native password manager", "path": path, "type": "stdio"}
		if strings.Contains(strings.ToLower(dir), "mozilla") {
			m["allowed_extensions"] = []string{FirefoxID}
		} else {
			m["allowed_origins"] = []string{"chrome-extension://" + ChromiumID + "/"}
		}
		b, _ := json.MarshalIndent(m, "", "  ")
		if e = os.MkdirAll(dir, 0700); e != nil {
			return installed, e
		}
		target := filepath.Join(dir, Name+".json")
		if e = writePrivate(target, b); e != nil {
			return installed, e
		}
		installed = append(installed, target)
	}
	return installed, nil
}

// CheckRegistration only accepts manifests pointing at this exact helper.
func CheckRegistration(executable string) bool {
	home, e := os.UserHomeDir()
	if e != nil {
		return false
	}
	dir := filepath.Join(home, "Library/Application Support/Google/Chrome/NativeMessagingHosts")
	if runtime.GOOS == "linux" {
		dir = filepath.Join(home, ".config/google-chrome/NativeMessagingHosts")
	}
	b, e := os.ReadFile(filepath.Join(dir, Name+".json"))
	if e != nil {
		return false
	}
	var m struct {
		Path string `json:"path"`
	}
	return json.Unmarshal(b, &m) == nil && m.Path == executable
}
func HelperPath() string {
	p, e := os.Executable()
	if e != nil {
		return ""
	}
	name := "passpine-host"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(p), name)
}
