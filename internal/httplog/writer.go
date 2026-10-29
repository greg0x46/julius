package httplog

import (
	"bufio"
	"io"
	"net"
	"net/http"
)

// writer observes the status and size of a response without changing it. It
// never buffers. Like any http.ResponseWriter it is used by one goroutine at
// a time; the middleware reads it only after the handler has returned.
type writer struct {
	http.ResponseWriter
	rec    *record
	status int // 0 until a final status is written
	bytes  int64
}

// Unwrap lets http.ResponseController reach the real connection
// (deadlines, flush, hijack).
func (w *writer) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *writer) httplogRecord() *record { return w.rec }

// WriteHeader mirrors net/http: informational 1xx headers (except 101) are
// not final, and only the first final status counts. The wrapped writer goes
// first so that an invalid code panics before anything is recorded.
func (w *writer) WriteHeader(code int) {
	w.ResponseWriter.WriteHeader(code)
	if w.status == 0 && (code < 100 || code > 199 || code == http.StatusSwitchingProtocols) {
		w.status = code
	}
}

func (w *writer) Write(p []byte) (int, error) {
	w.implicitOK()
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

// WriteString keeps io.WriteString from copying; io.WriteString falls back to
// Write itself when the wrapped writer has no WriteString.
func (w *writer) WriteString(s string) (int, error) {
	w.implicitOK()
	n, err := io.WriteString(w.ResponseWriter, s)
	w.bytes += int64(n)
	return n, err
}

// implicitOK records the 200 net/http sends when a handler writes or flushes
// without calling WriteHeader.
func (w *writer) implicitOK() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
}

// result is the final status and byte count: a handler that returns without
// writing yields net/http's implicit 200.
func (w *writer) result() (status int, bytes int64) {
	if w.status == 0 {
		return http.StatusOK, w.bytes
	}
	return w.status, w.bytes
}

// Optional interfaces. Each part exists only when the wrapped writer has the
// interface, so a type assertion on the wrapper is true iff it is true on the
// original.

type flushPart struct{ w *writer }

// FlushError makes http.ResponseController.Flush report errors of the
// underlying connection, as it would without the wrapper.
func (p flushPart) FlushError() error {
	p.w.implicitOK()
	return http.NewResponseController(p.w.ResponseWriter).Flush()
}

func (p flushPart) Flush() { _ = p.FlushError() }

type hijackPart struct {
	w *writer
	h http.Hijacker
}

// Hijack records 101 when no status was written; bytes sent over the
// hijacked connection are not observable.
func (p hijackPart) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := p.h.Hijack()
	if err == nil && p.w.status == 0 {
		p.w.status = http.StatusSwitchingProtocols
	}
	return conn, rw, err
}

type readFromPart struct {
	w  *writer
	rf io.ReaderFrom
}

func (p readFromPart) ReadFrom(src io.Reader) (int64, error) {
	n, err := p.rf.ReadFrom(src)
	p.w.bytes += n
	if n > 0 {
		p.w.implicitOK() // net/http commits an implicit 200 once bytes flow
	}
	return n, err
}

type (
	wF struct {
		*writer
		flushPart
	}
	wH struct {
		*writer
		hijackPart
	}
	wR struct {
		*writer
		readFromPart
	}
	wFH struct {
		*writer
		flushPart
		hijackPart
	}
	wFR struct {
		*writer
		flushPart
		readFromPart
	}
	wHR struct {
		*writer
		hijackPart
		readFromPart
	}
	wFHR struct {
		*writer
		flushPart
		hijackPart
		readFromPart
	}
)

// wrap returns the observing writer and the http.ResponseWriter to hand to
// the handler, which exposes exactly the optional interfaces of w
// (http.Flusher, http.Hijacker, io.ReaderFrom). http.Pusher is deliberately
// not forwarded.
func wrap(w http.ResponseWriter, rec *record) (*writer, http.ResponseWriter) {
	b := &writer{ResponseWriter: w, rec: rec}
	_, canFlush := w.(http.Flusher)
	h, canHijack := w.(http.Hijacker)
	rf, canReadFrom := w.(io.ReaderFrom)
	f, hj, r := flushPart{b}, hijackPart{b, h}, readFromPart{b, rf}
	switch {
	case canFlush && canHijack && canReadFrom:
		return b, wFHR{b, f, hj, r}
	case canFlush && canHijack:
		return b, wFH{b, f, hj}
	case canFlush && canReadFrom:
		return b, wFR{b, f, r}
	case canHijack && canReadFrom:
		return b, wHR{b, hj, r}
	case canFlush:
		return b, wF{b, f}
	case canHijack:
		return b, wH{b, hj}
	case canReadFrom:
		return b, wR{b, r}
	}
	return b, b
}
