package networth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/transactions"
	"github.com/greg0x46/julius/internal/valuation"
)

// Querier is satisfied by both *sql.DB and *sql.Tx — identical in shape to
// payables.Querier, reused as an alias so this package's functions can pass
// q straight through to the payables package without a type assertion.
type Querier = payables.Querier

// dateLayout is the on-disk format for captured_at, a date-only column —
// mirrors internal/recurrences' dateLayout.
const dateLayout = "2006-01-02"

func formatDate(t time.Time) string { return t.UTC().Format(dateLayout) }

// newValuation values at the given day with the application's exchange service.
func newValuation(q Querier, at time.Time) *valuation.Valuation {
	return valuation.New(exchange.New(q, nil), at)
}

// investmentBalance sums every synced investment's current balance, each valued
// in BRL, plus the manual investments' contribution. Balances are stored as
// TEXT to stay exact, so the summing happens here in Go with decimal.Decimal
// rather than through a float-lossy SQL SUM.
func investmentBalance(ctx context.Context, q Querier, valued *valuation.Valuation) (decimal.Decimal, error) {
	manual, err := investments.ManualNetWorthValued(ctx, q, valued)
	if err != nil {
		return decimal.Zero, err
	}
	rows, err := q.QueryContext(ctx, `SELECT balance, currency_code FROM financial_investments WHERE balance IS NOT NULL`)
	if err != nil {
		return decimal.Decimal{}, err
	}
	defer rows.Close()
	type holding struct {
		amount   decimal.Decimal
		currency string
	}
	var holdings []holding
	for rows.Next() {
		var raw string
		var currency sql.NullString
		if err := rows.Scan(dbtypes.Decimal(&raw), &currency); err != nil {
			return decimal.Decimal{}, err
		}
		amount, err := decimal.NewFromString(raw)
		if err != nil {
			return decimal.Decimal{}, fmt.Errorf("parse investment balance %q: %w", raw, err)
		}
		code := valuation.Currency(nil)
		if currency.Valid {
			code = valuation.Currency(&currency.String)
		}
		holdings = append(holdings, holding{amount, code})
	}
	if err := rows.Err(); err != nil {
		return decimal.Zero, err
	}
	rows.Close()
	total := manual
	for _, h := range holdings {
		value, err := valued.Convert(ctx, h.amount, h.currency)
		if err != nil {
			return decimal.Zero, err
		}
		total = total.Add(value)
	}
	return total, nil
}

// Compute aggregates the current net worth from live data — never from a
// stored snapshot: assets are cash (non-CREDIT account balances) +
// investments; liabilities are the current credit card bill cycle +
// remaining debts, all valued in BRL at today's rates. Receivables never
// enter this computation at all — see Breakdown's doc comment for why.
func Compute(ctx context.Context, q Querier) (Breakdown, error) {
	return ComputeValued(ctx, q, newValuation(q, time.Now()))
}

// ComputeValued is Compute with the valuation (and so the rates it used)
// supplied by the caller.
func ComputeValued(ctx context.Context, q Querier, valued *valuation.Valuation) (Breakdown, error) {
	// Cash is the same transactions.CashOnHandByCurrency the timeline anchors
	// on, so both views start from the same native balances. Credit card
	// accounts are excluded there — their balance is owed debt.
	cash, err := transactions.CashOnHandValued(ctx, q, nil, valued)
	if err != nil {
		return Breakdown{}, err
	}
	investments, err := investmentBalance(ctx, q, valued)
	if err != nil {
		return Breakdown{}, err
	}
	// What's currently owed across every credit card is the current bill
	// cycle's eligible transaction total, never financial_accounts.balance
	// (see transactions.CreditCardTransactionTotal for why: the provider
	// balance can include locally-ignored transactions).
	creditCard, err := transactions.CreditCardBalanceValued(ctx, q, valued.ValuedAt, time.Local, valued)
	if err != nil {
		return Breakdown{}, err
	}
	debt, err := payables.RemainingTotal(ctx, q, payables.KindDebt)
	if err != nil {
		return Breakdown{}, err
	}
	b := Breakdown{
		CashBalance:       cash,
		InvestmentBalance: investments,
		CreditCardBalance: creditCard,
		PayablesDebt:      debt,
	}
	return b.TotalsFor(), nil
}

// Snapshot computes today's net worth and upserts it into
// net_worth_snapshots keyed by captured_at (the local calendar day), so
// calling it repeatedly the same day only ever refreshes that one row
// instead of accumulating duplicates.
func Snapshot(ctx context.Context, q Querier) (SnapshotRow, error) {
	return SnapshotValued(ctx, q, newValuation(q, time.Now()))
}

