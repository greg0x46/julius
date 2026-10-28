package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/httplog"
)

// logSink collects the JSON log output of a handler wrapped in httplog.
type logSink struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *logSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

// drain returns everything logged so far and clears the sink.
func (s *logSink) drain() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.buf.String()
	s.buf.Reset()
	return out
}

func newLogged(t *testing.T, store *auth.Store) (http.Handler, *logSink) {
	t.Helper()
	h, err := NewHandler(true, testConfig, store)
	if err != nil {
		t.Fatal(err)
	}
	return withLogging(h)
}

func withLogging(h http.Handler) (http.Handler, *logSink) {
	sink := &logSink{}
	return httplog.Middleware(slog.New(slog.NewJSONHandler(sink, nil)))(h), sink
}

var tokenValue = regexp.MustCompile(`^[a-z0-9_.:-]{1,64}$`)

// logged is what one request left in the log.
type logged struct {
	raw    string
	access map[string]any
	events []map[string]any
}

// parseLines decodes every line, failing on invalid JSON and on controlled
// values that are not plain tokens (a typo in a literal would show as
// "invalid_value").
func parseLines(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line is not JSON: %v\n%q", err, line)
		}
		for _, key := range []string{"error_code", "error_reason", "invalid_field"} {
			if v, ok := m[key]; ok && (!tokenValue.MatchString(v.(string)) || v == "invalid_value") {
				t.Fatalf("%s = %q is not a valid token: %s", key, v, line)
			}
		}
		if values, ok := m["invalid_values"]; ok {
			for _, v := range values.([]any) {
				if !tokenValue.MatchString(v.(string)) || v == "invalid_value" {
					t.Fatalf("invalid_values holds %q: %s", v, line)
				}
			}
		}
		out = append(out, m)
	}
	return out
}

// one drains the sink expecting a single request: one http_request line plus
// the events that share its request_id.
func (s *logSink) one(t *testing.T) logged {
	t.Helper()
	raw := s.drain()
	c := logged{raw: raw}
	for _, m := range parseLines(t, raw) {
		if m["event"] == "http_request" {
			if c.access != nil {
				t.Fatalf("more than one access line:\n%s", raw)
			}
			c.access = m
			continue
		}
		c.events = append(c.events, m)
	}
	if c.access == nil {
		t.Fatalf("no access line:\n%s", raw)
	}
	for _, e := range c.events {
		if e["request_id"] != c.access["request_id"] {
			t.Fatalf("event %v is not correlated with the access line:\n%s", e["event"], raw)
		}
	}
	return c
}

func str(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// eventNames lists the event names in order.
func (c logged) eventNames() []string {
	var names []string
	for _, e := range c.events {
		names = append(names, str(e, "event"))
	}
	return names
}

func (c logged) event(t *testing.T, name string) map[string]any {
	t.Helper()
	for _, e := range c.events {
		if e["event"] == name {
			return e
		}
	}
	t.Fatalf("missing event %s; got %v\n%s", name, c.eventNames(), c.raw)
	return nil
}

// invalidValues decodes invalid_values.
func (c logged) invalidValues() []string {
	var out []string
	if values, ok := c.access["invalid_values"].([]any); ok {
		for _, v := range values {
			out = append(out, v.(string))
		}
	}
	return out
}

// wantError is the controlled diagnosis an access line must carry.
type wantError struct {
	code, reason, field string
	values              []string
}

func (c logged) expectError(t *testing.T, want wantError) {
	t.Helper()
	got := wantError{str(c.access, "error_code"), str(c.access, "error_reason"), str(c.access, "invalid_field"), c.invalidValues()}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("access line diagnosis = %+v, want %+v\n%s", got, want, c.raw)
	}
}

func (c logged) expectRequest(t *testing.T, route string, status int, authType, authResult string) {
	t.Helper()
	if str(c.access, "route") != route || int(c.access["status"].(float64)) != status || str(c.access, "auth_type") != authType || str(c.access, "auth_result") != authResult {
		t.Fatalf("access line = route %q status %v auth %s/%s, want %q %d %s/%s\n%s", c.access["route"], c.access["status"], c.access["auth_type"], c.access["auth_result"], route, status, authType, authResult, c.raw)
	}
}

func (c logged) expectClient(t *testing.T, name, prefix string) {
	t.Helper()
	if str(c.access, "client_name") != name || str(c.access, "client_id_prefix") != prefix {
		t.Fatalf("access line client = %q/%q, want %q/%q\n%s", c.access["client_name"], c.access["client_id_prefix"], name, prefix, c.raw)
	}
}

func (c logged) expectNoEvents(t *testing.T) {
	t.Helper()
	if len(c.events) != 0 {
		t.Fatalf("unexpected events %v\n%s", c.eventNames(), c.raw)
	}
}

func (c logged) expectEvent(t *testing.T, name string, attrs map[string]string) {
	t.Helper()
	e := c.event(t, name)
	for key, want := range attrs {
		if str(e, key) != want {
			t.Fatalf("event %s %s = %q, want %q\n%s", name, key, str(e, key), want, c.raw)
		}
	}
	// Events carry only the allowlisted attributes.
	for key := range e {
		switch key {
		case "time", "level", "msg", "event", "request_id", "reason", "limiter", "scope", "client_name", "client_id_prefix":
		default:
			t.Fatalf("event %s has unexpected attribute %s", name, key)
		}
	}
}

func jsonRequest(h http.Handler, method, path, body string, header map[string]string) *httptest.ResponseRecorder {
	return jsonReader(h, method, path, strings.NewReader(body), header)
}

func jsonReader(h http.Handler, method, path string, body io.Reader, header map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, body)
	r.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// flowOn registers a client through h and prepares an authorization request
// for it, like newFlowWithRedirect but on an arbitrary handler.
func flowOn(t *testing.T, h http.Handler, name, redirect string) *oauthFlow {
	t.Helper()
	metadata, err := json.Marshal(clientMetadata{Name: name, Redirects: []string{redirect}, Method: "none"})
	if err != nil {
		t.Fatal(err)
	}
	w := jsonRequest(h, "POST", "/oauth/register", string(metadata), nil)
	var c clientMetadata
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &c) != nil {
		t.Fatalf("register: %d %s", w.Code, w.Body)
	}
	f := &oauthFlow{handler: h, client: c.ID, verifier: strings.Repeat("v", 43)}
	f.params = url.Values{"client_id": {c.ID}, "redirect_uri": {redirect}, "response_type": {"code"}, "resource": {testConfig.PublicURL + "/mcp"}, "scope": {mcpScope}, "code_challenge": {pkce(f.verifier)}, "code_challenge_method": {"S256"}, "state": {"opaque-state"}}
	return f
}

func mustJSON(t *testing.T, raw []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("invalid JSON %q: %v", raw, err)
	}
}
