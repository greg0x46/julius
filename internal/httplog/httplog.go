package httplog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"
)

// AuthType is how a request authenticated (or tried to).
type AuthType string

// AuthResult is the outcome of the authentication check.
type AuthResult string

const (
	AuthNone    AuthType = "none"
	AuthSession AuthType = "session"
	AuthOAuth   AuthType = "oauth"

	AuthOK          AuthResult = "ok"
	AuthRejected    AuthResult = "rejected"
	AuthNotRequired AuthResult = "not_required"
)

const (
	maxRouteRunes      = 128
	maxUserAgentRunes  = 200
	maxClientNameRunes = 100
	maxAttrRunes       = 64
	clientIDPrefixLen  = 8
	unmatchedRoute     = "unmatched"
)

type recordKey struct{}

// fields is the mutable part of a record.
type fields struct {
	route                        string
	authType                     AuthType
	authResult                   AuthResult
	clientName, clientIDPrefix   string
	errCode, errReason, errField string
	errValues                    []string // replaced as a whole, never mutated in place
}

// record is the per-request state kept in the context. Every access to the
// mutable fields goes through mu: http.TimeoutHandler runs the handler in
// another goroutine that may keep calling the setters after the middleware
// has returned.
type record struct {
	logger *slog.Logger
	id     string // immutable

	mu sync.Mutex
	fields
}

func (rec *record) snapshot() fields {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return rec.fields
}

func recordFrom(ctx context.Context) *record {
	rec, _ := ctx.Value(recordKey{}).(*record)
	return rec
}

// Middleware logs exactly one "http_request" line per finished request, after
// the handler returns (also when it panics; the panic is re-raised
// unchanged). The request id is generated here and is never read from, or
// written to, HTTP headers. logger must be non-nil.
func Middleware(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		panic("httplog: nil logger")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &record{logger: logger, id: newRequestID(), fields: fields{authType: AuthNone, authResult: AuthNotRequired}}
			req := r.WithContext(context.WithValue(r.Context(), recordKey{}, rec))
			// Handlers may rewrite the URL in place; remember what was asked.
			method, urlPath := r.Method, ""
			if r.URL != nil {
				urlPath = r.URL.Path
			}
			sw, rw := wrap(w, rec)
			defer func() {
				p := recover()
				if p != nil {
					rec.markPanic(p)
					if sw.status == 0 {
						sw.status = http.StatusInternalServerError
					}
				}
				logAccess(rec, req, method, urlPath, sw, time.Since(start), p != nil)
				if p != nil {
					panic(p)
				}
			}()
			next.ServeHTTP(rw, req)
		})
	}
}

