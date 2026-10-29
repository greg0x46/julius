package mcpaccess

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db/dbtest"
)

func TestAuthorizationLifecycle(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		owner := auth.NewStore(conn)
		if err := owner.Initialize(ctx, "owner@example.com", "synthetic-password", bytes.Repeat([]byte{42}, 32), nil); err != nil {
			t.Fatal(err)
		}
		var revision int
		if err := conn.QueryRow(`SELECT revision FROM auth_owner WHERE id=1`).Scan(&revision); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{"first", "second"} {
			if _, err := conn.Exec(`INSERT INTO oauth_clients(id,name,redirect_uris) VALUES(?,?,?)`, id, "Synthetic client", `["https://client.example/callback"]`); err != nil {
				t.Fatal(err)
			}
		}
		now := time.Now().UTC().Truncate(time.Second)
		store := NewStore(conn)
		store.Now = func() time.Time { return now }
		first, err := store.Create(ctx, "first", ReadWrite, revision, now.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		second, err := store.Create(ctx, "second", Read, revision, now.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SetScope(ctx, second, ReadWrite); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("escalation: %v", err)
		}
		for _, scope := range []string{Read, ReadWrite} {
			if err := store.SetScope(ctx, first, scope); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.SetScope(ctx, first, "mcp:read"); !errors.Is(err, ErrScope) {
			t.Fatalf("invalid scope: %v", err)
		}
		for range 2 {
			if err := store.Revoke(ctx, first); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.SetScope(ctx, first, Read); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("revived: %v", err)
		}
		list, err := NewStore(conn).List(ctx)
		if err != nil || len(list) != 2 {
			t.Fatalf("restart: %v %v", list, err)
		}
		for _, a := range list {
			if a.ID == first && a.RevokedAt == nil {
				t.Fatal("missing revocation")
			}
			if a.ID == second && a.RevokedAt != nil {
				t.Fatal("revocation leaked")
			}
		}
		store.Now = func() time.Time { return now.Add(time.Hour) }
		if err := store.SetScope(ctx, second, Read); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expiry: %v", err)
		}
		list, err = store.List(ctx)
		if err != nil || !list[0].Expired || !list[1].Expired {
			t.Fatalf("expired list: %v %v", list, err)
		}
		if err := owner.ChangePassword(ctx, "synthetic-password", "changed"); err != nil {
			t.Fatal(err)
		}
		list, err = store.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range list {
			if a.RevokedAt == nil {
				t.Fatal("password change did not revoke")
			}
		}
		var newRevision int
		if err := conn.QueryRow(`SELECT revision FROM auth_owner WHERE id=1`).Scan(&newRevision); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Create(ctx, "first", Read, revision, now.Add(3*time.Hour)); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("old revision: %v", err)
		}
		id, err := store.Create(ctx, "first", Read, newRevision, now.Add(3*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.ResetPassword(ctx, "reset"); err != nil {
			t.Fatal(err)
		}
		list, err = store.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range list {
			if a.ID == id && a.RevokedAt == nil {
				t.Fatal("reset did not revoke")
			}
		}
		if enabled, err := Enabled(ctx, conn); err != nil || !enabled {
			t.Fatalf("default: %v %v", enabled, err)
		}
		if err := SetEnabled(ctx, conn, false); err != nil {
			t.Fatal(err)
		}
		if enabled, err := Enabled(ctx, conn); err != nil || enabled {
			t.Fatalf("disabled: %v %v", enabled, err)
		}
	})
}
