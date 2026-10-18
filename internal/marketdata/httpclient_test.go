package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// newTestHTTPClient returns a client on a limiter with a fake clock, so what
// the limiter would wait shows up in clock.slept instead of being slept, and
// the backoff sleeps the client does itself in the returned slice.
func newTestHTTPClient(t *testing.T, handler http.HandlerFunc) (*httpClient, string, *[]time.Duration, *fakeClock) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	limiter, clock := newTestLimiter(0)
	client := newHTTPClient("test", limiter)
	client.client = server.Client()
	backoffs := &[]time.Duration{}
	client.sleep = func(d time.Duration) { *backoffs = append(*backoffs, d) }
	client.backoff = time.Millisecond
	return client, server.URL, backoffs, clock
}

func getMap(c *httpClient, url string) (map[string]any, error) {
	var payload map[string]any
	err := c.getJSON(context.Background(), url, nil, &payload)
	return payload, err
}

func TestHTTPClientSendsTheRequestHeadersAndKeepsNumbersExact(t *testing.T) {
	client, url, _, _ := newTestHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "agent/1" {
			t.Errorf("User-Agent = %q, want agent/1", got)
		}
		w.Write([]byte(`{"price":49.2599983215332}`))
	})

	var payload struct{ Price json.Number }
	if err := client.getJSON(context.Background(), url, map[string]string{"User-Agent": "agent/1"}, &payload); err != nil {
		t.Fatalf("getJSON: %v", err)
	}
	if payload.Price != "49.2599983215332" {
		t.Errorf("price = %q, want the digits as sent", payload.Price)
	}
}

func TestHTTPClientWaitsOutAShortRetryAfter(t *testing.T) {
	attempts := 0
	client, url, backoffs, clock := newTestHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": 1})
	})

	if _, err := getMap(client, url); err != nil {
		t.Fatalf("getJSON: %v", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
	if len(clock.slept) != 1 || clock.slept[0] != 2*time.Second {
		t.Errorf("limiter waited %v, want the provider's own Retry-After (2s)", clock.slept)
	}
	if len(*backoffs) != 0 {
		t.Errorf("backoffs = %v, want none: the provider said how long to wait", *backoffs)
	}
}

// A Retry-After longer than is worth waiting inline is not slept through: the
// request fails with a RateLimitedError, and the whole provider stays paused
// so the next asset in line does not walk into the same wall.
func TestHTTPClientDefersWhenRetryAfterIsLong(t *testing.T) {
	attempts := 0
	client, url, backoffs, clock := newTestHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "300")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := getMap(client, url)
	var limited *RateLimitedError
	if !errors.As(err, &limited) {
		t.Fatalf("error = %v, want a *RateLimitedError", err)
	}
	if limited.RetryAfter != 300*time.Second || limited.Provider != "test" {
		t.Errorf("limited = %+v, want 300s for provider test", limited)
	}
	if attempts != 1 || len(*backoffs) != 0 || len(clock.slept) != 0 {
		t.Errorf("attempts = %d, backoffs = %v, waits = %v; want one request and no sleeping", attempts, *backoffs, clock.slept)
	}

	// Next request to the provider: refused before it is even sent.
	if _, err := getMap(client, url); !errors.As(err, &limited) {
		t.Fatalf("second request error = %v, want it refused by the limiter", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, a paused provider must not be contacted", attempts)
	}
}

func TestHTTPClientPersistent429WithoutAHintPausesTheProviderForTheDefaultPenalty(t *testing.T) {
	attempts := 0
	client, url, _, _ := newTestHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := getMap(client, url)
	var limited *RateLimitedError
	if !errors.As(err, &limited) {
		t.Fatalf("error = %v, want a *RateLimitedError", err)
	}
	if limited.RetryAfter != defaultPenalty {
		t.Errorf("RetryAfter = %v, want the default penalty %v", limited.RetryAfter, defaultPenalty)
	}
	if attempts != client.maxAttempts {
		t.Errorf("attempts = %d, want %d: short backoff retries come first", attempts, client.maxAttempts)
	}
	if errors.Is(err, ErrUnavailable) {
		t.Error("a rate limit is not an outage")
	}
}

func TestHTTPClientStopsAProactivePauseWhenTheWindowIsSpent(t *testing.T) {
	calls := 0
	client, url, _, _ := newTestHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("RateLimit-Remaining", "0")
		w.Header().Set("RateLimit-Reset", "300")
		json.NewEncoder(w).Encode(map[string]any{"ok": 1})
	})

	if _, err := getMap(client, url); err != nil {
		t.Fatalf("first request: %v", err)
	}
	var limited *RateLimitedError
	if _, err := getMap(client, url); !errors.As(err, &limited) {
		t.Fatalf("second request error = %v, want it deferred before reaching the server", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1: the spent window was announced on the first response", calls)
	}
}

