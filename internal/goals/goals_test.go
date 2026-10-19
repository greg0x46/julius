package goals_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/goals"
	"github.com/greg0x46/julius/internal/investments"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// invest opens a manual holding worth value, the only investment the fixture needs.
func invest(t *testing.T, conn *sql.DB, name, value string) {
	t.Helper()
	ctx := context.Background()
	account, err := investments.CreateAccount(ctx, conn, investments.AccountInput{Name: "Custódia " + name})
	if err != nil {
		t.Fatal(err)
	}
	v := dec(value)
	if _, err := investments.CreatePosition(ctx, conn, investments.PositionInput{
		AccountID: account.ID, Name: name, AssetType: "Renda fixa", InitialValue: &v,
		OccurredOn: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
}

func input(target string) goals.Input {
	return goals.Input{Name: "Primeiro milhão", Metric: goals.MetricInvestedValue, TargetAmount: dec(target)}
}

func TestProgressFollowsInvestedValueRecomputedOnRead(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	ctx := context.Background()
	day := time.Date(2027, 12, 31, 0, 0, 0, 0, time.UTC)
	notes := "  reserva de longo prazo  "
	in := input("1000")
	in.TargetDate, in.Notes = &day, &notes

	created, err := goals.Create(ctx, conn, in)
	if err != nil {
		t.Fatal(err)
	}
	if created.CurrentValue.String() != "0" || created.Progress.String() != "0" {
		t.Fatalf("empty goal = %+v", created)
	}
	if created.TargetDate == nil || created.TargetDate.Format("2006-01-02") != "2027-12-31" ||
		created.Notes == nil || *created.Notes != "reserva de longo prazo" {
		t.Fatalf("stored fields = %+v", created)
	}

	invest(t, conn, "CDB", "250")
	got, err := goals.Get(ctx, conn, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentValue.String() != "250" || got.Progress.String() != "0.25" {
		t.Fatalf("progress = %+v", got)
	}

	// Progress is not capped: a reached goal reports more than 100%.
	invest(t, conn, "Tesouro", "1750")
	got, err = goals.Get(ctx, conn, created.ID)
	if err != nil || got.CurrentValue.String() != "2000" || got.Progress.String() != "2" {
		t.Fatalf("beyond target = %+v, %v", got, err)
	}

	// Nothing derived is stored.
	var stored string
	if err := conn.QueryRow(`SELECT target_amount FROM goals WHERE id = ?`, created.ID).Scan(&stored); err != nil || stored != "1000" {
		t.Fatalf("stored target = %q, %v", stored, err)
	}
}

func TestListUpdateDelete(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	ctx := context.Background()
	if list, err := goals.List(ctx, conn); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("empty list = %#v, %v", list, err)
	}
	invest(t, conn, "CDB", "500")
	first, err := goals.Create(ctx, conn, input("1000"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := goals.Create(ctx, conn, goals.Input{Name: "Viagem", Metric: goals.MetricInvestedValue, TargetAmount: dec("4000")})
	if err != nil {
		t.Fatal(err)
	}
	list, err := goals.List(ctx, conn)
	if err != nil || len(list) != 2 || list[0].ID != first.ID || list[1].ID != second.ID {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if list[0].Progress.String() != "0.5" || list[1].Progress.String() != "0.125" {
		t.Fatalf("progress = %v / %v", list[0].Progress, list[1].Progress)
	}

	day := time.Date(2028, 1, 15, 0, 0, 0, 0, time.UTC)
	updated, err := goals.Update(ctx, conn, first.ID, goals.Input{
		Name: "  Reserva  ", Metric: goals.MetricInvestedValue, TargetAmount: dec("2000"), TargetDate: &day,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Reserva" || updated.TargetAmount.String() != "2000" || updated.Progress.String() != "0.25" ||
		updated.TargetDate == nil || updated.TargetDate.Format("2006-01-02") != "2028-01-15" || updated.Notes != nil {
		t.Fatalf("updated = %+v", updated)
	}

	if err := goals.Delete(ctx, conn, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := goals.Get(ctx, conn, first.ID); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("get after delete = %v", err)
	}
	if err := goals.Delete(ctx, conn, first.ID); !errors.Is(err, goals.ErrNotFound) {
		t.Fatalf("second delete = %v", err)
	}
	// Deleting a goal never touches the investments it measured.
	if positions, err := investments.ListPositions(ctx, conn, investments.PositionFilter{}); err != nil || len(positions) != 1 {
		t.Fatalf("positions after delete = %+v, %v", positions, err)
	}
}

func TestRejectsInvalidInput(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	ctx := context.Background()
	existing, err := goals.Create(ctx, conn, input("1000"))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]goals.Input{
		"empty name":      {Name: "  ", Metric: goals.MetricInvestedValue, TargetAmount: dec("10")},
		"zero target":     {Name: "x", Metric: goals.MetricInvestedValue, TargetAmount: decimal.Zero},
		"negative target": {Name: "x", Metric: goals.MetricInvestedValue, TargetAmount: dec("-1")},
		"no metric":       {Name: "x", TargetAmount: dec("10")},
		"net_worth":       {Name: "x", Metric: "net_worth", TargetAmount: dec("10")},
		"cash_balance":    {Name: "x", Metric: "cash_balance", TargetAmount: dec("10")},
		"unknown metric":  {Name: "x", Metric: "bogus", TargetAmount: dec("10")},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := goals.Create(ctx, conn, in); !errors.Is(err, goals.ErrInvalidInput) {
				t.Errorf("Create = %v, want ErrInvalidInput", err)
			}
			if _, err := goals.Update(ctx, conn, existing.ID, in); !errors.Is(err, goals.ErrInvalidInput) {
				t.Errorf("Update = %v, want ErrInvalidInput", err)
			}
		})
	}
	if list, err := goals.List(ctx, conn); err != nil || len(list) != 1 {
		t.Fatalf("rejected input persisted: %+v, %v", list, err)
	}
}

func TestMissingGoal(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	ctx := context.Background()
	const id = "11111111-1111-4111-8111-111111111111"
	if _, err := goals.Get(ctx, conn, id); !errors.Is(err, goals.ErrNotFound) {
		t.Errorf("Get = %v", err)
	}
	if _, err := goals.Update(ctx, conn, id, input("10")); !errors.Is(err, goals.ErrNotFound) {
		t.Errorf("Update = %v", err)
	}
	if err := goals.Delete(ctx, conn, id); !errors.Is(err, goals.ErrNotFound) {
		t.Errorf("Delete = %v", err)
	}
}

func TestWritesRollBackWhenProgressCannotBeValued(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		existing, err := goals.Create(ctx, conn, input("1000"))
		if err != nil {
			t.Fatal(err)
		}
		investmentID := insertForeignInvestment(t, conn)

		// Make FX reads fail without contacting the external rate provider.
		if _, err := conn.Exec(`ALTER TABLE exchange_rates RENAME TO unavailable_exchange_rates`); err != nil {
			t.Fatal(err)
		}
		if _, err := goals.Create(ctx, conn, input("2000")); err == nil {
			t.Fatal("Create succeeded despite failed valuation query")
		}
		changed := input("3000")
		changed.Name = "Changed name"
		if _, err := goals.Update(ctx, conn, existing.ID, changed); err == nil {
			t.Fatal("Update succeeded despite failed valuation query")
		}

		// Restore valuation reads so the normal API verifies persisted state.
		if _, err := conn.Exec(`ALTER TABLE unavailable_exchange_rates RENAME TO exchange_rates`); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(`DELETE FROM financial_investments WHERE id = ?`, investmentID); err != nil {
			t.Fatal(err)
		}
		list, err := goals.List(ctx, conn)
		if err != nil || len(list) != 1 {
			t.Fatalf("goals after failed writes = %+v, %v", list, err)
		}
		got := list[0]
		if got.ID != existing.ID || got.Name != existing.Name || !got.TargetAmount.Equal(existing.TargetAmount) ||
			!got.UpdatedAt.Equal(existing.UpdatedAt) {
			t.Fatalf("failed update changed goal: got %+v, want %+v", got, existing)
		}
	})
}

// insertForeignInvestment stores a synced USD holding, whose valuation needs
// an exchange rate.
func insertForeignInvestment(t *testing.T, conn *sql.DB) string {
	t.Helper()
	now := db.FormatTime(time.Now())
	source, run, raw, investment := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO data_sources(id,provider,external_item_id,created_at,updated_at) VALUES(?, 'pluggy', ?, ?, ?)`, []any{source, source, now, now}},
		{`INSERT INTO sync_runs(id,source_id,status,started_at,finished_at) VALUES(?, ?, 'completed', ?, ?)`, []any{run, source, now, now}},
		{`INSERT INTO raw_imports(id,sync_run_id,source_id,scope,page_sequence,request_attempt,request_method,request_path,http_status,response_headers,payload,payload_sha256,received_at) VALUES(?, ?, ?, 'investments', 1, 1, 'GET', '/x', 200, '{}', ?, 'sha', ?)`, []any{raw, run, source, []byte{}, now}},
		{`INSERT INTO financial_investments(id,source_id,external_id,name,investment_type,balance,currency_code,current_raw_import_id,normalized_hash,created_at,updated_at) VALUES(?, ?, ?, 'Synthetic USD fund', 'FIXED_INCOME', '100', 'USD', ?, 'hash', ?, ?)`, []any{investment, source, investment, raw, now, now}},
	} {
		if _, err := conn.Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	return investment
}
