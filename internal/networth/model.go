// Package networth computes and snapshots the user's net worth (assets
// minus liabilities) over time. There is no background worker: a snapshot
// for "today" is written on every read (Snapshot, called from the HTTP
// handler before List), which is enough for a self-hosted app used
// sporadically. The cash figure comes from transactions.CashOnHand, the
// same implementation internal/timeline anchors its series on.
package networth

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/fx"
)

// Breakdown is a point-in-time decomposition of net worth into the
// categories the frontend's composition card shows. Assets = CashBalance +
// InvestmentBalance; Liabilities = CreditCardBalance + PayablesDebt;
// NetWorth = Assets - Liabilities.
//
// Receivables (payables.KindReceivable) are deliberately not part of this
// model at all: unlike a bank balance or investment, money someone else
// owes the user is a future possibility, not a realized asset — it carries
// collection risk this package has no way to quantify. A debt
// (PayablesDebt), by contrast, is a firm obligation regardless of whether
// it's ever collected on the other side, so it stays a liability.
//
// CreditCardBalance is the current bill cycle's eligible transaction total
// (transactions.CreditCardTransactionTotal), not financial_accounts.balance —
// the same figure the homepage's "Dívida total" widget folds in as
// future_installments_total, so the two pages agree.
//
// A payable debt is counted as a liability independent of any CREDIT
// account balance: nothing in this codebase guarantees a debt payable is
// "backed by" a specific credit card, so the two are summed as distinct
// liabilities rather than deduplicated — a known simplification.
type Breakdown struct {
	CashBalance       decimal.Decimal
	InvestmentBalance decimal.Decimal
	CreditCardBalance decimal.Decimal
	PayablesDebt      decimal.Decimal
	TotalAssets       decimal.Decimal
	TotalLiabilities  decimal.Decimal
	NetWorth          decimal.Decimal
}

// TotalsFor derives TotalAssets/TotalLiabilities/NetWorth from the
// category fields — factored out so Compute and Snapshot's row-scan both
// produce a fully consistent Breakdown from the same math.
func (b Breakdown) TotalsFor() Breakdown {
	b.TotalAssets = b.CashBalance.Add(b.InvestmentBalance)
	b.TotalLiabilities = b.CreditCardBalance.Add(b.PayablesDebt)
	b.NetWorth = b.TotalAssets.Sub(b.TotalLiabilities)
	return b
}

// Snapshot mirrors one net_worth_snapshots row: a Breakdown captured on a
// given calendar day.
type SnapshotRow struct {
	Valuation        *fx.Metadata
	ValuationVersion int
	ValuationStatus  string
	ID               string
	CapturedAt       time.Time
	Breakdown        Breakdown
	// IsBackfilled marks a row written by Backfill (reconstructed from past
	// transactions) rather than Snapshot (a live read of today's
	// provider-synced balances). Breakdown.InvestmentBalance is always zero
	// on a backfilled row — see Backfill's doc comment for why.
	IsBackfilled bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

const (
	ValuationComplete      = "complete"
	ValuationReconstructed = "reconstructed_partial"
	ValuationUnavailable   = "unavailable"
)
