package httplog

import (
	"net/http"
	"net/netip"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	invalidValue = "invalid_value"

	// Canonical forms of the headers consulted for forwarded_ip.
	headerCFConnectingIP = "Cf-Connecting-Ip"
	headerXForwardedFor  = "X-Forwarded-For"

	maxForwardedLen = 64
)

// clean makes an external string safe for a log field: invalid UTF-8, control
// characters (including CR, LF, TAB, DEL, C1 such as U+0085), format
// characters (bidi overrides, zero-width), line/paragraph separators and any
// other whitespace become a space; runs of spaces collapse, the ends are
// trimmed and the result is cut to maxRunes runes.
func clean(s string, maxRunes int) string {
	var b strings.Builder
	pendingSpace := false
	runes := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		invalid := r == utf8.RuneError && size == 1
		i += size
		if invalid || unicode.IsControl(r) || unicode.IsSpace(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			pendingSpace = b.Len() > 0
			continue
		}
		need := 1
		if pendingSpace {
			need = 2
		}
		if runes+need > maxRunes {
			break
		}
		if pendingSpace {
			b.WriteByte(' ')
			pendingSpace = false
		}
		b.WriteRune(r)
		runes += need
	}
	return b.String()
}

// token returns s when it matches ^[a-z0-9_.:-]{1,64}$ and "invalid_value"
// otherwise. It guards values that callers must pass as literals.
func token(s string) string {
	if s == "" || len(s) > maxAttrRunes {
		return invalidValue
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' && c != '.' && c != ':' && c != '-' {
			return invalidValue
		}
	}
	return s
}

// optionalToken is token for values that may be absent.
func optionalToken(s string) string {
	if s == "" {
		return ""
	}
	return token(s)
}

// Known returns value when it is one of known, otherwise "other". It lets
// callers log client-provided metadata (such as OAuth grant types) without
// ever reflecting arbitrary input.
func Known(value string, known ...string) string {
	for _, k := range known {
		if value == k {
			return value
		}
	}
	return "other"
}

// peerIP is the address of the TCP peer. A RemoteAddr that is not an IP
// (empty, unix socket) yields "unknown" so the field always exists.
func peerIP(remoteAddr string) string {
	if ap, err := netip.ParseAddrPort(remoteAddr); err == nil {
		return canonicalIP(ap.Addr())
	}
	if addr, err := netip.ParseAddr(remoteAddr); err == nil {
		return canonicalIP(addr)
	}
	return "unknown"
}

// canonicalIP drops any IPv6 zone and unmaps IPv4-in-IPv6.
func canonicalIP(a netip.Addr) string {
	return a.WithZone("").Unmap().String()
}

// forwardedIP is the client address claimed by headers. It is UNVERIFIED and
// for diagnostics only. CF-Connecting-IP wins whenever present, even if
// invalid (no fallback to X-Forwarded-For); otherwise the first element of
// X-Forwarded-For is used. Anything but a clean, specified IP yields "".
func forwardedIP(h http.Header) string {
	if vals, ok := h[headerCFConnectingIP]; ok {
		if len(vals) != 1 {
			return ""
		}
		return parseForwarded(vals[0])
	}
	if vals := h[headerXForwardedFor]; len(vals) > 0 {
		first, _, _ := strings.Cut(vals[0], ",")
		return parseForwarded(first)
	}
	return ""
}

func parseForwarded(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxForwardedLen || strings.Contains(raw, "%") {
		return ""
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return ""
	}
	addr = addr.Unmap()
	if addr.IsUnspecified() {
		return ""
	}
	return addr.String()
}

// knownMethod returns the method when it is a standard HTTP method and
// "OTHER" otherwise.
func knownMethod(m string) string {
	switch m {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete,
		http.MethodHead, http.MethodOptions, http.MethodConnect, http.MethodTrace:
		return m
	}
	return "OTHER"
}
