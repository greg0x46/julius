package httplog

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// hammer calls every setter and reader of the record API.
func hammer(w http.ResponseWriter, r *http.Request, rounds int) {
	ctx := r.Context()
	for i := 0; i < rounds; i++ {
		SetAuth(ctx, AuthOAuth, AuthRejected)
		SetOAuthClient(ctx, "client-id-1234", "Client")
		SetError(ctx, "some_code", "some_reason", "some_field")
		SetErrorValues(ctx, "value_a", "value_b")
		SetWriterError(w, "writer_code")
		Event(ctx, "security.rate_limited", slog.String("limiter", "mcp"))
		_ = RequestID(ctx)
	}
}

// The middleware must stay race-free when annotations keep arriving from
// another goroutine while (and after) it logs and returns. Run with -race
// where available.
func TestRecordIsSafeWhileMiddlewareReturns(t *testing.T) {
	l, _ := newTestLogger()
	mw := Middleware(l)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				hammer(w, r, 200)
			}()
			hammer(w, r, 20)
		}))
		serve(h, newReq("GET", "/x"))
	}
	wg.Wait()
}

func TestRecordIsSafeBehindTimeoutHandler(t *testing.T) {
	l, s := newTestLogger()
	mux := http.NewServeMux()
	done := make(chan struct{})
	mux.HandleFunc("POST /oauth/register", func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		time.Sleep(60 * time.Millisecond) // outlives the timeout
		hammer(w, r, 100)
	})
	h := Middleware(l)(WithRoute(mux, http.TimeoutHandler(mux, 10*time.Millisecond, "timed out")))
	rr := serve(h, newReq("POST", "/oauth/register"))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rr.Code)
	}
	<-done
	lines := s.lines()
	m := decode(t, lines[0])
	if m["event"] != "http_request" || m["status"] != float64(503) || m["route"] != "POST /oauth/register" {
		t.Fatalf("line: %v", m)
	}
}

func TestConcurrentRequestsShareNoState(t *testing.T) {
	l, s := newTestLogger()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /x", func(w http.ResponseWriter, r *http.Request) {
		SetError(r.Context(), "code_for_this_request", "", "")
		w.WriteHeader(http.StatusTeapot)
	})
	srv := httptest.NewServer(Middleware(l)(mux))
	defer srv.Close()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(srv.URL + "/x")
			if err == nil {
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()
	ids := map[string]bool{}
	for _, line := range s.waitLines(t, 20) {
		m := decode(t, line)
		ids[m["request_id"].(string)] = true
		if m["status"] != float64(418) || m["error_code"] != "code_for_this_request" {
			t.Fatalf("line: %v", m)
		}
	}
	if len(ids) != 20 {
		t.Fatalf("%d distinct request ids for 20 requests", len(ids))
	}
}
