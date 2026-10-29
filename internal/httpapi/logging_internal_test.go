package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/httplog"
	"github.com/greg0x46/julius/internal/reporting"
)

func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not JSON: %q", line)
		}
		out = append(out, m)
	}
	return out
}

// acquire tells the two ways a request is throttled apart: the attempt window
// (per e-mail and global) and a hashing slot that is already taken.
func TestAcquireNamesTheLimiter(t *testing.T) {
	newRig := func() (*authAPI, http.Handler, *bytes.Buffer) {
		a := newAuthAPI(nil, auth.Config{})
		var buf bytes.Buffer
		h := httplog.Middleware(slog.New(slog.NewJSONHandler(&buf, nil)))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if a.acquire(w, r, r.URL.Query().Get("email")) {
				<-a.hashing
				w.WriteHeader(http.StatusNoContent)
			}
		}))
		return a, h, &buf
	}
	get := func(h http.Handler, email string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/x?email="+email, nil))
		return rec
	}
	assertThrottled := func(t *testing.T, rec *httptest.ResponseRecorder, buf *bytes.Buffer, limiter, reason string) {
		t.Helper()
		if rec.Code != 429 || rec.Header().Get("Retry-After") != "60" {
			t.Fatalf("status %d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
		}
		var event, access map[string]any
		for _, l := range logLines(t, buf) {
			switch l["event"] {
			case "security.rate_limited":
				event = l
			case "http_request":
				access = l
			}
		}
		if event == nil || access == nil {
			t.Fatalf("missing event or access line:\n%s", buf)
		}
		if event["limiter"] != limiter || event["reason"] != reason || event["request_id"] != access["request_id"] {
			t.Fatalf("event %v, access %v", event, access)
		}
		if access["error_code"] != "too-many-attempts" || access["error_reason"] != reason {
			t.Fatalf("access %v", access)
		}
		if strings.Contains(buf.String(), "mail.example") {
			t.Fatalf("e-mail in the log:\n%s", buf)
		}
	}

	t.Run("busy hashing", func(t *testing.T) {
		a, h, buf := newRig()
		a.hashing <- struct{}{}
		assertThrottled(t, get(h, "a@mail.example"), buf, "auth_hashing_busy", "busy")
	})
	t.Run("per e-mail window", func(t *testing.T) {
		_, h, buf := newRig()
		for i := 0; i < 5; i++ {
			if rec := get(h, "a@mail.example"); rec.Code != 204 {
				t.Fatalf("attempt %d: %d", i, rec.Code)
			}
		}
		buf.Reset()
		assertThrottled(t, get(h, "a@mail.example"), buf, "auth_attempts", "window_exceeded")
	})
	t.Run("global window", func(t *testing.T) {
		_, h, buf := newRig()
		for i := 0; i < 30; i++ {
			if rec := get(h, fmt.Sprintf("u%d@mail.example", i)); rec.Code != 204 {
				t.Fatalf("attempt %d: %d", i, rec.Code)
			}
		}
		buf.Reset()
		assertThrottled(t, get(h, "u30@mail.example"), buf, "auth_attempts", "window_exceeded")
	})
	t.Run("no event when allowed", func(t *testing.T) {
		_, h, buf := newRig()
		if rec := get(h, "a@mail.example"); rec.Code != 204 {
			t.Fatalf("status %d", rec.Code)
		}
		for _, l := range logLines(t, buf) {
			if l["event"] == "security.rate_limited" {
				t.Fatal("event for an allowed request")
			}
		}
	})
}

func TestReportingProblemFeedsErrorCode(t *testing.T) {
	var buf bytes.Buffer
	h := httplog.Middleware(slog.New(slog.NewJSONHandler(&buf, nil)))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeReportingProblem(w, &reporting.IncompleteError{Detail: "synthetic"})
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))
	if rec.Code != 503 || !strings.Contains(rec.Body.String(), `"type":"/problems/reporting-fx-unavailable"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	lines := logLines(t, &buf)
	if len(lines) != 1 || lines[0]["error_code"] != "reporting-fx-unavailable" {
		t.Fatalf("log: %v", lines)
	}
}

// writeProblem passes its kind to the access log, which accepts only
// [a-z0-9_.:-]{1,64} and otherwise logs "invalid_value". Pin every literal
// kind in the package so a typo cannot silently degrade the log.
func TestProblemKindsAreLoggable(t *testing.T) {
	loggable := regexp.MustCompile(`^[a-z0-9_.:-]{1,64}$`)
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, entry.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			var kind ast.Expr
			switch {
			case fn.Name == "writeProblem" && len(call.Args) == 5:
				kind = call.Args[2]
			case fn.Name == "invalid" && len(call.Args) == 3: // toFilters' problem builder
				kind = call.Args[0]
			default:
				return true
			}
			lit, ok := kind.(*ast.BasicLit)
			if !ok {
				// The only forwarded kind: toFilters' Problem.Type minus its prefix.
				var b strings.Builder
				_ = printer.Fprint(&b, fset, kind)
				if b.String() != `problem.Type[len("/problems/"):]` {
					t.Errorf("%s: kind %s is not a literal", fset.Position(kind.Pos()), b.String())
				}
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil || !loggable.MatchString(value) {
				t.Errorf("%s: problem kind %s would be logged as invalid_value", fset.Position(lit.Pos()), lit.Value)
			}
			checked++
			return true
		})
	}
	if checked < 100 {
		t.Fatalf("only %d problem kinds found; the scan is probably broken", checked)
	}
}
