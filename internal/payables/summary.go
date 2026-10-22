package payables

import (
	"context"

	"github.com/shopspring/decimal"
)

// LinkSummary is one of a payable's links together with everything read off
// the transaction behind it: how much it currently contributes to
// settled_amount, and the display fields a detail view shows beside it. All
// of it comes from the single transaction row summarize already loads, so a
// caller never goes back to the database for the same row.
//
// Transaction is zero-valued and EffectiveAmount is zero when the link
// points at a transaction row that no longer exists — the same "no data,
// not an error" treatment linkEffectiveAmount gives a missing transaction.
// Such a link still appears here: it is one of the payable's links, it just
// contributes nothing.
type LinkSummary struct {
	Link            Link
	EffectiveAmount decimal.Decimal
	Transaction     LinkedTransactionSummary
	// CountsAsSettlement is true only when the real transaction is linked
	// to the payable's accounting scenario through a settlement. A link with
	// no settlement behind it stays visible — it is a real link, just one
	// made before the payable had a plan — but allocations in other scenarios
	// must never affect the official payable balance.
	CountsAsSettlement bool
}

// Summary is a Payable's derived state: settled/remaining amounts, the
// status they imply, and the links they were derived from. None of it is
// ever stored — it is recomputed from the payable's links on every read,
// which is the invariant this package exists to protect (see
// .specs/motores-de-dominio.md, princípio 1).
//
// Summarize is the one place that recomputation lives. Callers that need
// the numbers — the HTTP layer's payable DTO, the scenario handlers'
// "how much is left to plan" — go through here
// rather than re-deriving it, so they cannot drift apart.
//
// Links carries the rows the amounts came from rather than a bare count, so
// a caller rendering the links (the payable detail view) reuses this one
// pass instead of reloading every link and every transaction behind it.
type Summary struct {
	Settled   decimal.Decimal
	Remaining decimal.Decimal
	Status    Status
	Links     []LinkSummary
}

// Summarize derives p's current settled/remaining/status from its links.
func Summarize(ctx context.Context, q Querier, p Payable) (Summary, error) {
	return summarize(ctx, q, p)
}

// SummarizeAll is Summarize for a whole set of payables at once, keyed by
// payable id. It answers the set in two queries — the links for all of
// them, then the transactions behind those links — where a loop over
// Summarize would cost two per payable.
//
// Every payable in list has an entry, including one with no links at all;
// a caller ranging over its own list never has to check for a missing key.
func SummarizeAll(ctx context.Context, q Querier, list []Payable) (map[string]Summary, error) {
	return summarizeAll(ctx, q, list)
}

func summarize(ctx context.Context, q Querier, p Payable) (Summary, error) {
	all, err := summarizeAll(ctx, q, []Payable{p})
	if err != nil {
		return Summary{}, err
	}
	return all[p.ID], nil
}

// summarizeAll derives a Summary for every payable in list in two queries —
// the links for the whole set, then the transactions behind them — rather
// than the query-per-link a naive loop over Summarize would cost. It is the
// one derivation the whole package's read paths share.
func summarizeAll(ctx context.Context, q Querier, list []Payable) (map[string]Summary, error) {
	linksByPayable, err := allLinkSummaries(ctx, q, list)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Summary, len(list))
	for _, p := range list {
		out[p.ID] = summaryFrom(p, linksByPayable[p.ID])
	}
	return out, nil
}

// summaryFrom is the pure half of the derivation: given a payable and the
// links already read off the database, decide what counts and do the
// arithmetic.
func summaryFrom(p Payable, all []LinkSummary) Summary {
	amounts := make([]decimal.Decimal, 0, len(all))
	for _, ls := range all {
		if ls.CountsAsSettlement {
			amounts = append(amounts, ls.EffectiveAmount)
		}
	}
	settled := settledAmount(p.StartingSettledAmount, amounts)
	remaining := remainingAmount(p.TotalAmount, settled)
	return Summary{
		Settled:   settled,
		Remaining: remaining,
		Status:    statusFor(remaining),
		Links:     all,
	}
}

// allLinkSummaries loads every link of every payable in list together with
// the transaction row behind it, in two queries total. One load of each
// transaction serves every use of it: the effective amount and the detail
// view's display fields.
func allLinkSummaries(ctx context.Context, q Querier, list []Payable) (map[string][]LinkSummary, error) {
	payableIDs := make([]string, len(list))
	for i, p := range list {
		payableIDs[i] = p.ID
	}
	linksByPayable, err := linksFor(ctx, q, payableIDs)
	if err != nil {
		return nil, err
	}

	var transactionIDs []string
	seen := map[string]bool{}
	for _, rows := range linksByPayable {
		for _, l := range rows {
			if !seen[l.TransactionID] {
				seen[l.TransactionID] = true
				transactionIDs = append(transactionIDs, l.TransactionID)
			}
		}
	}
	snapshots, err := loadTransactions(ctx, q, transactionIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]LinkSummary, len(linksByPayable))
	for payableID, rows := range linksByPayable {
		summaries := make([]LinkSummary, len(rows))
		for i, loaded := range rows {
			l := loaded.Link
			ls := LinkSummary{Link: l, EffectiveAmount: decimal.Zero, CountsAsSettlement: loaded.countsAsSettlement}
			if snapshot, ok := snapshots[l.TransactionID]; ok {
				ls.EffectiveAmount = snapshot.linkEffectiveAmount()
				ls.Transaction = LinkedTransactionSummary{
					OccurredAt:  snapshot.occurredAt,
					Description: snapshot.description,
				}
			}
			summaries[i] = ls
		}
		out[payableID] = summaries
	}
	return out, nil
}

// RemainingTotal sums remainingAmount across every payable of kind — the
// "quanto ainda se deve / ainda se tem a receber" figure.
//
// A settled payable contributes zero by construction (remainingAmount is
// zero exactly when statusFor reports StatusSettled), so a caller wanting
// only the open ones needs no status filter here.
//
// This is the payables half of a debt figure and nothing more: what is
// riding on the current credit-card bill cycle belongs to the Lançamentos
// engine (transactions.CreditCardTransactionTotal), and folding the two
// together is the caller's composition — this package deliberately does not
// reach across for it.
func RemainingTotal(ctx context.Context, q Querier, kind Kind) (decimal.Decimal, error) {
	list, err := List(ctx, q, &kind)
	if err != nil {
		return decimal.Decimal{}, err
	}
	summaries, err := summarizeAll(ctx, q, list)
	if err != nil {
		return decimal.Decimal{}, err
	}
	total := decimal.Zero
	for _, p := range list {
		total = total.Add(summaries[p.ID].Remaining)
	}
	return total, nil
}
