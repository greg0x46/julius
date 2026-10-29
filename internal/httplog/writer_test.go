package httplog

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/textproto"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestInterfacePreservationMatrix(t *testing.T) {
	for _, flusher := range []bool{false, true} {
		for _, hijacker := range []bool{false, true} {
			for _, readerFrom := range []bool{false, true} {
				name := fmt.Sprintf("flusher=%v/hijacker=%v/readerFrom=%v", flusher, hijacker, readerFrom)
				t.Run(name, func(t *testing.T) {
					fw := newFakeWriter(flusher, hijacker, readerFrom)
					l, s := newTestLogger()
					handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						_, isFlusher := w.(http.Flusher)
						_, isHijacker := w.(http.Hijacker)
						_, isReaderFrom := w.(io.ReaderFrom)
						if isFlusher != flusher || isHijacker != hijacker || isReaderFrom != readerFrom {
							t.Errorf("wrapper has flusher=%v hijacker=%v readerFrom=%v", isFlusher, isHijacker, isReaderFrom)
						}
						if u, ok := w.(interface{ Unwrap() http.ResponseWriter }); !ok || u.Unwrap() != fw.ResponseWriter {
							t.Error("Unwrap does not return the original writer")
						}
						if f, ok := w.(http.Flusher); ok {
							f.Flush()
							if fw.flush.flushes != 1 {
								t.Errorf("flushes = %d", fw.flush.flushes)
							}
						}
						if h, ok := w.(http.Hijacker); ok {
							if _, _, err := h.Hijack(); err != nil || fw.hij.hijacks != 1 {
								t.Errorf("hijack err=%v calls=%d", err, fw.hij.hijacks)
							}
						}
						if n, err := io.Copy(w, plainReader("12345")); n != 5 || err != nil {
							t.Errorf("io.Copy = %d, %v", n, err)
						}
						wantRF := 0
						if readerFrom {
							wantRF = 1
						}
						if fw.rf.calls != wantRF {
							t.Errorf("ReadFrom calls = %d, want %d", fw.rf.calls, wantRF)
						}
						if fw.base.body.String() != "12345" {
							t.Errorf("body = %q", fw.base.body.String())
						}
					})
					Middleware(l)(handler).ServeHTTP(fw.ResponseWriter, newReq("GET", "/x"))
					m := decode(t, s.lines()[0])
					wantStatus := float64(200)
					if hijacker && !flusher {
						wantStatus = 101 // hijacked before any status was written (Flush would have sent 200)
					}
					if m["response_bytes"] != float64(5) {
						t.Errorf("response_bytes = %v", m["response_bytes"])
					}
					if m["status"] != wantStatus {
						t.Errorf("status = %v, want %v", m["status"], wantStatus)
					}
				})
			}
		}
	}
}

func TestStatusTracking(t *testing.T) {
	tests := []struct {
		name  string
		fn    func(w http.ResponseWriter)
		want  float64
		codes []int // WriteHeader calls the wrapped writer must receive
	}{
		{"1xx is not final", func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusEarlyHints)
			w.WriteHeader(102)
			w.WriteHeader(http.StatusCreated)
		}, 201, []int{103, 102, 201}},
		{"1xx then implicit 200", func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusContinue)
			_, _ = w.Write([]byte("x"))
		}, 200, []int{100}},
		{"101 is final", func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusSwitchingProtocols)
			w.WriteHeader(200)
		}, 101, []int{101, 200}},
		{"first final status wins", func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusTeapot)
			w.WriteHeader(http.StatusInternalServerError)
		}, 418, []int{418, 500}},
		{"write after status keeps it", func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, "x")
		}, 202, []int{202}},
		{"only 1xx", func(w http.ResponseWriter) { w.WriteHeader(http.StatusEarlyHints) }, 200, []int{103}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fb := &fakeBase{}
			l, s := newTestLogger()
			Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { tc.fn(w) })).ServeHTTP(fb, newReq("GET", "/x"))
			if m := decode(t, s.lines()[0]); m["status"] != tc.want {
				t.Errorf("status = %v, want %v", m["status"], tc.want)
			}
			if !reflect.DeepEqual(fb.codes, tc.codes) {
				t.Errorf("wrapped writer got %v, want %v", fb.codes, tc.codes)
			}
		})
	}
}

