package investments_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/investments"
)

func instant(value string) *time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return &t
}

func strRef(v string) *string { return &v }

func calendarDay(value string) time.Time {
	t, err := time.Parse(investments.DateLayout, value)
	if err != nil {
		panic(err)
	}
	return t
}

// addHolding stores h as a financial_investments row, the way sync would.
func (f *ledgerFixture) addHolding(h investments.SyncedHolding) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	decimalArg := func(v interface{ String() string }, ok bool) any {
		if !ok {
			return nil
		}
		return v.String()
	}
	f.exec(`INSERT INTO financial_investments (
			id, source_id, external_id, name, code, investment_type, subtype, currency_code, issuer_cnpj,
			rate, rate_type, issue_date, purchase_date, due_date, quantity, amount, amount_original, as_of_date,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 'BRL', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'hash', ?, ?)`,
		id, f.sourceID, h.ExternalID, h.Name, h.Code, h.InvestmentType, h.Subtype, h.IssuerCNPJ,
		decimalArg(h.Rate, h.Rate != nil), h.RateType,
		investments.FormatProviderDatePtr(h.IssueDate), investments.FormatProviderDatePtr(h.PurchaseDate), investments.FormatProviderDatePtr(h.DueDate),
		decimalArg(h.Quantity, h.Quantity != nil), decimalArg(h.Amount, h.Amount != nil),
		decimalArg(h.AmountOriginal, h.AmountOriginal != nil), db.FormatTimePtr(h.AsOfDate),
		f.rawImportID, now, now)
	return id
}

func (f *ledgerFixture) recordQuote(h investments.SyncedHolding, asOf, amount string) {
	f.t.Helper()
	h.AsOfDate, h.Amount = instant(asOf), decRef(amount)
	if err := investments.RecordSyncedQuotes(f.ctx, f.conn, h, nil); err != nil {
		f.t.Fatalf("RecordSyncedQuotes: %v", err)
	}
}

func (f *ledgerFixture) addMovement(investmentID, direction, amount, quantity string, on time.Time) {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	var quantityArg any
	if quantity != "" {
		quantityArg = quantity
	}
	f.exec(`INSERT INTO financial_investment_transactions (
			id, source_id, investment_id, external_id, movement_type, direction, quantity, amount, occurred_at,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'X', ?, ?, ?, ?, ?, 'hash', ?, ?)`,
		id, f.sourceID, investmentID, id, direction, quantityArg, amount, db.FormatTime(on), f.rawImportID, now, now)
}

func (f *ledgerFixture) yield(positionID string, from *time.Time, to string) investments.YieldResult {
	f.t.Helper()
	result, err := investments.PositionYield(f.ctx, f.conn, positionID, from, calendarDay(to))
	if err != nil {
		f.t.Fatalf("PositionYield: %v", err)
	}
	return result
}

func (f *ledgerFixture) yieldReason(positionID string) string {
	f.t.Helper()
	_, err := investments.PositionYield(f.ctx, f.conn, positionID, nil, calendarDay("2026-09-28"))
	var reason *investments.YieldUnavailableError
	if !errors.As(err, &reason) {
		f.t.Fatalf("PositionYield error = %v, want YieldUnavailableError", err)
	}
	return reason.Reason
}

// lca mirrors the Nubank LCA that motivated the price series: bought in 2024,
// before the connection existed, so no movement was ever synced for it.
func lca() investments.SyncedHolding {
	return investments.SyncedHolding{
		ExternalID: "lca-1", Name: strRef("LCA - BANCO VOTORANTIM S.A."), Code: strRef("24I01250142"),
		InvestmentType: strRef("FIXED_INCOME"), Subtype: strRef("LCA"),
		IssueDate: instant("2024-09-06T03:00:00Z"), PurchaseDate: instant("2024-09-06T03:00:00Z"),
		Quantity: decRef("200"), Amount: decRef("258.6"), AmountOriginal: decRef("200"),
		AsOfDate: instant("2026-09-28T00:03:22Z"),
	}
}

func TestPositionYieldOfFixedIncomeWithoutMovementsStartsFromItsPurchasePrice(t *testing.T) {
	f := newLedgerFixture(t)
	holding := lca()
	id := f.addHolding(holding)
	f.recordQuote(holding, "2026-08-31T06:25:30Z", "256.29")
	f.recordQuote(holding, "2026-09-28T00:03:22Z", "258.6")

	sinceInception := f.yield(id, nil, "2026-09-28")
	if sinceInception.Value.String() != "58.6" {
		t.Fatalf("since inception = %s, want 58.6", sinceInception.Value)
	}
	if !sinceInception.From.Equal(calendarDay("2024-09-05")) || sinceInception.NetFlows.String() != "200" {
		t.Fatalf("from = %s, flows = %s; want the day before the purchase and the 200 applied",
			sinceInception.From.Format(investments.DateLayout), sinceInception.NetFlows)
	}

	from := calendarDay("2026-09-01")
	period := f.yield(id, &from, "2026-09-28")
	if period.Value.String() != "2.31" || period.Partial {
		t.Fatalf("September = %s (partial %v), want 2.31 from 256.29 to 258.60", period.Value, period.Partial)
	}
}

