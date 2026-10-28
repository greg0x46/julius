// Package httplog emits one structured access-log line per HTTP request plus
// a closed set of security events, using only log/slog.
//
// Model: Middleware creates a per-request record (random request_id, route,
// auth outcome, controlled error values) and stores it in the request
// context. Code deeper in the stack annotates the record through SetAuth,
// SetOAuthClient and SetError (or SetWriterError when only the
// ResponseWriter is at hand), and emits events with Event. When the handler
// returns, Middleware writes the "http_request" line; Event lines share its
// request_id, so IPs and user agents appear only on the access line.
//
// The log never contains query strings, bodies, raw URL paths or headers
// other than the few listed on the access line, and every external string is
// sanitized and length-capped. Client-supplied addresses (CF-Connecting-IP,
// X-Forwarded-For) are logged as unverified diagnostics and are never to be
// used for authorization, rate limiting or URL building. The response is
// never altered: the ResponseWriter wrapper only counts status and bytes and
// preserves exactly the optional interfaces of the writer it wraps.
package httplog
