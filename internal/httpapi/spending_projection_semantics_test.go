package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/scenarios"
	"github.com/greg0x46/julius/internal/transactions"
)

func TestSpendingProjectionUsesCanonicalDirectionAndRealization(t *testing.T) {
	for _, tc := range []struct {
		name       string
		kind       scenarios.Kind
		amount     string
		realized   bool
		wantTotal  string
		wantCount  int
		wantSource string
	}{
		{"debt expense", scenarios.KindDebtPlan, "10", false, "10", 1, "projetado"},
		{"receivable income", scenarios.KindReceivablePlan, "10", false, "0", 0, ""},
		{"standalone expense", scenarios.KindStandalone, "-10", false, "10", 1, "projetado"},
		{"standalone income", scenarios.KindStandalone, "10", false, "0", 0, ""},
		{"realized debt", scenarios.KindDebtPlan, "10", true, "10", 1, "real"},
		{"realized standalone", scenarios.KindStandalone, "-10", true, "10", 1, "real"},
		{"recurring expense", scenarios.KindRecurring, "-10", false, "10", 1, "projetado"},
		{"realized recurring expense", scenarios.KindRecurring, "-10", true, "10", 1, "real"},
		{"recurring income", scenarios.KindRecurring, "10", false, "0", 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
				ctx := context.Background()
				now := time.Now().UTC()
				day := time.Date(now.Year(), now.Month(), 15, 12, 0, 0, 0, time.UTC)
				amount := decimal.RequireFromString(tc.amount)
				category := "Synthetic expense"
				var scenarioID, installmentID string
				var payable *payables.Payable
				if tc.kind == scenarios.KindRecurring {
					kind := recurrences.KindIncome
					if amount.IsNegative() {
						kind = recurrences.KindExpense
					}
					commitment, err := recurrences.Create(ctx, conn, recurrences.Write{
						Name: "Synthetic recurrence", Kind: kind, Amount: amount.Abs(),
						CategoryID: "000433b6-3094-5a9c-87df-465b70574a4b", Cadence: recurrences.CadenceMonthly,
						DayOfMonth: 15, StartDate: day, IsActive: true,
					})
					if err != nil {
						t.Fatal(err)
					}
					scenarioID = commitment.ID
				} else {
					var payableID *string
					if tc.kind == scenarios.KindDebtPlan || tc.kind == scenarios.KindReceivablePlan {
						kind := payables.KindDebt
						if tc.kind == scenarios.KindReceivablePlan {
							kind = payables.KindReceivable
						}
						p, err := payables.Create(ctx, conn, kind, "Synthetic payable", amount, amount)
						if err != nil {
							t.Fatal(err)
						}
						payable, payableID = &p, &p.ID
					}
					scenario, err := scenarios.CreateScenario(ctx, conn, tc.kind, "Synthetic scenario", payableID)
					if err != nil {
						t.Fatal(err)
					}
					scenarioID = scenario.ID
					installment, err := scenarios.CreateScenarioTransaction(ctx, conn, scenarioID, "Synthetic installment", amount, day, &category)
					if err != nil {
						t.Fatal(err)
					}
					installmentID = installment.ID
				}
				if tc.realized {
					accountID := uuid.NewString()
					stamp := db.FormatTime(now)
					if _, err := conn.Exec(`INSERT INTO financial_accounts (id, origin, name, account_type, currency_code, opening_balance, created_at, updated_at)
						VALUES (?, 'manual', 'Synthetic account', 'BANK', 'BRL', '0', ?, ?)`, accountID, stamp, stamp); err != nil {
						t.Fatal(err)
					}
					transactionID, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{
						AccountID: accountID, Description: "Actual expense", Amount: decimal.NewFromInt(-10), OccurredAt: day,
					})
					if err != nil {
						t.Fatal(err)
					}
					if payable != nil {
						if _, err := payables.CreateLink(ctx, conn, payables.KindDebt, payable.ID, transactionID); err != nil {
							t.Fatal(err)
						}
					}
					eventKey := "scenario:" + scenarioID + ":transaction:" + installmentID
					if tc.kind == scenarios.KindRecurring {
						eventKey = "scenario:" + scenarioID + ":occurrence:" + day.Format("2006-01-02")
					}
					if _, err := scenarios.RealizeEvent(ctx, conn, scenarioID, eventKey, scenarios.RealizationWrite{
						State: scenarios.RealizationStateLinked, TransactionID: &transactionID,
					}); err != nil {
						t.Fatal(err)
					}
				}
				r := httptest.NewRequest("GET", "/api/transactions/spending-by-category?timezone=UTC&scenario_id="+scenarioID, nil)
				w := httptest.NewRecorder()
				handleSpendingByCategory(conn)(w, r)
				if w.Code != 200 {
					t.Fatalf("spending: %d %s", w.Code, w.Body.String())
				}
				var got spendingByCategoryDTO
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if !decimal.RequireFromString(got.Total).Equal(decimal.RequireFromString(tc.wantTotal)) || got.Reporting.Eligible != tc.wantCount || got.Reporting.Converted != tc.wantCount || len(got.Items) != tc.wantCount {
					t.Fatalf("spending = %+v, want total=%s count=%d", got, tc.wantTotal, tc.wantCount)
				}
				if tc.wantCount > 0 && got.Items[0].Source != tc.wantSource {
					t.Fatalf("source = %s, want %s", got.Items[0].Source, tc.wantSource)
				}
				if tc.kind != scenarios.KindRecurring && tc.wantSource == "projetado" && got.Items[0].CategoryName != category {
					t.Fatalf("projected category = %s, want %s", got.Items[0].CategoryName, category)
				}
			})
		})
	}
}

func TestSpendingProjectionKeepsCalendarBoundsAndExactCategoryTotals(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := t.Context()
		scenario, err := scenarios.CreateScenario(ctx, conn, scenarios.KindStandalone, "Synthetic scenario", nil)
		if err != nil {
			t.Fatal(err)
		}
		empty := ""
		category := "Synthetic category"
		for _, entry := range []struct {
			day, amount string
			category    *string
		}{
			{"2026-08-31", "-100", nil},
			{"2026-09-01", "-0.00000000000000001", nil},
			{"2026-09-15", "-2.00000000000000001", &empty},
			{"2026-09-30", "-3", &category},
			{"2026-09-15", "4", &category},
			{"2026-09-15", "0", &category},
			{"2026-10-01", "-100", nil},
		} {
			day, err := time.Parse("2006-01-02", entry.day)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := scenarios.CreateScenarioTransaction(ctx, conn, scenario.ID, "Synthetic installment", decimal.RequireFromString(entry.amount), day, entry.category); err != nil {
				t.Fatal(err)
			}
		}
		items, count, err := projectedSpendingByCategory(ctx, conn, scenario.ID,
			money.Date{Year: 2026, Month: time.September, Day: 1},
			money.Date{Year: 2026, Month: time.September, Day: 30})
		if err != nil {
			t.Fatal(err)
		}
		if count != 3 || len(items) != 2 {
			t.Fatalf("projected spending = %+v, count=%d; want 3 outflows in 2 categories", items, count)
		}
		amounts := make(map[string]string, len(items))
		for _, item := range items {
			amounts[item.CategoryName] = item.Amount
			if item.Source != "projetado" || item.CategoryID != nil {
				t.Errorf("projected category metadata = %+v", item)
			}
		}
		if amounts["Sem categoria"] != "2.00000000000000002" || amounts[category] != "3" {
			t.Fatalf("category amounts = %v", amounts)
		}
	})
}