func TestFlushBeforeWriteHeaderIsImplicit200(t *testing.T) {
	rr := httptest.NewRecorder()
	l, s := newTestLogger()
	Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		w.WriteHeader(http.StatusInternalServerError) // too late, as in net/http
	})).ServeHTTP(rr, newReq("GET", "/x"))
	if m := decode(t, s.lines()[0]); m["status"] != float64(200) {
		t.Fatalf("status = %v", m["status"])
	}
	if rr.Code != 200 || !rr.Flushed {
		t.Fatalf("recorder code=%d flushed=%v", rr.Code, rr.Flushed)
	}
}

func TestInformationalOverRealServer(t *testing.T) {
	l, s := newTestLogger()
	srv := newServer(t, Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Link", "</style.css>; rel=preload")
		w.WriteHeader(http.StatusEarlyHints)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "body")
	})))
	var got103 bool
	trace := &httptrace.ClientTrace{Got1xxResponse: func(code int, _ textproto.MIMEHeader) error {
		got103 = code == 103
		return nil
	}}
	req, _ := http.NewRequest("GET", srv.URL+"/x", nil)
	resp, err := http.DefaultClient.Do(req.WithContext(httptrace.WithClientTrace(req.Context(), trace)))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "body" || !got103 {
		t.Fatalf("status=%d body=%q 103=%v", resp.StatusCode, body, got103)
	}
	if m := decode(t, s.waitLines(t, 1)[0]); m["status"] != float64(200) || m["response_bytes"] != float64(4) {
		t.Fatalf("line: %v", m)
	}
}

