package db_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/greg0x46/julius/internal/db/dbtest"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
)

// Existing clients must survive the migration dated at migration time, so the
// stale-client prune cannot remove one that was registered moments before the
// upgrade; the column rolls back without touching the rows.
func TestOAuthClientCreatedAtMigrationDatesExistingClients(t *testing.T) {
	for _, backend := range dbtest.Backends() {
		t.Run(backend.Name, func(t *testing.T) {
			conn := backend.Open(t)
			dialect := goose.DialectSQLite3
			if backend.Name == "postgres" {
				dialect = goose.DialectPostgres
			}
			provider, err := goose.NewProvider(dialect, conn, os.DirFS("migrations/"+backend.Name))
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if _, err := provider.DownTo(ctx, 8); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"existing-1", "existing-2"} {
				if _, err := conn.Exec(`INSERT INTO oauth_clients(id,name,redirect_uris) VALUES(?,?,?)`, id, "Synthetic", `["https://client.example/callback"]`); err != nil {
					t.Fatal(err)
				}
			}
			before := time.Now().UTC().Add(-time.Minute)
			if _, err := provider.UpTo(ctx, 9); err != nil {
				t.Fatal(err)
			}
			after := time.Now().UTC().Add(time.Minute)
			rows, err := conn.Query(`SELECT created_at FROM oauth_clients ORDER BY id`)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			n := 0
			for rows.Next() {
				var created time.Time
				if err := rows.Scan(dbtypes.Time(&created)); err != nil {
					t.Fatal(err)
				}
				if created.Before(before) || created.After(after) {
					t.Fatalf("existing client dated %v, want the migration time", created)
				}
				n++
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if n != 2 {
				t.Fatalf("%d clients after the migration, want 2", n)
			}
			// An insert that omits the column still works (tests, tooling).
			if _, err := conn.Exec(`INSERT INTO oauth_clients(id,name,redirect_uris) VALUES('later','Synthetic','[]')`); err != nil {
				t.Fatalf("insert without created_at: %v", err)
			}
			if _, err := provider.DownTo(ctx, 8); err != nil {
				t.Fatal(err)
			}
			var clients int
			if err := conn.QueryRow(`SELECT COUNT(*) FROM oauth_clients`).Scan(&clients); err != nil || clients != 3 {
				t.Fatalf("rollback kept %d clients (%v), want 3", clients, err)
			}
			if _, err := conn.Exec(`SELECT created_at FROM oauth_clients`); err == nil {
				t.Fatal("created_at survived the rollback")
			}
		})
	}
}
