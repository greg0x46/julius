package httpapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// authCall sends a JSON request with an explicit session cookie (or none),
// unlike doJSON, which adds the fixture session.
func authCall(t *testing.T, srv *httptest.Server, method, path, body string, cookie *http.Cookie) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("X-Julius-Request", "1")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp
}

func authStatus(t *testing.T, srv *httptest.Server, method, path, body string, cookie *http.Cookie, want int) *http.Response {
	t.Helper()
	resp := authCall(t, srv, method, path, body, cookie)
	if resp.StatusCode != want {
		t.Fatalf("%s %s = %d, want %d", method, path, resp.StatusCode, want)
	}
	return resp
}

func loginBody(email, password string) string {
	return `{"email":"` + email + `","password":"` + password + `"}`
}

func sessionOf(t *testing.T, srv *httptest.Server, password string) *http.Cookie {
	t.Helper()
	resp := authStatus(t, srv, "POST", "/api/auth/login", loginBody("owner@example.com", password), nil, 200)
	return resp.Cookies()[0]
}

// A login that succeeds costs nothing, so the owner's own logins cannot lock
// the owner out; five rejected ones do, and then even the right password waits.
func TestOnlyFailedLoginsSpendTheLoginBudget(t *testing.T) {
	srv, _ := newLockedTestServer(t)
	good, bad := loginBody("owner@example.com", testPassword), loginBody("owner@example.com", "wrong")
	for i := 0; i < 12; i++ {
		authStatus(t, srv, "POST", "/api/auth/login", good, nil, 200)
	}
	for i := 0; i < 4; i++ {
		authStatus(t, srv, "POST", "/api/auth/login", bad, nil, 401)
	}
	authStatus(t, srv, "POST", "/api/auth/login", good, nil, 200)
	authStatus(t, srv, "POST", "/api/auth/login", bad, nil, 401)
	resp := authStatus(t, srv, "POST", "/api/auth/login", good, nil, 429)
	if resp.Header.Get("Retry-After") != "60" {
		t.Fatalf("Retry-After = %q", resp.Header.Get("Retry-After"))
	}
	// The budget is per e-mail: another address is still checked.
	authStatus(t, srv, "POST", "/api/auth/login", loginBody("other@example.com", "wrong"), nil, 401)
}

// Failed guesses at the login form cannot block a password change, which has
// its own budget keyed by the session. Its successes are free as well.
func TestPasswordChangeHasItsOwnBudget(t *testing.T) {
	t.Run("login failures do not block it", func(t *testing.T) {
		srv, _ := newLockedTestServer(t)
		session := sessionOf(t, srv, testPassword)
		for i := 0; i < 6; i++ {
			want := 401
			if i == 5 {
				want = 429
			}
			authStatus(t, srv, "POST", "/api/auth/login", loginBody("owner@example.com", "wrong"), nil, want)
		}
		authStatus(t, srv, "PUT", "/api/auth/password", `{"current_password":"wrong","password":"another password"}`, session, 401)
		authStatus(t, srv, "PUT", "/api/auth/password", `{"current_password":"`+testPassword+`","password":"another password"}`, session, 200)
	})
	t.Run("its failures do not block login", func(t *testing.T) {
		srv, _ := newLockedTestServer(t)
		session := sessionOf(t, srv, testPassword)
		wrong := `{"current_password":"wrong","password":"another password"}`
		for i := 0; i < 5; i++ {
			authStatus(t, srv, "PUT", "/api/auth/password", wrong, session, 401)
		}
		resp := authStatus(t, srv, "PUT", "/api/auth/password", `{"current_password":"`+testPassword+`","password":"another password"}`, session, 429)
		if resp.Header.Get("Retry-After") != "60" {
			t.Fatalf("Retry-After = %q", resp.Header.Get("Retry-After"))
		}
		authStatus(t, srv, "POST", "/api/auth/login", loginBody("owner@example.com", testPassword), nil, 200)
		// Another session has a budget of its own.
		other := sessionOf(t, srv, testPassword)
		authStatus(t, srv, "PUT", "/api/auth/password", wrong, other, 401)
	})
	t.Run("successful changes are free", func(t *testing.T) {
		srv, _ := newLockedTestServer(t)
		password := testPassword
		for i := 0; i < 7; i++ {
			next := "password number " + string(rune('a'+i))
			session := sessionOf(t, srv, password)
			authStatus(t, srv, "PUT", "/api/auth/password", `{"current_password":"`+password+`","password":"`+next+`"}`, session, 200)
			password = next
		}
	})
}
