# Logs and diagnostics

Julius logs to stderr as JSON (Go `log/slog`), one `http_request` line per
request and one short event line for each security-relevant action: sign-in,
sign-out, OAuth client registration, consent, token revocation, rate limiting
and rejected origins. Routine requests that succeed are skipped (static assets,
`GET /health`, session and sync-status polling); every 4xx/5xx is logged. An
event line shares the `request_id` of its request line. The server generates
that id and never reads it from, or returns it in, a header.

A request line has the method, the route pattern (never path values or the
query string), status, duration, response size, the TCP peer address, the
unverified forwarded address when a proxy header is present, the user agent,
the authentication outcome and, for a rejection, controlled `error_code`,
`error_reason`, `invalid_field` and `invalid_values`. These last fields say
which validation failed, so a rejected request can be diagnosed from the log
alone.

Never logged: passwords, emails, cookies, tokens, authorization codes, OAuth
`state` and PKCE values, `Authorization` headers, query strings, request or
response bodies, redirect URIs and client secrets. Free text from clients (user
agent, client name) is sanitized to one line and truncated.

Go's standard `log` shares stderr and writes plain-text lines (startup,
background work), so parse with `fromjson?` to skip them:

```sh
# systemd (replace <your-unit>)
journalctl -u <your-unit> -o cat | jq -R 'fromjson? | select(.event=="http_request")'

# Docker (the log goes to stderr, hence 2>&1)
docker logs <container> 2>&1 | jq -R 'fromjson? | select(.event=="http_request")'
```

Useful filters for the `jq` expression:

```sh
# why was a client registration refused?
select(.error_code=="invalid_client_metadata") | {request_id,error_reason,invalid_field,invalid_values}
# server errors
select(.status>=500)
# everything one request logged
select(.request_id=="<id>")
# security events: logins, consent, rate limits, rejected origins
select((.event // "") | startswith("security.") or startswith("auth.") or startswith("oauth."))
```

Retention belongs to the host (journald, the container runtime). The fields,
events and error catalog are in the
[reference](../development/contexts/settings-authentication.md#http-access-log-and-security-events).