func TestHTTPClientExhausted5xxIsUnavailable(t *testing.T) {
	attempts := 0
	client, url, _, _ := newTestHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("upstream down"))
	})

	_, err := getMap(client, url)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
	if statusCode(err) != http.StatusBadGateway {
		t.Errorf("status = %d, want the 502 still readable", statusCode(err))
	}
	if attempts != client.maxAttempts {
		t.Errorf("attempts = %d, want %d", attempts, client.maxAttempts)
	}
}

func TestHTTPClientClientErrorIsLeftToTheProvider(t *testing.T) {
	attempts := 0
	client, url, _, _ := newTestHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := getMap(client, url)
	if statusCode(err) != http.StatusNotFound || errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want a plain 404 status error", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, a 4xx is not retried", attempts)
	}
}

func TestHTTPClientConnectionFailureIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close() // nothing listens there any more

	limiter, _ := newTestLimiter(0)
	client := newHTTPClient("test", limiter)
	client.sleep = func(time.Duration) {}
	if _, err := getMap(client, url); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestHTTPClientTimeoutIsUnavailable(t *testing.T) {
	attempts := 0
	limiter, _ := newTestLimiter(0)
	client := newHTTPClient("test", limiter)
	client.sleep = func(time.Duration) {}
	client.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		return nil, timeoutError{}
	})}

	_, err := getMap(client, "http://provider.invalid/quote")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
	if attempts != client.maxAttempts {
		t.Errorf("attempts = %d, want %d: a timeout is retried", attempts, client.maxAttempts)
	}
}

func TestHTTPClientStopsWhenTheCallerGivesUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	limiter, _ := newTestLimiter(0)
	client := newHTTPClient("test", limiter)
	client.sleep = func(time.Duration) {}
	client.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		cancel()
		return nil, r.Context().Err()
	})}

	var payload map[string]any
	err := client.getJSON(ctx, "http://provider.invalid/quote", nil, &payload)
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want the cancellation itself", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, a cancelled request is not retried", attempts)
	}
}

func TestHTTPClientMalformedBodyIsAnError(t *testing.T) {
	client, url, _, _ := newTestHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>"))
	})
	_, err := getMap(client, url)
	if err == nil || errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want a decode error", err)
	}
}

// The message reaches logs, so it carries a short excerpt of the body; the
// full body stays on the error for the provider to inspect.
func TestHTTPStatusErrorMessageTruncatesTheBody(t *testing.T) {
	body := strings.Repeat("é", 5000)
	err := &httpStatusError{StatusCode: 502, Body: body}
	message := err.Error()
	if len(message) > 300 || !strings.HasPrefix(message, "status 502: ") || !utf8.ValidString(message) {
		t.Fatalf("message is %d bytes (valid utf-8: %v)", len(message), utf8.ValidString(message))
	}
	if err.Body != body {
		t.Fatal("the body was cut on the error value")
	}
	if short := (&httpStatusError{StatusCode: 404, Body: "nope"}).Error(); short != "status 404: nope" {
		t.Fatalf("short body = %q", short)
	}
}
