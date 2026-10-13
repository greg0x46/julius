package auth

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/settings"
)

const password = "correct horse battery staple"

func databases(t *testing.T, test func(*testing.T, *sql.DB)) {
	t.Helper()
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			dsn := filepath.Join(t.TempDir(), "auth.db")
			if dialect == "postgres" {
				base := os.Getenv("JULIUS_TEST_POSTGRES_DSN")
				if base == "" {
					t.Skip("JULIUS_TEST_POSTGRES_DSN not set")
				}
				raw, err := sql.Open("pgx", base)
				if err != nil {
					t.Fatal(err)
				}
				defer raw.Close()
				schema := "auth_test_" + fmt.Sprintf("%x", uuid.New())
				if _, err := raw.Exec(`CREATE SCHEMA ` + schema); err != nil {
					t.Fatal(err)
				}
				defer raw.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
				u, err := url.Parse(base)
				if err != nil {
					t.Fatal(err)
				}
				q := u.Query()
				q.Set("search_path", schema)
				u.RawQuery = q.Encode()
				dsn = u.String()
			}
			conn, err := db.Open(dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			test(t, conn)
		})
	}
}
func TestMigrationPreservesSecretsAndRollsBack(t *testing.T) {
	databases(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		store := NewStore(conn)
		master := bytes.Repeat([]byte{9}, 32)
		old, err := settings.Setup(ctx, conn, password)
		if err != nil {
			t.Fatal(err)
		}
		if err := settings.Set(ctx, conn, "pluggy.client_secret", "sensitive", true, old); err != nil {
			t.Fatal(err)
		}
		if err := settings.Set(ctx, conn, "other_secret", "other", true, old); err != nil {
			t.Fatal(err)
		}
		if err := settings.Set(ctx, conn, "plain", "unchanged", false, nil); err != nil {
			t.Fatal(err)
		}
		wrong := "incorrect password"
		if err := store.Initialize(ctx, "owner@example.com", password, master, &wrong); err == nil {
			t.Fatal("wrong legacy password accepted")
		}
		// Corruption after an earlier value has been rewritten must roll the transaction back.
		var ciphertext string
		if err := conn.QueryRow(`SELECT value FROM settings WHERE key='other_secret'`).Scan(&ciphertext); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(`UPDATE settings SET value='broken' WHERE key='other_secret'`); err != nil {
			t.Fatal(err)
		}
		legacy := password
		if err := store.Initialize(ctx, "owner@example.com", password, master, &legacy); err == nil {
			t.Fatal("corrupt secret accepted")
		}
		var owners int
		if err := conn.QueryRow(`SELECT COUNT(*) FROM auth_owner`).Scan(&owners); err != nil || owners != 0 {
			t.Fatalf("partial owner: %d %v", owners, err)
		}
		got, _, err := settings.Get(ctx, conn, "pluggy.client_secret", old)
		if err != nil || got != "sensitive" {
			t.Fatalf("rollback failed: %q %v", got, err)
		}
		if _, err := conn.Exec(`UPDATE settings SET value=? WHERE key='other_secret'`, ciphertext); err != nil {
			t.Fatal(err)
		}
		if err := store.Initialize(ctx, " Owner@Example.com ", password, master, &legacy); err != nil {
			t.Fatal(err)
		}
		if err := store.Ready(ctx, master); err != nil {
			t.Fatal(err)
		}
		for name, want := range map[string]string{"pluggy.client_secret": "sensitive", "other_secret": "other", "plain": "unchanged"} {
			got, _, err := settings.Get(ctx, conn, name, master)
			if err != nil || got != want {
				t.Fatalf("%s: %q %v", name, got, err)
			}
		}
		if err := store.Initialize(ctx, "owner@example.com", password, master, &legacy); err == nil {
			t.Fatal("repeat migration accepted")
		}
		if err := store.Ready(ctx, bytes.Repeat([]byte{8}, 32)); err == nil {
			t.Fatal("wrong master accepted")
		}
		configured, err := settings.IsConfigured(ctx, conn)
		if err != nil || configured {
			t.Fatal("legacy verifier still present")
		}
	})
}
func TestSessionsExpiryResetAndRestart(t *testing.T) {
	databases(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		s := NewStore(conn)
		master := bytes.Repeat([]byte{1}, 32)
		now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
		s.Now = func() time.Time { return now }
		if err := s.Initialize(ctx, "owner@example.com", password, master, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Login(ctx, "someone@example.com", password); !errors.Is(err, ErrCredentials) {
			t.Fatal(err)
		}
		if _, err := s.Login(ctx, "owner@example.com", "wrong"); !errors.Is(err, ErrCredentials) {
			t.Fatal(err)
		}
		login := func() string {
			t.Helper()
			token, err := s.Login(ctx, "OWNER@example.com", password)
			if err != nil {
				t.Fatal(err)
			}
			return token
		}
		first, second := login(), login()
		var stored string
		if err := conn.QueryRow(`SELECT token_hash FROM auth_sessions LIMIT 1`).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored == first || stored == second {
			t.Fatal("raw session persisted")
		}
		restarted := NewStore(conn)
		restarted.Now = s.Now
		if _, err := restarted.Authenticate(ctx, first); err != nil {
			t.Fatal(err)
		}
		if err := s.Logout(ctx, first); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Authenticate(ctx, first); !errors.Is(err, ErrSession) {
			t.Fatal(err)
		}
		if _, err := s.Authenticate(ctx, second); err != nil {
			t.Fatal(err)
		}
		now = now.Add(IdleTimeout)
		if _, err := s.Authenticate(ctx, second); !errors.Is(err, ErrSession) {
			t.Fatal("idle session accepted", err)
		}
		active := login()
		for i := 0; i < 14; i++ {
			now = now.Add(12 * time.Hour)
			_, err := s.Authenticate(ctx, active)
			if i < 13 && err != nil {
				t.Fatal(err)
			}
			if i == 13 && !errors.Is(err, ErrSession) {
				t.Fatal("absolute expiry ignored", err)
			}
		}
		a, b := login(), login()
		if err := s.ChangePassword(ctx, "wrong", password+" new"); !errors.Is(err, ErrCredentials) {
			t.Fatal(err)
		}
		if _, err := s.Authenticate(ctx, a); err != nil {
			t.Fatal("wrong password revoked session", err)
		}
		if err := s.ChangePassword(ctx, password, password+" new"); err != nil {
			t.Fatal(err)
		}
		for _, token := range []string{a, b} {
			if _, err := s.Authenticate(ctx, token); !errors.Is(err, ErrSession) {
				t.Fatal(err)
			}
		}
		token, err := s.Login(ctx, "owner@example.com", password+" new")
		if err != nil {
			t.Fatal(err)
		}
		if err := s.ResetPassword(ctx, "x"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Login(ctx, "owner@example.com", "x"); err != nil {
			t.Fatal("one-character password did not work", err)
		}
		if _, err := s.Authenticate(ctx, token); !errors.Is(err, ErrSession) {
			t.Fatal(err)
		}
		if err := s.Ready(ctx, master); err != nil {
			t.Fatal("reset changed encryption", err)
		}
		// A session inserted by a login racing a reset carries the obsolete revision.
		if _, err := conn.Exec(`INSERT INTO auth_sessions(token_hash,owner_id,revision,created_at,last_seen_at,expires_at) VALUES(?,1,1,?,?,?)`, digest(token), db.FormatTime(now), db.FormatTime(now), db.FormatTime(now.Add(Lifetime))); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Authenticate(ctx, token); !errors.Is(err, ErrSession) {
			t.Fatal("stale revision accepted", err)
		}
	})
}
func TestConfiguration(t *testing.T) {
	for _, value := range []string{"", "http://example.com", "https://user@example.com", "https://example.com/path", "https://example.com?x=1"} {
		if (Config{PublicURL: value}).Validate() == nil {
			t.Errorf("accepted %q", value)
		}
	}
	for _, value := range []string{"https://example.com", "http://localhost:5173", "http://127.0.0.1:4200", "http://[::1]:4200"} {
		if err := (Config{PublicURL: value}).Validate(); err != nil {
			t.Errorf("%q: %v", value, err)
		}
	}
	if ValidPassword("") {
		t.Error("empty password accepted")
	}
	for _, value := range []string{"x", " ", string(bytes.Repeat([]byte{'x'}, 1024))} {
		if !ValidPassword(value) {
			t.Errorf("non-empty password rejected: length %d", len(value))
		}
	}
}
