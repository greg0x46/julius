package httplog

import (
	"net/http"
	"path"
	"strings"
)

// shouldLog decides whether a finished request gets an access line. Only
// successful requests (status < 400) that the auth layer did not reject are
// ever skipped, so every 4xx/5xx and every rejected credential is logged.
// Skipped as routine noise:
//   - static frontend assets: GET/HEAD served by the SPA catch-all route "/"
//     whose path is under /assets/ or has an asset extension (the path is
//     inspected here and never logged); extensionless SPA navigations such
//     as "/" or "/transactions" are still logged;
//   - trivial CORS preflights (OPTIONS with Access-Control-Request-Method);
//   - the health check;
//   - frequent polling that adds no diagnostics: the session probe and the
//     sync run status poll.
func shouldLog(method, route, urlPath string, status int, auth AuthResult, h http.Header) bool {
	if status >= 400 || auth == AuthRejected {
		return true
	}
	switch route {
	case "GET /health", "GET /api/auth/session", "GET /api/sync-runs/{id}":
		return false
	}
	switch method {
	case http.MethodOptions:
		return h.Get("Access-Control-Request-Method") == ""
	case http.MethodGet, http.MethodHead:
		return route != "/" || !isAssetPath(urlPath)
	}
	return true
}

func isAssetPath(p string) bool {
	if strings.HasPrefix(p, "/assets/") {
		return true
	}
	switch strings.ToLower(path.Ext(p)) {
	case ".js", ".mjs", ".css", ".map", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif",
		".svg", ".ico", ".woff", ".woff2", ".ttf", ".otf", ".eot", ".webmanifest":
		return true
	}
	return false
}