// SnapshotValued is Snapshot with the valuation supplied by the caller; the
// row records its day and every rate it used.
func SnapshotValued(ctx context.Context, q Querier, valued *valuation.Valuation) (SnapshotRow, error) {
	breakdown, err := ComputeValued(ctx, q, valued)
	if err != nil {
		return SnapshotRow{}, err
	}
	rates, err := json.Marshal(valued.Rates)
	if err != nil {
		return SnapshotRow{}, err
	}
	status := ValuationComplete
	if valued.Stale() {
		status = ValuationStale
	}

	now := time.Now()
	capturedAt := formatDate(now)
	nowStored := db.FormatTime(now)
	id := uuid.NewString()

	_, err = q.ExecContext(ctx, `
		INSERT INTO net_worth_snapshots (
			id, captured_at, total_assets, total_liabilities, net_worth,
			cash_balance, investment_balance, credit_card_balance, payables_debt,
			is_backfilled, created_at, updated_at,
			valuation_version, valuation_status, valued_at, rate_policy, valuation_rates
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, FALSE, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (captured_at) DO UPDATE SET
			total_assets = excluded.total_assets,
			total_liabilities = excluded.total_liabilities,
			net_worth = excluded.net_worth,
			cash_balance = excluded.cash_balance,
			investment_balance = excluded.investment_balance,
			credit_card_balance = excluded.credit_card_balance,
			payables_debt = excluded.payables_debt,
			is_backfilled = FALSE,
			updated_at = excluded.updated_at,
			valuation_version = excluded.valuation_version,
			valuation_status = excluded.valuation_status,
			valued_at = excluded.valued_at,
			rate_policy = excluded.rate_policy,
			valuation_rates = excluded.valuation_rates`,
		id, capturedAt,
		money.CanonicalDecimal(breakdown.TotalAssets), money.CanonicalDecimal(breakdown.TotalLiabilities), money.CanonicalDecimal(breakdown.NetWorth),
		money.CanonicalDecimal(breakdown.CashBalance), money.CanonicalDecimal(breakdown.InvestmentBalance), money.CanonicalDecimal(breakdown.CreditCardBalance),
		money.CanonicalDecimal(breakdown.PayablesDebt),
		nowStored, nowStored,
		ValuationVersion, status, db.FormatTime(valued.ValuedAt), valued.RatePolicy, string(rates),
	)
	if err != nil {
		return SnapshotRow{}, err
	}

	return getByCapturedAt(ctx, q, capturedAt)
}

func scanSnapshotRow(scan func(...any) error) (SnapshotRow, error) {
	var (
		s                                                      SnapshotRow
		capturedAtRaw, createdAtRaw, updatedAtRaw              string
		totalAssetsRaw, totalLiabilitiesRaw, netWorthRaw       string
		cashRaw, investmentRaw, creditCardRaw, payablesDebtRaw string
		isBackfilledRaw                                        int
		valuedAtRaw, ratePolicy                                sql.NullString
		ratesRaw                                               string
	)
	if err := scan(dbtypes.ID(&s.ID), dbtypes.Date(&capturedAtRaw), dbtypes.Decimal(&totalAssetsRaw), dbtypes.Decimal(&totalLiabilitiesRaw), dbtypes.Decimal(&netWorthRaw),
		dbtypes.Decimal(&cashRaw), dbtypes.Decimal(&investmentRaw), dbtypes.Decimal(&creditCardRaw), dbtypes.Decimal(&payablesDebtRaw), dbtypes.Bool(&isBackfilledRaw),
		dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw), &s.CurrencyCode,
		&s.ValuationVersion, &s.ValuationStatus, dbtypes.Time(&valuedAtRaw), &ratePolicy, &ratesRaw); err != nil {
		return SnapshotRow{}, err
	}
	s.IsBackfilled = isBackfilledRaw != 0
	var err error
	if valuedAtRaw.Valid {
		valuedAt, err := dbtypes.ParseTime(valuedAtRaw.String)
		if err != nil {
			return SnapshotRow{}, err
		}
		s.Valuation = &valuation.Metadata{BaseCurrency: s.CurrencyCode, ValuedAt: valuedAt, RatePolicy: ratePolicy.String, Rates: map[string]valuation.Rate{}}
		if err := json.Unmarshal([]byte(ratesRaw), &s.Valuation.Rates); err != nil {
			return SnapshotRow{}, fmt.Errorf("parse snapshot valuation rates: %w", err)
		}
	}
	if s.CapturedAt, err = dbtypes.ParseDate(capturedAtRaw); err != nil {
		return SnapshotRow{}, err
	}
	if s.CreatedAt, err = dbtypes.ParseTime(createdAtRaw); err != nil {
		return SnapshotRow{}, err
	}
	if s.UpdatedAt, err = dbtypes.ParseTime(updatedAtRaw); err != nil {
		return SnapshotRow{}, err
	}
	for _, field := range []struct {
		raw string
		dst *decimal.Decimal
	}{
		{totalAssetsRaw, &s.Breakdown.TotalAssets},
		{totalLiabilitiesRaw, &s.Breakdown.TotalLiabilities},
		{netWorthRaw, &s.Breakdown.NetWorth},
		{cashRaw, &s.Breakdown.CashBalance},
		{investmentRaw, &s.Breakdown.InvestmentBalance},
		{creditCardRaw, &s.Breakdown.CreditCardBalance},
		{payablesDebtRaw, &s.Breakdown.PayablesDebt},
	} {
		v, err := decimal.NewFromString(field.raw)
		if err != nil {
			return SnapshotRow{}, err
		}
		*field.dst = v
	}
	return s, nil
}

const snapshotColumns = `id, captured_at, total_assets, total_liabilities, net_worth,
	cash_balance, investment_balance, credit_card_balance, payables_debt, is_backfilled,
	created_at, updated_at, valuation_currency,
	valuation_version, valuation_status, valued_at, rate_policy, valuation_rates`

func getByCapturedAt(ctx context.Context, q Querier, capturedAt string) (SnapshotRow, error) {
	row := q.QueryRowContext(ctx, `SELECT `+snapshotColumns+` FROM net_worth_snapshots WHERE captured_at = ?`, capturedAt)
	s, err := scanSnapshotRow(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return SnapshotRow{}, fmt.Errorf("net worth snapshot for %s not found after upsert", capturedAt)
	}
	return s, err
}

// List returns every snapshot with captured_at in [from, to] (inclusive),
// ordered oldest first — the series the frontend's chart plots.
func List(ctx context.Context, q Querier, from, to time.Time) ([]SnapshotRow, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT `+snapshotColumns+` FROM net_worth_snapshots
		WHERE captured_at >= ? AND captured_at <= ?
		ORDER BY captured_at`, formatDate(from), formatDate(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []SnapshotRow
	for rows.Next() {
		s, err := scanSnapshotRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
