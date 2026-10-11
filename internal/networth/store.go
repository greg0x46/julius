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
	"github.com/greg0x46/julius/internal/fx"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/transactions"
)

// Querier is satisfied by both *sql.DB and *sql.Tx — identical in shape to
// payables.Querier, reused as an alias so this package's functions can pass
// q straight through to the payables package without a type assertion.
type Querier = payables.Querier

// dateLayout is the on-disk format for captured_at, a date-only column —
// mirrors internal/recurrences' dateLayout.
const dateLayout = "2006-01-02"

func formatDate(t time.Time) string { return t.UTC().Format(dateLayout) }

// investmentBalance sums every synced investment's current balance. Balances
// are stored as TEXT to stay exact, so the summing happens here in Go with
// decimal.Decimal rather than through a float-lossy SQL SUM.
func investmentBalance(ctx context.Context, q Querier, valuation *fx.Valuation) (decimal.Decimal, error) {
	manual, err := investments.ManualNetWorthValued(ctx, q, valuation)
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
	total := manual
	for rows.Next() {
		var raw string
		var currency sql.NullString
		if err := rows.Scan(&raw, &currency); err != nil {
			return decimal.Decimal{}, err
		}
		amount, err := decimal.NewFromString(raw)
		if err != nil {
			return decimal.Decimal{}, fmt.Errorf("parse investment balance %q: %w", raw, err)
		}
		holdings = append(holdings, holding{amount, fx.Currency(&currency.String)})
	}
	if err := rows.Err(); err != nil {
		return decimal.Zero, err
	}
	rows.Close()
	for _, h := range holdings {
		value, err := valuation.Convert(ctx, q, h.amount, h.currency)
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
// remaining debts. Receivables never enter this computation at all — see
// Breakdown's doc comment for why.
func Compute(ctx context.Context, q Querier) (Breakdown, error) {
	return ComputeValued(ctx, q, fx.New(time.Now()))
}

// ComputeValued uses live balances; the valuation timestamp controls FX only.
func ComputeValued(ctx context.Context, q Querier, valuation *fx.Valuation) (Breakdown, error) {
	cash, err := transactions.CashOnHandValued(ctx, q, nil, valuation)
	if err != nil {
		return Breakdown{}, err
	}
	investments, err := investmentBalance(ctx, q, valuation)
	if err != nil {
		return Breakdown{}, err
	}
	creditCard, err := transactions.CreditCardBalanceValued(ctx, q, valuation.ValuedAt, time.Local, valuation)
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
// net_worth_snapshots keyed by captured_at (the UTC calendar day), so
// calling it repeatedly the same day only ever refreshes that one row
// instead of accumulating duplicates.
func Snapshot(ctx context.Context, q Querier) (SnapshotRow, error) {
	now := time.Now()
	valuation := fx.New(now)
	breakdown, err := ComputeValued(ctx, q, valuation)
	if err != nil {
		return SnapshotRow{}, err
	}

	capturedAt := formatDate(now)
	nowStored := db.FormatTime(now)
	id := uuid.NewString()
	rates, err := json.Marshal(valuation.Rates)
	if err != nil {
		return SnapshotRow{}, err
	}

	_, err = q.ExecContext(ctx, `
		INSERT INTO net_worth_snapshots (
			id, captured_at, total_assets, total_liabilities, net_worth,
			cash_balance, investment_balance, credit_card_balance, payables_debt,
			is_backfilled, created_at, updated_at,
			reference_currency, valued_at, rate_policy, fx_rates, valuation_version, valuation_status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, 1, ?)
		ON CONFLICT (captured_at) DO UPDATE SET
			total_assets = excluded.total_assets,
			total_liabilities = excluded.total_liabilities,
			net_worth = excluded.net_worth,
			cash_balance = excluded.cash_balance,
			investment_balance = excluded.investment_balance,
			credit_card_balance = excluded.credit_card_balance,
			payables_debt = excluded.payables_debt,
			is_backfilled = 0,
			updated_at = excluded.updated_at,
			reference_currency = excluded.reference_currency,
			valued_at = excluded.valued_at,
			rate_policy = excluded.rate_policy,
			fx_rates = excluded.fx_rates,
			valuation_version = excluded.valuation_version,
			valuation_status = excluded.valuation_status`,
		id, capturedAt,
		money.CanonicalDecimal(breakdown.TotalAssets), money.CanonicalDecimal(breakdown.TotalLiabilities), money.CanonicalDecimal(breakdown.NetWorth),
		money.CanonicalDecimal(breakdown.CashBalance), money.CanonicalDecimal(breakdown.InvestmentBalance), money.CanonicalDecimal(breakdown.CreditCardBalance),
		money.CanonicalDecimal(breakdown.PayablesDebt),
		nowStored, nowStored, valuation.ReferenceCurrency, db.FormatTime(valuation.ValuedAt), valuation.RatePolicy, string(rates), ValuationComplete,
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
	)
	var referenceCurrency, valuedAt, ratePolicy sql.NullString
	var rates string
	if err := scan(&s.ID, &capturedAtRaw, &totalAssetsRaw, &totalLiabilitiesRaw, &netWorthRaw,
		&cashRaw, &investmentRaw, &creditCardRaw, &payablesDebtRaw, &isBackfilledRaw,
		&createdAtRaw, &updatedAtRaw, &referenceCurrency, &valuedAt, &ratePolicy, &rates, &s.ValuationVersion, &s.ValuationStatus); err != nil {
		return SnapshotRow{}, err
	}
	s.IsBackfilled = isBackfilledRaw != 0
	if s.ValuationVersion != 0 {
		at, err := db.ParseTime(valuedAt.String)
		if err != nil {
			return SnapshotRow{}, err
		}
		s.Valuation = &fx.Metadata{ReferenceCurrency: referenceCurrency.String, ValuedAt: at, RatePolicy: ratePolicy.String}
		if err := json.Unmarshal([]byte(rates), &s.Valuation.Rates); err != nil {
			return SnapshotRow{}, err
		}
	}
	var err error
	if s.CapturedAt, err = time.Parse(dateLayout, capturedAtRaw); err != nil {
		return SnapshotRow{}, err
	}
	if s.CreatedAt, err = db.ParseTime(createdAtRaw); err != nil {
		return SnapshotRow{}, err
	}
	if s.UpdatedAt, err = db.ParseTime(updatedAtRaw); err != nil {
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
	created_at, updated_at, reference_currency, valued_at, rate_policy, fx_rates, valuation_version, valuation_status`

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