// A purchase PU from 2024 must not price January 2026: that would report two
// years of accrual as the period's rendimento. The period starts on the first
// day a recent quote covers instead, and says so.
func TestPositionYieldStartsAtTheFirstPricedDayWhenTheStartHasNoRecentQuote(t *testing.T) {
	f := newLedgerFixture(t)
	holding := lca()
	id := f.addHolding(holding)
	f.recordQuote(holding, "2026-08-31T06:25:30Z", "256.29")
	f.recordQuote(holding, "2026-09-28T00:03:22Z", "258.6")

	from := calendarDay("2026-01-01")
	period := f.yield(id, &from, "2026-09-28")
	if !period.Partial || !period.From.Equal(calendarDay("2026-08-31")) || period.Value.String() != "2.31" {
		t.Fatalf("got %s from %s (partial %v), want 2.31 from 2026-08-31 (the quote's day), partial",
			period.Value, period.From.Format(investments.DateLayout), period.Partial)
	}
}

func TestDailyYieldAddsUpToThePeriod(t *testing.T) {
	f := newLedgerFixture(t)
	holding := lca()
	id := f.addHolding(holding)
	f.recordQuote(holding, "2026-09-25T06:31:45Z", "258.47")
	f.recordQuote(holding, "2026-09-27T12:00:00Z", "258.6")

	days, err := investments.DailyYield(f.ctx, f.conn, id, calendarDay("2026-09-25"), calendarDay("2026-09-28"))
	if err != nil {
		t.Fatalf("DailyYield: %v", err)
	}
	got := map[string]string{}
	for _, d := range days {
		got[d.Day.Format(investments.DateLayout)] = d.Value.String()
	}
	want := map[string]string{"2026-09-26": "0", "2026-09-27": "0.13", "2026-09-28": "0"}
	if len(got) != len(want) {
		t.Fatalf("days = %v, want %v", got, want)
	}
	for day, value := range want {
		if got[day] != value {
			t.Fatalf("days = %v, want %v", got, want)
		}
	}
}

func cdb(issued string) investments.SyncedHolding {
	return investments.SyncedHolding{
		ExternalID: uuid.NewString(), Name: strRef("CDB - NU FINANCEIRA S.A."),
		InvestmentType: strRef("FIXED_INCOME"), Subtype: strRef("CDB"), IssuerCNPJ: strRef("30.680.829/0001-43"),
		Rate: decRef("120"), RateType: strRef("CDI"),
		IssueDate: instant(issued), PurchaseDate: instant(issued), DueDate: instant("2028-09-12T03:00:00Z"),
		Quantity: decRef("75000"), Amount: decRef("754.61"), AmountOriginal: decRef("750"),
		AsOfDate: instant("2026-09-28T00:00:00Z"),
	}
}

// Titles with the same terms accrue the same PU, so they share an asset and
// its price series; any difference in terms makes a different title.
func TestFixedIncomeTitlesShareAnAssetOnlyWithTheSameTerms(t *testing.T) {
	f := newLedgerFixture(t)
	first, twin, later := cdb("2026-09-13T03:00:00Z"), cdb("2026-09-13T03:00:00Z"), cdb("2026-09-14T03:00:00Z")
	if code := first.Ticker(); code == nil || *code != "CDB-30680829-120CDI-20260913-20280912" {
		t.Fatalf("code = %v, want CDB-30680829-120CDI-20260913-20280912", code)
	}
	resolve := func(h investments.SyncedHolding) string {
		asset, err := investments.ResolveSyncedAsset(f.ctx, f.conn, h)
		if err != nil {
			t.Fatalf("ResolveSyncedAsset: %v", err)
		}
		return asset.ID
	}
	if resolve(first) != resolve(twin) {
		t.Fatal("identical terms resolved to different assets")
	}
	if resolve(first) == resolve(later) {
		t.Fatal("a different issue date resolved to the same asset")
	}
	withoutCNPJ := first
	withoutCNPJ.IssuerCNPJ = nil
	if withoutCNPJ.Ticker() != nil {
		t.Fatal("a code was built from partial terms")
	}
}

