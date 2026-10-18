package marketdata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
	"unicode/utf8"
)

// httpClient is the shared shape every provider fetches through: a plain
// http.Client with a fixed timeout — this codebase never uses
// context.WithTimeout (see internal/pluggy/client.go for the same
// convention) — drawing every request from its provider's Limiter, with a
// short linear backoff on 5xx and 429. A provider that asks for a longer
// pause than the backoff covers (Retry-After, an exhausted quota window) is
// not waited out inline: the request fails with a RateLimitedError so the
// caller can defer the rest of that provider's work to a later run instead of
// stalling behind it. A market data lookup has none of the idempotency or
// pagination concerns internal/pluggy's client has to handle.
//
// Failures come back classified the way Service needs them: a network error,
// a timeout or a 5xx that outlasted the retries wraps ErrUnavailable; a 429
// that outlasted them is a *RateLimitedError; any other 4xx is an
// *httpStatusError the provider inspects itself, because only it knows
// whether its 404 means "no such instrument". A response bigger than
// maxResponseBytes is an outage of the provider too, and is not retried.
type httpClient struct {
	client      *http.Client
	maxAttempts int
	backoff     time.Duration
	maxBytes    int64
	sleep       func(time.Duration)
	limiter     *Limiter
	provider    string
}

// maxResponseBytes caps what getJSON reads from one response, so a
// misbehaving or hijacked endpoint cannot make the process buffer an
// unbounded body. The largest answers are full daily histories: fifty years
// of trading days (about 12,600 bars) is roughly 1.4 MB from Yahoo, 1.3 MB
// from CoinGecko and 2.2 MB from brapi, whose rows are the widest. 8 MiB is
// about four times the biggest of those; and with the limiters allowing one
// request at a time per provider, the buffer it costs is small.
const maxResponseBytes = 8 << 20

func newHTTPClient(provider string, limiter *Limiter) *httpClient {
	return &httpClient{
		client:      &http.Client{Timeout: 10 * time.Second},
		maxAttempts: 3,
		backoff:     500 * time.Millisecond,
		maxBytes:    maxResponseBytes,
		sleep:       time.Sleep,
		limiter:     limiter,
		provider:    provider,
	}
}

// httpStatusError is a non-2xx response that got all the way back from the
// wire. Exported fields let a provider distinguish "not found"/"needs a
// token" (4xx, not retried) from a rate limit or outage (429/5xx, retried).
type httpStatusError struct {
	StatusCode int
	Body       string
}

// maxErrorBodyBytes bounds the body excerpt in the message, which is logged.
const maxErrorBodyBytes = 200

func (e *httpStatusError) Error() string {
	body := e.Body
	if len(body) > maxErrorBodyBytes {
		cut := maxErrorBodyBytes
		for cut > 0 && !utf8.RuneStart(body[cut]) {
			cut--
		}
		body = body[:cut] + "..."
	}
	return fmt.Sprintf("status %d: %s", e.StatusCode, body)
}

// statusCode is the HTTP status behind err, or 0 when err is not a response
// that came back with an error status.
func statusCode(err error) int {
	var status *httpStatusError
	if errors.As(err, &status) {
		return status.StatusCode
	}
	return 0
}

// getJSON issues a GET, retrying on 429/5xx with linear backoff, and decodes
// the response body into out with UseNumber() so a caller can turn a price
// into decimal.Decimal without ever passing it through float64 (out is a
// *map[string]any, or a struct whose price fields are json.Number). A body
// over maxBytes fails with ErrUnavailable and is not retried: asking again
// would only download the same oversized answer.
func (c *httpClient) getJSON(ctx context.Context, url string, headers map[string]string, out any) error {
	var lastErr error
	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		release, err := c.limiter.Acquire(ctx)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			release()
			return err
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}

		resp, err := c.client.Do(req)
		if err != nil {
			release()
			if ctxErr := ctx.Err(); ctxErr != nil {
				// The caller gave up; that is not the provider's fault and
				// not worth another attempt.
				return ctxErr
			}
			lastErr = err
			if attempt < c.maxAttempts {
				c.sleep(time.Duration(attempt) * c.backoff)
				continue
			}
			return fmt.Errorf("%w: request failed: %w", ErrUnavailable, err)
		}

		// One byte past the cap tells a body that fits exactly from one that
		// does not.
		content, readErr := io.ReadAll(io.LimitReader(resp.Body, c.maxBytes+1))
		resp.Body.Close()
		release()
		c.limiter.Observe(resp.StatusCode, resp.Header)
		if readErr != nil {
			return fmt.Errorf("%w: read response: %w", ErrUnavailable, readErr)
		}
		if int64(len(content)) > c.maxBytes {
			return fmt.Errorf("%w: response larger than %d bytes", ErrUnavailable, c.maxBytes)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := parseRetryAfter(resp.Header, c.limiter.now())
			if attempt < c.maxAttempts {
				lastErr = &httpStatusError{StatusCode: resp.StatusCode, Body: string(content)}
				if retryAfter == 0 {
					// The provider gave no hint: a short backoff, like a 5xx.
					c.sleep(time.Duration(attempt) * c.backoff)
				}
				// With a hint, Observe has already paused the provider for
				// that long: the next Acquire waits it out, or gives up with
				// a RateLimitedError when it is longer than worth waiting.
				continue
			}
			// Out of attempts: hold the whole provider back and let the
			// caller defer.
			pause := retryAfter
			if pause == 0 {
				pause = defaultPenalty
			}
			c.limiter.Pause(pause)
			return &RateLimitedError{Provider: c.provider, RetryAfter: pause}
		}
		if resp.StatusCode >= 500 {
			lastErr = &httpStatusError{StatusCode: resp.StatusCode, Body: string(content)}
			if attempt < c.maxAttempts {
				c.sleep(time.Duration(attempt) * c.backoff)
				continue
			}
			return fmt.Errorf("%w: %w", ErrUnavailable, lastErr)
		}
		if resp.StatusCode >= 400 {
			return &httpStatusError{StatusCode: resp.StatusCode, Body: string(content)}
		}

		dec := json.NewDecoder(bytes.NewReader(content))
		dec.UseNumber()
		if err := dec.Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		return nil
	}
	return lastErr
}