// compareHandlers serves the same handler directly and behind the
// middleware over real connections and asserts the responses are identical.
func TestResponseIdenticalWithAndWithoutMiddleware(t *testing.T) {
	content := bytes.Repeat([]byte("0123456789"), 5000)
	modTime := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	file := filepath.Join(t.TempDir(), "data.bin")
	if err := os.WriteFile(file, content, 0o600); err != nil {
		t.Fatal(err)
	}
	handlers := map[string]http.HandlerFunc{
		"explicit status headers body": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Custom", "v")
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"ok":true}`)
		},
		"implicit 200 with sniffing": func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "<html>hi</html>") },
		"empty":                      func(w http.ResponseWriter, r *http.Request) {},
		"no content":                 func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) },
		"http.Error":                 func(w http.ResponseWriter, r *http.Request) { http.Error(w, "nope", http.StatusBadRequest) },
		"not found":                  func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) },
		"redirect": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/elsewhere?a=1", http.StatusTemporaryRedirect)
		},
		"serve content": func(w http.ResponseWriter, r *http.Request) {
			http.ServeContent(w, r, "data.bin", modTime, bytes.NewReader(content))
		},
		"serve file": func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, file) },
		"io.Copy from file": func(w http.ResponseWriter, r *http.Request) {
			f, _ := os.Open(file)
			defer f.Close()
			_, _ = io.Copy(w, f)
		},
		"flush then body": func(w http.ResponseWriter, r *http.Request) {
			w.(http.Flusher).Flush()
			_, _ = io.WriteString(w, "after flush")
		},
		"large chunked body": func(w http.ResponseWriter, r *http.Request) {
			for i := 0; i < 20; i++ {
				_, _ = w.Write(content[:3000])
			}
		},
		"early hints": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Link", "</a.css>; rel=preload")
			w.WriteHeader(http.StatusEarlyHints)
			w.Header().Set("X-After", "1")
			_, _ = io.WriteString(w, "final")
		},
	}
	l, _ := newTestLogger()
	type snapshot struct {
		status int
		header http.Header
		body   string
	}
	fetch := func(t *testing.T, url, method, rng string) snapshot {
		t.Helper()
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		req, _ := http.NewRequest(method, url, nil)
		if rng != "" {
			req.Header.Set("Range", rng)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		h := resp.Header.Clone()
		h.Del("Date")
		return snapshot{resp.StatusCode, h, string(body)}
	}
	for name, h := range handlers {
		for _, req := range []struct{ method, rng string }{{"GET", ""}, {"HEAD", ""}, {"GET", "bytes=5-14"}} {
			t.Run(name+"/"+req.method+req.rng, func(t *testing.T) {
				direct := newServer(t, h)
				wrapped := newServer(t, Middleware(l)(h))
				want := fetch(t, direct.URL+"/p", req.method, req.rng)
				got := fetch(t, wrapped.URL+"/p", req.method, req.rng)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("responses differ\nwithout: %+v\nwith:    %+v", want, got)
				}
			})
		}
	}
}

func TestStreamingIsNotBuffered(t *testing.T) {
	for _, flush := range []struct {
		name string
		fn   func(w http.ResponseWriter) error
	}{
		{"Flusher", func(w http.ResponseWriter) error { w.(http.Flusher).Flush(); return nil }},
		{"ResponseController", func(w http.ResponseWriter) error { return http.NewResponseController(w).Flush() }},
	} {
		t.Run(flush.name, func(t *testing.T) {
			l, s := newTestLogger()
			release := make(chan struct{})
			var once = func() func() {
				done := false
				return func() {
					if !done {
						done = true
						close(release)
					}
				}
			}()
			defer once()
			flushErr := make(chan error, 1)
			srv := newServer(t, Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: one\n\n")
				flushErr <- flush.fn(w)
				<-release
				_, _ = io.WriteString(w, "data: two\n\n")
			})))
			resp, err := http.Get(srv.URL + "/events")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			first := make(chan string, 1)
			br := bufio.NewReader(resp.Body)
			go func() {
				line, _ := br.ReadString('\n')
				first <- line
			}()
			select {
			case line := <-first:
				if line != "data: one\n" {
					t.Fatalf("first chunk = %q", line)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("first chunk did not arrive before the handler finished: response is buffered")
			}
			if err := <-flushErr; err != nil {
				t.Fatalf("flush error: %v", err)
			}
			once()
			rest, _ := io.ReadAll(br)
			if string(rest) != "\ndata: two\n\n" {
				t.Fatalf("rest = %q", rest)
			}
			if m := decode(t, s.waitLines(t, 1)[0]); m["status"] != float64(200) || m["response_bytes"] != float64(22) {
				t.Fatalf("line: %v", m)
			}
		})
	}
}

func TestResponseControllerDeadlinesReachConnection(t *testing.T) {
	l, _ := newTestLogger()
	result := make(chan error, 1)
	srv := newServer(t, Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		err := errors.Join(
			rc.SetReadDeadline(time.Now().Add(time.Minute)),
			rc.SetWriteDeadline(time.Now().Add(time.Minute)),
			rc.Flush(),
		)
		result <- err
	})))
	resp, err := http.Get(srv.URL + "/x")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if err := <-result; err != nil {
		t.Fatalf("ResponseController through the wrapper: %v", err)
	}
}

func TestResponseControllerThroughForeignWrapper(t *testing.T) {
	// A foreign wrapper without Flusher that still unwraps to a recorder: the
	// middleware must not hide the way down to the real writer.
	rr := httptest.NewRecorder()
	l, s := newTestLogger()
	Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Flusher); ok {
			t.Error("wrapper must not claim Flusher the foreign writer lacks")
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("Flush through Unwrap chain: %v", err)
		}
		_, _ = io.WriteString(w, "x")
	})).ServeHTTP(unwrapOnly{rr}, newReq("GET", "/x"))
	if !rr.Flushed || rr.Body.String() != "x" {
		t.Fatalf("flushed=%v body=%q", rr.Flushed, rr.Body.String())
	}
	if m := decode(t, s.lines()[0]); m["response_bytes"] != float64(1) {
		t.Fatalf("line: %v", m)
	}
}

func TestReadFromCountsBytesOverRealServer(t *testing.T) {
	content := bytes.Repeat([]byte("abcdefghij"), 20000) // 200 kB
	file := filepath.Join(t.TempDir(), "big.bin")
	if err := os.WriteFile(file, content, 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		fn    http.HandlerFunc
		rng   string
		bytes float64
		code  float64
	}{
		{"io.Copy from *os.File", func(w http.ResponseWriter, r *http.Request) {
			f, err := os.Open(file)
			if err != nil {
				t.Error(err)
				return
			}
			defer f.Close()
			if _, ok := w.(io.ReaderFrom); !ok {
				t.Error("wrapper hides io.ReaderFrom of the real writer")
			}
			_, _ = io.Copy(w, f)
		}, "", 200000, 200},
		{"ServeContent", func(w http.ResponseWriter, r *http.Request) {
			http.ServeContent(w, r, "big.bin", time.Time{}, bytes.NewReader(content))
		}, "", 200000, 200},
		{"ServeContent range", func(w http.ResponseWriter, r *http.Request) {
			http.ServeContent(w, r, "big.bin", time.Time{}, bytes.NewReader(content))
		}, "bytes=100-199", 100, 206},
		{"ServeFile", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, file) }, "", 200000, 200},
		{"mixed Write and ReadFrom", func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "header|")
			_, _ = io.Copy(w, plainReader("payload"))
		}, "", 14, 200},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l, s := newTestLogger()
			srv := newServer(t, Middleware(l)(tc.fn))
			req, _ := http.NewRequest("GET", srv.URL+"/f", nil)
			if tc.rng != "" {
				req.Header.Set("Range", tc.rng)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			m := decode(t, s.waitLines(t, 1)[0])
			if m["response_bytes"] != tc.bytes || m["status"] != tc.code || float64(len(body)) != tc.bytes {
				t.Fatalf("logged bytes=%v status=%v, received %d bytes (want %v)", m["response_bytes"], m["status"], len(body), tc.bytes)
			}
		})
	}
}

func TestHijackOverRealServer(t *testing.T) {
	for _, via := range []string{"Hijacker", "ResponseController"} {
		t.Run(via, func(t *testing.T) {
			l, s := newTestLogger()
			srv := newServer(t, Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var conn net.Conn
				var rw *bufio.ReadWriter
				var err error
				if via == "Hijacker" {
					conn, rw, err = w.(http.Hijacker).Hijack()
				} else {
					conn, rw, err = http.NewResponseController(w).Hijack()
				}
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				_, _ = rw.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 6\r\nConnection: close\r\n\r\nraw-ok")
				_ = rw.Flush()
			})))
			conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_, _ = io.WriteString(conn, "GET /ws HTTP/1.1\r\nHost: x\r\n\r\n")
			raw, _ := io.ReadAll(conn)
			if !strings.HasSuffix(string(raw), "raw-ok") {
				t.Fatalf("raw response = %q", raw)
			}
			m := decode(t, s.waitLines(t, 1)[0])
			if m["status"] != float64(101) || m["response_bytes"] != float64(0) {
				t.Fatalf("line: %v", m)
			}
		})
	}
}

func TestNestedAndAlreadyWrappedWriters(t *testing.T) {
	l, s := newTestLogger()
	inner := Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, "abc")
		w.(http.Flusher).Flush()
	}))
	rr := httptest.NewRecorder()
	Middleware(l)(inner).ServeHTTP(rr, newReq("GET", "/x"))
	lines := s.lines()
	if len(lines) != 2 {
		t.Fatalf("lines: %q", s.String())
	}
	for _, line := range lines {
		if m := decode(t, line); m["status"] != float64(202) || m["response_bytes"] != float64(3) {
			t.Fatalf("line: %v", m)
		}
	}
	if rr.Code != 202 || rr.Body.String() != "abc" || !rr.Flushed {
		t.Fatalf("recorder: %d %q flushed=%v", rr.Code, rr.Body.String(), rr.Flushed)
	}
}

func TestWriteStringCountsBytes(t *testing.T) {
	fb := &fakeBase{}
	l, s := newTestLogger()
	Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, err := io.WriteString(w, "héllo")
		if n != 6 || err != nil {
			t.Errorf("WriteString = %d, %v", n, err)
		}
	})).ServeHTTP(fb, newReq("GET", "/x"))
	if fb.body.String() != "héllo" {
		t.Fatalf("body = %q", fb.body.String())
	}
	if m := decode(t, s.lines()[0]); m["response_bytes"] != float64(6) || m["status"] != float64(200) {
		t.Fatalf("line: %v", m)
	}
}

// plainReader hides WriterTo so io.Copy goes through the writer's ReadFrom.
func plainReader(s string) io.Reader { return struct{ io.Reader }{strings.NewReader(s)} }

func TestHTTP2StreamingAndInterfaces(t *testing.T) {
	l, s := newTestLogger()
	release := make(chan struct{})
	var hijacker bool
	srv := httptest.NewUnstartedServer(Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hijacker = w.(http.Hijacker)
		_, _ = io.WriteString(w, "one\n")
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush: %v", err)
		}
		<-release
		_, _ = io.WriteString(w, "two\n")
	})))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()

	resp, err := srv.Client().Get(srv.URL + "/h2")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.ProtoMajor != 2 {
		t.Skipf("HTTP/2 not negotiated (%s)", resp.Proto)
	}
	first := make(chan string, 1)
	br := bufio.NewReader(resp.Body)
	go func() {
		line, _ := br.ReadString('\n')
		first <- line
	}()
	select {
	case line := <-first:
		if line != "one\n" {
			t.Fatalf("first = %q", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP/2 response is buffered")
	}
	close(release)
	rest, _ := io.ReadAll(br)
	if string(rest) != "two\n" {
		t.Fatalf("rest = %q", rest)
	}
	if hijacker {
		t.Error("wrapper claims Hijacker on an HTTP/2 stream")
	}
	if m := decode(t, s.waitLines(t, 1)[0]); m["status"] != float64(200) || m["response_bytes"] != float64(8) {
		t.Fatalf("line: %v", m)
	}
}
