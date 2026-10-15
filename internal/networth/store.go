package networth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
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

// cashBalance is the asset side's cash figure: the same
// transactions.CashOnHand the timeline anchors on, so the two views can
// never disagree. Credit card accounts are excluded there — their balance
// is owed debt, handled by creditCardBalance below.
func cashBalance(ctx context.Context, q Querier) (decimal.Decimal, error) {
	return transactions.CashOnHand(ctx, q, nil)
}

// creditCardBalance is what's currently owed across every credit card,
// computed the same way handlePayableTotalOwed's "Dívida total" homepage
// widget does — the current bill cycle's eligible transaction total, never
// financial_accounts.balance directly (see the doc comment on
// transactions.CreditCardTransactionTotal for why: the provider balance can
// include locally-ignored transactions). Reusing that exact function is
// what keeps this figure and the homepage's in agreement.
func creditCardBalance(ctx context.Context, q Querier) (decimal.Decimal, error) {
	return transactions.CreditCardTransactionTotal(ctx, q, "BRL")
}

// investmentBalance sums every synced investment's current balance. Balances
// are stored as TEXT to stay exact, so the summing happens here in Go with
// decimal.Decimal rather than through a float-lossy SQL SUM.
func investmentBalance(ctx context.Context, q Querier) (decimal.Decimal, error) {
	manual, err := investments.ManualNetWorth(ctx, q)
	if err != nil {
		return decimal.Zero, err
	}
	rows, err := q.QueryContext(ctx, `SELECT balance FROM financial_investments WHERE balance IS NOT NULL AND (currency_code IS NULL OR currency_code = 'BRL')`)
	if err != nil {
		return decimal.Decimal{}, err
	}
	defer rows.Close()
	total := manual
	for rows.Next() {
		var raw string
		if err := rows.Scan(dbtypes.Decimal(&raw)); err != nil {
			return decimal.Decimal{}, err
		}
		amount, err := decimal.NewFromString(raw)
		if err != nil {
			return decimal.Decimal{}, fmt.Errorf("parse investment balance %q: %w", raw, err)
		}
		total = total.Add(amount)
	}
	return total, rows.Err()
}

// Compute aggregates the current net worth from live data — never from a
// stored snapshot: assets are cash (non-CREDIT account balances) +
// investments; liabilities are the current credit card bill cycle +
// remaining debts. Receivables never enter this computation at all — see
// Breakdown's doc comment for why.
func Compute(ctx context.Context, q Querier) (Breakdown, error) {
	cash, err := cashBalance(ctx, q)
	if err != nil {
		return Breakdown{}, err
	}
	investments, err := investmentBalance(ctx, q)
	if err != nil {
		return Breakdown{}, err
	}
	creditCard, err := creditCardBalance(ctx, q)
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
	breakdown, err := Compute(ctx, q)
	if err != nil {
		return SnapshotRow{}, err
	}

	now := time.Now()
	capturedAt := formatDate(now)
	nowStored := db.FormatTime(now)
	id := uuid.NewString()

	_, err = q.ExecContext(ctx, `
		INSERT INTO net_worth_snapshots (
			id, captured_at, total_assets, total_liabilities, net_worth,
			cash_balance, investment_balance, credit_card_balance, payables_debt,
			is_backfilled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, FALSE, ?, ?)
		ON CONFLICT (captured_at) DO UPDATE SET
			total_assets = excluded.total_assets,
			total_liabilities = excluded.total_liabilities,
			net_worth = excluded.net_worth,
			cash_balance = excluded.cash_balance,
			investment_balance = excluded.investment_balance,
			credit_card_balance = excluded.credit_card_balance,
			payables_debt = excluded.payables_debt,
			is_backfilled = FALSE,
			updated_at = excluded.updated_at`,
		id, capturedAt,
		money.CanonicalDecimal(breakdown.TotalAssets), money.CanonicalDecimal(breakdown.TotalLiabilities), money.CanonicalDecimal(breakdown.NetWorth),
		money.CanonicalDecimal(breakdown.CashBalance), money.CanonicalDecimal(breakdown.InvestmentBalance), money.CanonicalDecimal(breakdown.CreditCardBalance),
		money.CanonicalDecimal(breakdown.PayablesDebt),
		nowStored, nowStored,
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
	if err := scan(dbtypes.ID(&s.ID), dbtypes.Date(&capturedAtRaw), dbtypes.Decimal(&totalAssetsRaw), dbtypes.Decimal(&totalLiabilitiesRaw), dbtypes.Decimal(&netWorthRaw),
		dbtypes.Decimal(&cashRaw), dbtypes.Decimal(&investmentRaw), dbtypes.Decimal(&creditCardRaw), dbtypes.Decimal(&payablesDebtRaw), dbtypes.Bool(&isBackfilledRaw),
		dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw)); err != nil {
		return SnapshotRow{}, err
	}
	s.IsBackfilled = isBackfilledRaw != 0
	var err error
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
	created_at, updated_at`

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
