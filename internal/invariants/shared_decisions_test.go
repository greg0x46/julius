package invariants_test

import (
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/ledger"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/projections"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/timeline"
	"github.com/greg0x46/julius/internal/transactions"
)

func TestBRLLinkEligibilityAcrossPlanningModules(t *testing.T) {
	brl := &money.EffectiveMoney{Value: mustDec(t, "-50.25"), CurrencyCode: "BRL"}
	zero := &money.EffectiveMoney{Value: mustDec(t, "0"), CurrencyCode: "BRL"}
	usd := &money.EffectiveMoney{Value: mustDec(t, "50.25"), CurrencyCode: "USD"}
	for _, direction := range []struct {
		name      string
		required  money.Classification
		payable   payables.Kind
		recurring recurrences.Kind
		wrong     string
	}{
		{"outflow", money.Outflow, payables.KindDebt, recurrences.KindExpense, "not_outflow"},
		{"inflow", money.Inflow, payables.KindReceivable, recurrences.KindIncome, "not_inflow"},
	} {
		t.Run(direction.name, func(t *testing.T) {
			for _, tc := range []struct {
				name           string
				classification money.Classification
				inclusion      money.InclusionState
				effective      *money.EffectiveMoney
				linked         bool
				want           string
			}{
				{"eligible", direction.required, money.Considered, brl, false, ""},
				{"zero still linkable", direction.required, money.Considered, zero, false, ""},
				{"ignored precedes all checks", money.Unclassified, money.Ignored, nil, true, "ignored"},
				{"direction precedes currency and conflict", money.Unclassified, money.Considered, usd, true, direction.wrong},
				{"foreign precedes conflict", direction.required, money.Considered, usd, true, "missing_brl_pair"},
				{"missing money", direction.required, money.Considered, nil, false, "missing_brl_pair"},
				{"existing link", direction.required, money.Considered, brl, true, "conflict"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					p := payables.EligibilityForLink(direction.payable, tc.classification, tc.inclusion, tc.effective, tc.linked)
					r := recurrences.EligibilityForReconciliation(direction.recurring, tc.classification, tc.inclusion, tc.effective, tc.linked)
					if p.Eligible != (tc.want == "") || r.Eligible != (tc.want == "") {
						t.Fatalf("payable=%+v recurrence=%+v, want reason %q", p, r, tc.want)
					}
					pReason, rReason := "", ""
					if p.Reason != nil {
						pReason = string(*p.Reason)
					}
					if r.Reason != nil {
						rReason = string(*r.Reason)
					}
					wantP, wantR := tc.want, tc.want
					if tc.want == "conflict" {
						wantP, wantR = "already_linked", "already_reconciled"
					}
					if pReason != wantP || rReason != wantR {
						t.Errorf("reasons = %q / %q, want %q / %q", pReason, rReason, wantP, wantR)
					}
					shared := money.BRLLinkIneligibility(tc.classification, direction.required, tc.inclusion, tc.effective)
					wantShared := tc.want
					if tc.want == "conflict" {
						wantShared = ""
					}
					if shared == nil {
						if wantShared != "" {
							t.Errorf("shared rule accepted, want %q", wantShared)
						}
					} else if string(*shared) != wantShared {
						t.Errorf("shared reason = %q, want %q", *shared, wantShared)
					}
				})
			}
		})
	}
}

func TestMonthBucketsAgreeWithRecurringProjections(t *testing.T) {
	for _, tc := range []struct {
		year  int
		month time.Month
		last  int
	}{
		{2026, time.February, 28}, {2028, time.February, 29},
		{2026, time.April, 30}, {2026, time.December, 31},
	} {
		from := time.Date(tc.year, tc.month, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(tc.year, tc.month, tc.last, 0, 0, 0, 0, time.UTC)
		t.Run(from.Format("2006-01"), func(t *testing.T) {
			f := newFixture(t)
			account := f.addAccount("1000", "BANK")
			service := ledger.Service{DB: f.conn}
			item, err := service.CreateManual(f.ctx, transactions.ManualInput{
				AccountID: account, Description: "Despesa", Amount: mustDec(t, "-25.50"), OccurredAt: to,
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			period, err := money.PeriodFor(item.OccurredAt, money.GroupMonth, "UTC")
			if err != nil {
				t.Fatal(err)
			}
			if period.StartDate.String() != from.Format("2006-01-02") || period.EndDate.String() != to.Format("2006-01-02") {
				t.Fatalf("period = %+v, want %s through %s", period, from, to)
			}
			commitment, err := recurrences.Create(f.ctx, f.conn, recurrences.Write{
				Name: "Despesa mensal", Kind: recurrences.KindExpense, Amount: mustDec(t, "100"),
				CategoryID: categorySupermercado, Cadence: recurrences.CadenceMonthly, DayOfMonth: 31,
				StartDate: from, IsActive: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			events, err := projections.List(f.ctx, f.conn, projections.ProjectionQuery{
				From: from, To: to, Selection: projections.SelectionExplicit, IDs: []string{commitment.ID},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 1 || !events[0].Date.Equal(to) {
				t.Fatalf("events = %+v, want one on %s", events, to)
			}
			assertDec(t, "projected expense", events[0].Amount, "-100")
			series, err := timeline.BuildSeries(f.ctx, f.conn, timeline.BuildParams{
				From: from, To: to, ReferenceDate: from, Timezone: "UTC", ScenarioIDs: []string{commitment.ID},
			})
			if err != nil {
				t.Fatal(err)
			}
			inflow, outflow := f.totals()
			assertDec(t, "ledger inflow", inflow, "0")
			assertDec(t, "ledger outflow", outflow, "25.5")
			totals := timeline.TotalsForPeriod(series)
			assertDec(t, "timeline income", totals.Income, "0")
			assertDec(t, "timeline expense", totals.Expense, "125.5")
			assertDec(t, "ending balance", series.Points[len(series.Points)-1].Balance, "874.5")
		})
	}
}
