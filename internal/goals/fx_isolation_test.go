package goals_test

import (
	"context"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/goals"
)

// A goal measures investments only: foreign cash without a rate must not make
// the goal unreadable.
func TestGoalsIgnoreUnrelatedForeignCashWithoutRate(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	ctx := context.Background()
	now := db.FormatTime(time.Now())
	if _, err := conn.Exec(`INSERT INTO financial_accounts
		(id, origin, name, account_type, currency_code, opening_balance, created_at, updated_at)
		VALUES ('052e2a8b-6b4c-5c1d-90c3-2f8654bcd27c', 'manual', 'Synthetic USD', 'BANK', 'USD', '100', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	invest(t, conn, "CDB", "250")
	// Make any FX read fail without contacting the external rate provider.
	if _, err := conn.Exec(`ALTER TABLE exchange_rates RENAME TO unavailable_exchange_rates`); err != nil {
		t.Fatal(err)
	}

	created, err := goals.Create(ctx, conn, input("1000"), time.UTC)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.CurrentValue.String() != "250" {
		t.Fatalf("current value = %s, want 250", created.CurrentValue)
	}
	if _, err := goals.Update(ctx, conn, created.ID, input("2000"), time.UTC); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if list, err := goals.List(ctx, conn, time.UTC); err != nil || len(list) != 1 {
		t.Fatalf("List = %+v, %v", list, err)
	}
}