// qty × PU is the gross value: the IR/IOF the institution provisions stays
// out of it, for the caller to deduct once.
func TestPositionYieldOfFixedIncomeWithItsPurchaseSyncedIsGross(t *testing.T) {
	f := newLedgerFixture(t)
	holding := cdb("2026-09-13T03:00:00Z")
	id := f.addHolding(holding)
	f.addMovement(id, "inflow", "750", "75000", time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC))
	f.recordQuote(holding, "2026-09-28T00:00:00Z", "754.61")

	if got := f.yield(id, nil, "2026-09-28"); got.Value.String() != "4.61" {
		t.Fatalf("since inception = %s, want 4.61", got.Value)
	}
}

func equity(quantity string) investments.SyncedHolding {
	return investments.SyncedHolding{
		ExternalID: uuid.NewString(), Name: strRef("HGLG11"), Code: strRef("HGLG11"),
		InvestmentType: strRef("EQUITY"), Quantity: decRef(quantity),
	}
}

// A closed holding needs no price: what came out minus what went in is the
// realized rendimento, dividends included.
func TestPositionYieldOfAClosedEquityIsRealized(t *testing.T) {
	f := newLedgerFixture(t)
	id := f.addHolding(equity("0"))
	f.addMovement(id, "inflow", "800", "10", day(1))
	f.addMovement(id, "outflow", "790", "10", day(10))
	f.addMovement(id, "outflow", "20", "", day(15))

	if got := f.yield(id, nil, "2026-09-28"); got.Value.String() != "10" {
		t.Fatalf("since inception = %s, want 10", got.Value)
	}
}

func TestPositionYieldRefusesWhatTheHistoryCannotExplain(t *testing.T) {
	f := newLedgerFixture(t)

	oversold := f.addHolding(equity("0"))
	f.addMovement(oversold, "inflow", "1600", "80", day(1))
	f.addMovement(oversold, "outflow", "2900", "120", day(2))
	if reason := f.yieldReason(oversold); reason != investments.YieldReasonIncompleteHistory {
		t.Fatalf("oversold equity reason = %s, want %s", reason, investments.YieldReasonIncompleteHistory)
	}

	// A CDB redeemed whole with its purchase from before the window: far past
	// the accrual a healthy redemption shows.
	redeemed := cdb("2025-06-01T03:00:00Z")
	redeemed.Quantity, redeemed.AmountOriginal = decRef("0"), decRef("0")
	redeemedID := f.addHolding(redeemed)
	f.addMovement(redeemedID, "inflow", "500", "50000", day(1))
	f.addMovement(redeemedID, "outflow", "1568.02", "144276.22", day(2))
	if reason := f.yieldReason(redeemedID); reason != investments.YieldReasonIncompleteHistory {
		t.Fatalf("redeemed CDB reason = %s, want %s", reason, investments.YieldReasonIncompleteHistory)
	}

	untouched := f.addHolding(equity("10"))
	if reason := f.yieldReason(untouched); reason != investments.YieldReasonNoHistory {
		t.Fatalf("no movements reason = %s, want %s", reason, investments.YieldReasonNoHistory)
	}

	unpriced := f.addHolding(equity("10"))
	f.addMovement(unpriced, "inflow", "1000", "10", day(1))
	if reason := f.yieldReason(unpriced); reason != investments.YieldReasonNoPrice {
		t.Fatalf("unpriced reason = %s, want %s", reason, investments.YieldReasonNoPrice)
	}
}

func TestPositionYieldOfAManualHoldingFollowsItsValuations(t *testing.T) {
	f := newLedgerFixture(t)
	positionID := f.addPosition(f.custodyID, "Ações")
	f.deposit("1000", day(1))
	f.create(investments.OperationInput{
		AccountID: f.custodyID, PositionID: &positionID, Kind: investments.OperationBuy,
		OccurredOn: day(2), Amount: dec("600"), Quantity: decRef("6"), UnitPrice: decRef("100"),
	})
	f.create(investments.OperationInput{
		AccountID: f.custodyID, PositionID: &positionID, Kind: investments.OperationValuation,
		OccurredOn: day(10), Amount: dec("660"),
	})

	if got := f.yield(positionID, nil, "2026-09-20"); got.Value.String() != "60" {
		t.Fatalf("since inception = %s, want 60", got.Value)
	}
	// Between the purchase and the first valuation the holding is carried at
	// cost, which says nothing about its value: the period starts on the
	// valuation instead.
	from := day(5)
	period := f.yield(positionID, &from, "2026-09-20")
	if !period.Partial || !period.From.Equal(day(10)) || !period.Value.IsZero() {
		t.Fatalf("got %s from %s (partial %v), want 0 from the valuation day, partial",
			period.Value, period.From.Format(investments.DateLayout), period.Partial)
	}
}