// WithRoute resolves which pattern of mux matches the request before next
// runs and stores it in the request record, so the access log names the route
// even when next rejects the request before reaching mux or hands mux a copy
// of the request. mux.Handler does not modify r. It is a no-op without
// Middleware in the chain and leaves the record untouched when nothing
// matches.
func WithRoute(mux *http.ServeMux, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rec := recordFrom(r.Context()); rec != nil {
			if _, pattern := mux.Handler(r); pattern != "" {
				rec.mu.Lock()
				rec.route = clean(pattern, maxRouteRunes)
				rec.mu.Unlock()
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RequestID returns the id of the request in ctx, or "" when no Middleware
// ran.
func RequestID(ctx context.Context) string {
	if rec := recordFrom(ctx); rec != nil {
		return rec.id
	}
	return ""
}

// SetAuth records how the request authenticated. Call it from the code that
// really validates credentials. Unknown values are logged as "invalid_value".
func SetAuth(ctx context.Context, t AuthType, r AuthResult) {
	rec := recordFrom(ctx)
	if rec == nil {
		return
	}
	switch t {
	case AuthNone, AuthSession, AuthOAuth:
	default:
		t = invalidValue
	}
	switch r {
	case AuthOK, AuthRejected, AuthNotRequired:
	default:
		r = invalidValue
	}
	rec.mu.Lock()
	rec.authType, rec.authResult = t, r
	rec.mu.Unlock()
}

// SetOAuthClient records the sanitized client name and the first 8 characters
// of the client id.
func SetOAuthClient(ctx context.Context, id, name string) {
	rec := recordFrom(ctx)
	if rec == nil {
		return
	}
	name = clean(name, maxClientNameRunes)
	id = clean(id, clientIDPrefixLen)
	rec.mu.Lock()
	rec.clientName, rec.clientIDPrefix = name, id
	rec.mu.Unlock()
}

// SetError records controlled internal values for the access log. Callers
// pass literals or constants only; each value must match ^[a-z0-9_.:-]{1,64}$
// and anything else (an empty code included) is replaced by "invalid_value".
// An empty reason or field is omitted. The last call replaces all three.
func SetError(ctx context.Context, code, reason, field string) {
	rec := recordFrom(ctx)
	if rec == nil {
		return
	}
	code, reason, field = token(code), optionalToken(reason), optionalToken(field)
	rec.mu.Lock()
	rec.errCode, rec.errReason, rec.errField = code, reason, field
	rec.mu.Unlock()
}

// maxErrorValues caps what SetErrorValues keeps.
const maxErrorValues = 8

// SetErrorValues records up to 8 additional categorical values that explain a
// rejection (for example the known grant types a client sent, already passed
// through Known). Each value is validated like SetError's and replaced by
// "invalid_value" when it does not match; values beyond the eighth are
// dropped and an empty list clears the field. It is independent of SetError.
func SetErrorValues(ctx context.Context, values ...string) {
	rec := recordFrom(ctx)
	if rec == nil {
		return
	}
	var kept []string
	for _, v := range values[:min(len(values), maxErrorValues)] {
		kept = append(kept, token(v))
	}
	rec.mu.Lock()
	rec.errValues = kept
	rec.mu.Unlock()
}

// maxUnwrapDepth bounds the Unwrap walk against cyclic wrappers.
const maxUnwrapDepth = 32

// SetWriterError sets only error_code, for helpers that have a ResponseWriter
// but no *http.Request. It follows the Unwrap chain to this package's writer
// and does nothing when it is not found (for example behind
// http.TimeoutHandler, whose writer does not unwrap).
func SetWriterError(w http.ResponseWriter, code string) {
	for i := 0; i < maxUnwrapDepth && w != nil; i++ {
		if h, ok := w.(interface{ httplogRecord() *record }); ok {
			rec := h.httplogRecord()
			code = token(code)
			rec.mu.Lock()
			rec.errCode = code
			rec.mu.Unlock()
			return
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return
		}
		w = u.Unwrap()
	}
}

// markPanic sets a default error_code without overriding a more specific one.
func (rec *record) markPanic(p any) {
	code := "handler_panic"
	if err, ok := p.(error); ok && errors.Is(err, http.ErrAbortHandler) {
		code = "handler_aborted"
	}
	rec.mu.Lock()
	if rec.errCode == "" {
		rec.errCode = code
	}
	rec.mu.Unlock()
}

// Events: the closed set of names with their levels, the allowlisted
// attribute keys (in output order) and each key's maximum length in runes.
var eventLevels = map[string]slog.Level{
	"auth.login.success":         slog.LevelInfo,
	"auth.login.failure":         slog.LevelWarn,
	"auth.logout":                slog.LevelInfo,
	"oauth.client.registered":    slog.LevelInfo,
	"oauth.consent.granted":      slog.LevelInfo,
	"oauth.consent.denied":       slog.LevelInfo,
	"oauth.credentials.rejected": slog.LevelWarn,
	"oauth.token.revoked":        slog.LevelInfo,
	"security.rate_limited":      slog.LevelWarn,
	"security.invalid_origin":    slog.LevelWarn,
}

var eventAttrs = [...]struct {
	key      string
	maxRunes int
}{
	{"reason", maxAttrRunes},
	{"limiter", maxAttrRunes},
	{"scope", maxAttrRunes},
	{"client_name", maxClientNameRunes},
	{"client_id_prefix", clientIDPrefixLen},
}

// Event emits one structured security event carrying the request's
// request_id; the client address and user agent are only on the access line.
// A name outside the closed set drops the event; attributes outside the
// allowlist (reason, limiter, scope, client_name, client_id_prefix) and
// non-string values are dropped, the rest are sanitized and truncated. It
// does nothing without Middleware in the chain.
func Event(ctx context.Context, name string, attrs ...slog.Attr) {
	rec := recordFrom(ctx)
	if rec == nil {
		return
	}
	level, ok := eventLevels[name]
	if !ok || !rec.logger.Enabled(ctx, level) {
		return
	}
	var values [len(eventAttrs)]string
	for _, a := range attrs {
		v := a.Value.Resolve()
		if v.Kind() != slog.KindString {
			continue
		}
		for i, allowed := range eventAttrs {
			if a.Key == allowed.key {
				values[i] = clean(v.String(), allowed.maxRunes)
			}
		}
	}
	out := make([]slog.Attr, 0, 2+len(values))
	out = append(out, slog.String("event", name), slog.String("request_id", rec.id))
	for i, v := range values {
		if v != "" {
			out = append(out, slog.String(eventAttrs[i].key, v))
		}
	}
	rec.logger.LogAttrs(ctx, level, name, out...)
}

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // never returns an error since Go 1.24
	return hex.EncodeToString(b[:])
}

func levelFor(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	}
	return slog.LevelInfo
}

// durationMS is the duration in milliseconds with microsecond precision.
func durationMS(d time.Duration) float64 {
	return math.Round(float64(d)/float64(time.Microsecond)) / 1000
}

// logAccess writes the "http_request" line unless it is routine noise.
func logAccess(rec *record, req *http.Request, method, urlPath string, sw *writer, took time.Duration, panicked bool) {
	status, size := sw.result()

	s := rec.snapshot()

	route := s.route
	if route == "" {
		route = clean(req.Pattern, maxRouteRunes)
	}
	if route == "" {
		route = unmatchedRoute
	}
	if !panicked && !shouldLog(method, route, urlPath, status, s.authResult, req.Header) {
		return
	}

	level := levelFor(status)
	if panicked {
		level = slog.LevelError
	}
	ctx := context.WithoutCancel(req.Context())
	if !rec.logger.Enabled(ctx, level) {
		return
	}

	attrs := make([]slog.Attr, 0, 17)
	attrs = append(attrs,
		slog.String("event", "http_request"),
		slog.String("request_id", rec.id),
		slog.String("method", knownMethod(method)),
		slog.String("route", route),
		slog.Int("status", status),
		slog.Float64("duration_ms", durationMS(took)),
		slog.Int64("response_bytes", size),
		slog.String("peer_ip", peerIP(req.RemoteAddr)),
	)
	if ip := forwardedIP(req.Header); ip != "" {
		attrs = append(attrs, slog.String("forwarded_ip", ip))
	}
	if ua := clean(req.Header.Get("User-Agent"), maxUserAgentRunes); ua != "" {
		attrs = append(attrs, slog.String("user_agent", ua))
	}
	attrs = append(attrs,
		slog.String("auth_type", string(s.authType)),
		slog.String("auth_result", string(s.authResult)),
	)
	for _, o := range [...]struct{ key, value string }{
		{"client_name", s.clientName},
		{"client_id_prefix", s.clientIDPrefix},
		{"error_code", s.errCode},
		{"error_reason", s.errReason},
		{"invalid_field", s.errField},
	} {
		if o.value != "" {
			attrs = append(attrs, slog.String(o.key, o.value))
		}
	}
	if len(s.errValues) > 0 {
		attrs = append(attrs, slog.Any("invalid_values", s.errValues))
	}
	rec.logger.LogAttrs(ctx, level, "http_request", attrs...)
}
