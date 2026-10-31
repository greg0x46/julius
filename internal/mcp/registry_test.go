package mcp

import (
	"database/sql"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
)

func insertClient(t *testing.T, conn *sql.DB, id string, created time.Time) {
	t.Helper()
	if _, err := conn.Exec(`INSERT INTO oauth_clients(id,name,redirect_uris,created_at) VALUES(?,?,?,?)`, id, "Synthetic", `["https://client.example/callback"]`, db.FormatTime(created)); err != nil {
		t.Fatal(err)
	}
}

// authorize gives the client a connection, which makes it permanent.
func authorize(t *testing.T, conn *sql.DB, client string) {
	t.Helper()
	now := db.FormatTime(time.Now())
	if _, err := conn.Exec(`INSERT INTO mcp_authorizations(id,client_id,scope,approved_scope,revision,created_at,expires_at) VALUES(?,?,?,?,1,?,?)`, uuid.NewString(), client, mcpScope, mcpScope, now, db.FormatTime(time.Now().Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
}

// grantFor stores a consent request for the client that expires at the given time.
func grantFor(t *testing.T, conn *sql.DB, client string, expires time.Time) {
	t.Helper()
	if _, err := conn.Exec(`INSERT INTO oauth_grants(token_hash,kind,client_id,redirect_uri,resource,scope,challenge,state,csrf_hash,revision,expires_at) VALUES(?,'consent',?,'https://client.example/callback','r',?,'c','s','h',1,?)`, uuid.NewString(), client, mcpScope, db.FormatTime(expires)); err != nil {
		t.Fatal(err)
	}
}

// fillRegistry fills the registry to maxClients with clients named c-000,
// c-001, ... created a second apart within the last hour, c-000 the oldest, so
// none is stale. The client that setup registered stays.
func fillRegistry(t *testing.T, conn *sql.DB, authorized bool) {
	t.Helper()
	n := maxClients - clientCount(t, conn)
	start := time.Now().Add(-time.Hour)
	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("c-%03d", i)
		created := start.Add(time.Duration(i) * time.Second)
		if _, err = tx.Exec(`INSERT INTO oauth_clients(id,name,redirect_uris,created_at) VALUES(?,?,?,?)`, id, "Synthetic", `[]`, db.FormatTime(created)); err != nil {
			t.Fatal(err)
		}
		if authorized {
			if _, err = tx.Exec(`INSERT INTO mcp_authorizations(id,client_id,scope,approved_scope,revision,created_at,expires_at) VALUES(?,?,?,?,1,?,?)`, uuid.NewString(), id, mcpScope, mcpScope, db.FormatTime(created), db.FormatTime(created.Add(48*time.Hour))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func clientExists(t *testing.T, conn *sql.DB, id string) bool {
	t.Helper()
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM oauth_clients WHERE id=?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func clientCount(t *testing.T, conn *sql.DB) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM oauth_clients`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func register(t *testing.T, h http.Handler) int {
	t.Helper()
	return jsonRequest(h, "POST", "/oauth/register", `{`+validRedirectField+`}`, nil).Code
}

func TestRegisterEvictsTheOldestNeverAuthorizedClientWhenFull(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		h, _ := newLogged(t, store)
		fillRegistry(t, conn, false)
		authorize(t, conn, "c-000")                               // oldest, but connected
		grantFor(t, conn, "c-001", time.Now().Add(5*time.Minute)) // consent page still open
		grantFor(t, conn, "c-002", time.Now().Add(-time.Minute))  // expired: no protection

		var grantsBefore int
		if err := conn.QueryRow(`SELECT COUNT(*) FROM oauth_grants`).Scan(&grantsBefore); err != nil {
			t.Fatal(err)
		}
		if code := register(t, h); code != 201 {
			t.Fatalf("register on a full registry: %d", code)
		}
		if !clientExists(t, conn, "c-000") || !clientExists(t, conn, "c-001") {
			t.Fatal("an authorized client or one with a live grant was evicted")
		}
		if clientExists(t, conn, "c-002") {
			t.Fatal("the oldest evictable client was kept")
		}
		if n := clientCount(t, conn); n != maxClients {
			t.Fatalf("registry holds %d clients, want %d", n, maxClients)
		}
		// The expired grant went with its client; the live one stayed.
		var grants int
		if err := conn.QueryRow(`SELECT COUNT(*) FROM oauth_grants`).Scan(&grants); err != nil || grants != grantsBefore-1 {
			t.Fatalf("grants = %d (%v), want %d", grants, err, grantsBefore-1)
		}

		if code := register(t, h); code != 201 {
			t.Fatalf("second register: %d", code)
		}
		if clientExists(t, conn, "c-003") || !clientExists(t, conn, "c-004") || clientCount(t, conn) != maxClients {
			t.Fatal("the next oldest evictable client was not the one removed")
		}
	})
}

func TestRegisterKeepsEveryAuthorizedClientAndRefusesWhenNothingIsEvictable(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		h, _ := newLogged(t, store)
		fillRegistry(t, conn, true)
		if code := register(t, h); code != 429 {
			t.Fatalf("register with nothing to evict: %d", code)
		}
		if n := clientCount(t, conn); n != maxClients {
			t.Fatalf("registry holds %d clients, want %d", n, maxClients)
		}
		var authorizations int
		if err := conn.QueryRow(`SELECT COUNT(*) FROM mcp_authorizations`).Scan(&authorizations); err != nil || authorizations != maxClients {
			t.Fatalf("authorizations = %d (%v)", authorizations, err)
		}
	})
}

func TestRegisterPrunesStaleNeverAuthorizedClients(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		h, _ := newLogged(t, store)
		existing := clientCount(t, conn)
		old := time.Now().Add(-staleClientAge - time.Hour)
		for _, id := range []string{"stale", "stale-expired-grant", "stale-authorized", "stale-live-grant"} {
			insertClient(t, conn, id, old)
		}
		insertClient(t, conn, "fresh", time.Now().Add(-time.Hour))
		authorize(t, conn, "stale-authorized")
		grantFor(t, conn, "stale-expired-grant", time.Now().Add(-time.Minute))
		grantFor(t, conn, "stale-live-grant", time.Now().Add(time.Minute))

		if code := register(t, h); code != 201 {
			t.Fatalf("register: %d", code)
		}
		for id, want := range map[string]bool{"stale": false, "stale-expired-grant": false, "stale-authorized": true, "stale-live-grant": true, "fresh": true} {
			if got := clientExists(t, conn, id); got != want {
				t.Errorf("client %q exists = %v, want %v", id, got, want)
			}
		}
		if n := clientCount(t, conn); n != existing+4 {
			t.Fatalf("registry holds %d clients, want %d: 3 kept and the new one", n, existing+4)
		}
	})
}

// The registration bucket (20/min) and the shared one (120/min) do not draw
// on each other.
func TestRegistrationHasItsOwnRateBucket(t *testing.T) {
	const metadata = "/.well-known/oauth-authorization-server"
	t.Run("registration spam leaves the other routes alone", func(t *testing.T) {
		store, _ := setup(t, dbtest.OpenSQLite(t))
		h, sink := newLogged(t, store)
		for i := 0; i < 20; i++ {
			if code := register(t, h); code != 201 {
				t.Fatalf("registration %d: %d", i, code)
			}
		}
		sink.drain()
		w := jsonRequest(h, "POST", "/oauth/register", `{`+validRedirectField+`}`, nil)
		expectOAuthError(t, w, 429, "temporarily_unavailable")
		if w.Header().Get("Retry-After") != "60" {
			t.Fatalf("Retry-After = %q", w.Header().Get("Retry-After"))
		}
		c := sink.one(t)
		c.expectRequest(t, "POST /oauth/register", 429, "none", "not_required")
		c.expectError(t, wantError{code: "temporarily_unavailable", reason: "rate_limited"})
		c.expectEvent(t, "security.rate_limited", map[string]string{"limiter": "oauth_register", "reason": "window_exceeded"})
		if w := oauthRequest(h, "GET", metadata, "", nil, ""); w.Code != 200 {
			t.Fatalf("metadata after registration spam: %d", w.Code)
		}
		if w := oauthRequest(h, "GET", "/oauth/authorize?client_id=unknown", "", nil, ""); w.Code != 400 {
			t.Fatalf("authorize after registration spam: %d (want the usual 400, not 429)", w.Code)
		}
	})
	t.Run("other routes do not spend the registration budget", func(t *testing.T) {
		store, _ := setup(t, dbtest.OpenSQLite(t))
		h, _ := newLogged(t, store)
		for i := 0; i < 120; i++ {
			if w := oauthRequest(h, "GET", metadata, "", nil, ""); w.Code != 200 {
				t.Fatalf("request %d: %d", i, w.Code)
			}
		}
		if w := oauthRequest(h, "GET", metadata, "", nil, ""); w.Code != 429 {
			t.Fatalf("shared bucket not spent: %d", w.Code)
		}
		if code := register(t, h); code != 201 {
			t.Fatalf("registration with the shared bucket spent: %d", code)
		}
	})
}
