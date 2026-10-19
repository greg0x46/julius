package ledger

import (
	"context"

	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/transactions"
)

// OnIgnored is the cleanup for "a transaction just became ignored", shared by
// every path that can ignore one: manual PUT, rules on new/retroactive
// application and sync updates. A transaction excluded from the totals can
// neither pay down a payable, carry a recurring commitment's occurrence nor
// fund an investment operation. A detached occurrence is untouched: that
// decision is about the occurrence, not about any transaction.
func OnIgnored(ctx context.Context, q transactions.Querier, transactionID string) error {
	if err := payables.UnlinkIfPresent(ctx, q, transactionID); err != nil {
		return err
	}
	if err := recurrences.UnlinkIfPresent(ctx, q, transactionID); err != nil {
		return err
	}
	return investments.UnlinkTransactionIfPresent(ctx, q, transactionID)
}
