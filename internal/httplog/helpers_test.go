package httplog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// sink is a goroutine-safe buffer for the JSON log output.
type sink struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *sink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *sink) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func (s *sink) lines() []string {
	var out []string
	for _, l := range strings.Split(s.String(), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func newTestLogger() (*slog.Logger, *sink) {
	s := &sink{}
	return slog.New(slog.NewJSONHandler(s, nil)), s
}

// waitLines waits until at least n lines were logged (a real server may
// finish the response before the middleware's deferred log is visible).
func (s *sink) waitLines(t *testing.T, n int) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if l := s.lines(); len(l) >= n {
			return l
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d log lines, have: %q", n, s.String())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func decode(t *testing.T, line string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("log line is not JSON: %v\n%s", err, line)
	}
	return m
}

// keysOf returns the top-level keys of a JSON object line in order.
func keysOf(t *testing.T, line string) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(line))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		t.Fatalf("not a JSON object: %q (%v)", line, err)
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, tok.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// serve runs one request through h and returns the recorder.
func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	return rr
}

// access runs one request through the middleware and returns the single
// access line, decoded.
func access(t *testing.T, h http.Handler, r *http.Request) (map[string]any, *sink) {
	t.Helper()
	l, s := newTestLogger()
	serve(Middleware(l)(h), r)
	lines := s.lines()
	if len(lines) != 1 {
		t.Fatalf("want exactly 1 log line, got %d: %q", len(lines), s.String())
	}
	return decode(t, lines[0]), s
}

func newReq(method, target string) *http.Request {
	return httptest.NewRequest(method, target, nil)
}

func newServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// fakeBase is a minimal ResponseWriter with none of the optional interfaces.
type fakeBase struct {
	hdr   http.Header
	codes []int
	body  bytes.Buffer
}

func (f *fakeBase) Header() http.Header {
	if f.hdr == nil {
		f.hdr = http.Header{}
	}
	return f.hdr
}
func (f *fakeBase) Write(p []byte) (int, error) { return f.body.Write(p) }
func (f *fakeBase) WriteHeader(code int)        { f.codes = append(f.codes, code) }

type fakeFlush struct{ flushes int }

func (f *fakeFlush) Flush() { f.flushes++ }

type fakeHijack struct{ hijacks int }

func (f *fakeHijack) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	f.hijacks++
	c, _ := net.Pipe()
	return c, bufio.NewReadWriter(bufio.NewReader(c), bufio.NewWriter(c)), nil
}

type fakeReadFrom struct {
	dst   *fakeBase
	calls int
}

func (f *fakeReadFrom) ReadFrom(r io.Reader) (int64, error) {
	f.calls++
	return io.Copy(&f.dst.body, r)
}

// fakeWriter builds a ResponseWriter that has exactly the requested optional
// interfaces.
type fakeWriter struct {
	http.ResponseWriter
	base  *fakeBase
	flush *fakeFlush
	hij   *fakeHijack
	rf    *fakeReadFrom
}

func newFakeWriter(flusher, hijacker, readerFrom bool) fakeWriter {
	b := &fakeBase{}
	f, h, r := &fakeFlush{}, &fakeHijack{}, &fakeReadFrom{dst: b}
	fw := fakeWriter{base: b, flush: f, hij: h, rf: r}
	switch {
	case flusher && hijacker && readerFrom:
		fw.ResponseWriter = struct {
			*fakeBase
			*fakeFlush
			*fakeHijack
			*fakeReadFrom
		}{b, f, h, r}
	case flusher && hijacker:
		fw.ResponseWriter = struct {
			*fakeBase
			*fakeFlush
			*fakeHijack
		}{b, f, h}
	case flusher && readerFrom:
		fw.ResponseWriter = struct {
			*fakeBase
			*fakeFlush
			*fakeReadFrom
		}{b, f, r}
	case hijacker && readerFrom:
		fw.ResponseWriter = struct {
			*fakeBase
			*fakeHijack
			*fakeReadFrom
		}{b, h, r}
	case flusher:
		fw.ResponseWriter = struct {
			*fakeBase
			*fakeFlush
		}{b, f}
	case hijacker:
		fw.ResponseWriter = struct {
			*fakeBase
			*fakeHijack
		}{b, h}
	case readerFrom:
		fw.ResponseWriter = struct {
			*fakeBase
			*fakeReadFrom
		}{b, r}
	default:
		fw.ResponseWriter = b
	}
	return fw
}

func discardLogger() *log.Logger { return log.New(io.Discard, "", 0) }
