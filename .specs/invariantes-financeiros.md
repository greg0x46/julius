# Financial invariants (decision and ownership matrix)

**Living**, cross-cutting doc: describes the **current** behavior of the core
financial rules, which module owns each one and which test pins it. It is not a
design doc — the why of each engine is in
[`motores-de-dominio.md`](motores-de-dominio.md), and per-feature detail in the
`contextos/<context>/reference.md` files (in particular
[`transacoes`](contextos/transacoes/reference.md),
[`pendencias`](contextos/pendencias/reference.md),
[`cenarios`](contextos/cenarios/reference.md),
[`investimentos`](contextos/investimentos/reference.md) and
[`patrimonio-liquido`](contextos/patrimonio-liquido/reference.md)).

Cross-engine tests live in `internal/invariants`, a test-only package.
`TestMatrixReferencesExistingTests` fails if this matrix cites a test that does
not exist — when renaming a test, update its row.

When a change alters any rule below, update its row in the same PR. A divergence
found and **not** fixed goes into the "Divergences" section at the end; it is
never "fixed" in passing.

## Matrix

| Concept | Rule (invariant) | Current behavior | Owning module | Pinning test |
| --- | --- | --- | --- | --- |
| Monetary precision | Money and decimal constraints retain exact values without floating-point casts. | Postgres stores native NUMERIC; SQLite stores decimal text and checks positivity by sign/digits. Positive amounts below floating-point range and large fractional amounts round-trip unchanged. | `money.CanonicalDecimal`, `internal/db/migrations` | `TestBaselineExactDecimalChecks`, `TestBaselineNativeTypes` |
| Account balance | Synced accounts use provider balances; manual banks use opening balance + posted movements + adjustments; categorizing, ignoring or marking as transfer never changes it. A `CREDIT` account is not cash. | BRL sum of provider balances for synced accounts and derived balances (opening + posted undeleted movements + adjustment deltas) for manual banks. Foreign amounts are excluded without rates. It is the Timeline's anchor for today and Net Worth's cash. | `transactions.CashOnHand` (`internal/transactions/balance.go`) | `TestCategorizationDoesNotAlterProviderBalances`, `TestCashOnHandExcludesCreditAccounts`, `TestCashOnHandScopedToAccountIDsFiltersUntypedAccounts`, `TestManualAccountBalanceBothDialects`, `TestManualAccountLifecycle`, `TestBuildSeriesExcludesForeignAccountAmounts` |
| Historical balance reconstruction | Past days come from today's anchor by reversing what moved money; a transfer counts as a cash movement. | The Timeline walks items with `MovesCash()` (transfer yes, ignored no), plus dated manual adjustments. Future posted manual movements already included in the anchor are reversed before walking their dates, so they affect the curve once. The net-worth backfill reverses every eligible synced entry on a synced account and posted manual-account movement, plus dated manual adjustments, passing `Considered` and kind `""` — including ignored ones (see Divergences). Opening balance is the manual history baseline. | `timeline.BuildSeries`/`buildPoints`, `networth.Backfill` (`cashDeltasDescending`), `money.MovedCash` | `TestBuildSeriesKeepsTransferCategorizedCashMovement`, `TestBuildSeriesStillDropsIgnoredTransactions`, `TestBackfillReversesTransferCategorizedTransactions`, `TestIgnoredRowDivergesBetweenTimelineAndBackfill`, `TestBackfillManualAccountMovementsAndAdjustments`, `TestBuildSeriesManualAdjustmentsReconstructBalance`, `TestBuildSeriesFutureManualMovementAlreadyInAnchor` |
| Income and expense | Only an eligible entry enters the total; the first exclusion reason is the one reported. Order: `ignored` > `unclassified` > `ineligible_status` > `missing_money_pair` > `zero_value` > `transfer_category`. | `POSTED` and `PENDING` pass the same status check, including valid pending card expenses; status is not settlement. Each provider installment row counts once in its report period (provider date by default; bill due date or +1 month under paid_at), with separate projected cash timing. The order is load-bearing: `transfer_category` comes last because `MovedCash` retains it as a cash-flow candidate. | `money.Eligibility`, `money.MovedCash`, `transactions.Query` totals | `TestEligibility`, `TestEligibilityPrecedenceMatrix`, `TestPendingCardInstallmentsCountOnceInTheirPeriods` |
| Transfer | A transfer between own accounts is neither income nor expense, but moves cash on both legs. | A category of kind `transfer` yields `ReasonTransferCategory`; outside totals, inside cash. Card debt still reduces by a bill payment categorized as a transfer; its bank debit changes provider cash without recognizing the purchases as expenses again. | `money.Eligibility`, `transactions.Query`, `transactions.SpendingByCategory`, `transactions.CreditCardTransactionTotal` | `TestTransfersCreateNoIncomeOrExpense`, `TestQueryExcludesTransferCategoryFromTotals`, `TestQueryExcludesAutomaticSamePersonTransferFromTotals`, `TestSpendingByCategoryExcludesTransfers`, `TestBuildSeriesTransferMovesCashButIsNeitherIncomeNorExpense`, `TestBuildSeriesDropsAnUnsettledTransfer`, `TestBuildSeriesDropsAnUnclassifiedTransfer`, `TestCreditCardTotalIgnoresTransferCategory`, `TestPendingCardPurchaseAndBillSettlementHaveSeparateEffects` |
| Ignored | Ignored beats transfer on precedence and is not discounted from the reported balance. Ignoring drops the links that depended on the entry. | `SetInclusion(ignored)` fires `OnIgnoredHook`: `payables.UnlinkIfPresent` (link + settlement/allocation) and, on the investments path, `investments.UnlinkTransactionIfPresent`. An ignored row cannot be reconciled. | `transactions.SetInclusion`, `payables.UnlinkIfPresent`, `investments.UnlinkTransactionIfPresent` | `TestUnlinkIfPresentTriggeredByIgnoringTransaction`, `TestSetInclusionInvokesOnIgnoredHook`, `TestIgnoredBankLineCannotBeReconciled`, `TestUnlinkTransactionDropsEveryParcelAndTheOrphanPivot`, `TestObligationCannotSettleTwice` |
| Obligation (payable) | An entry settles at most one obligation, once. Settled/remaining are recomputed on read, never stored, and remaining stays in `[0, total]`. | `payables.EligibilityForLink` + `CreateLink` (conflict if already linked; UNIQUE on `payable_transaction_links.transaction_id`). Only settlements of the accounting-source scenario count in `Summarize`. | `payables.CreateLink`, `payables.Summarize` | `TestObligationCannotSettleTwice`, `TestCreateLinkConflictsOnAlreadyLinked`, `TestPayableTransactionLinksUniqueAcrossKinds`, `TestRemainingAmountClampedToZero`, `TestRemainingAmountClampedToTotal` |
| Plan/recurrence realization | An entry realizes at most one complete event (settlement or recurrence reconciliation). | `scenarios.RealizeEvent` returns `ErrTransactionAlreadyRealized` on hitting index `uq_scenario_realizations_one_complete_event_transaction`. Allocations (`allocation`) are not in that index. Automatic recurrence candidates are scoped to the occurrence's month and the commitment's account (when specified), regardless of category; the automation rule decides the match. | `scenarios.RealizeEvent`, `recurrences.Reconciler` | `TestSameTransactionCannotReconcileTwoOccurrences`, `TestObligationCannotSettleTwice`, `TestReconcilerMatchesRegardlessOfCategory`, `TestCandidatesByMonthScopesToAccountAndMonth` |
| Investment | Reconciled principal (contribution/withdrawal) is neither income nor expense, but the bank's cash is preserved. Income, tax and cost count. | The reconciled part becomes `ReasonInvestmentTransfer` and leaves `ReportableAmount`; only the unreconciled remainder counts. Manual income/tax/cost enter via `investments.ManualReportingEntries`, without moving cash; the part already reconciled to a bank line is not counted again. | `investments/reconciliations.go`, `investments.ReconciledTransactionAmounts`, `transactions/query.go` (`applyInvestmentTransfer`), `timeline.BuildSeries` | `TestInvestmentPrincipalIsNotIncome`, `TestInvestmentAllocationSeparatesReportingFromCash`, `TestReconciledInvestmentSplitFlowsThroughQueryAndCategories`, `TestBankLineSplitsBetweenOperations` |
| Hypothetical | A standalone scenario movement never becomes a real entry nor changes balance. | A `standalone` scenario is born inactive; it enters only by active selection or explicit `ScenarioIDs`, as `TierHipotetico`/`SourceScenario`. An explicit empty selection emits nothing. An event linked to a real entry is suppressed (no double counting). | `projections.List`, `timeline.BuildSeries`, `scenarios.CreateScenario` | `TestHypotheticalMovementsAreNotRealized`, `TestListSelectionAndEventKeys`, `TestListReturnsUnifiedKindsAndRealizationState` |
| Reprocessing | Running the same processing again does not duplicate financial effect. | PENDING → POSTED retains transaction identity and user decisions, counts the expense once, and replaces rather than adds a corrected amount. Resync with an equal hash is a no-op; `SetInclusion` is idempotent; re-applied automation (including combined category/reconcile rules) counts only real changes and never overrides a manual decision; `ApplyAutomatic` writes only with no prior decision; the day's snapshot is a one-row upsert; backfill never overwrites; link and realization are protected by UNIQUE; a repeated reconciliation is refused. | `syncsvc`, `transactions.SetInclusion`, `automation`, `categories`, `networth.Snapshot`/`Backfill`, `payables.CreateLink`, `investments.CreateReconciliation` | `TestSyncPendingToPostedKeepsIdentityAndUserDecisions`, `TestRepeatedProcessingDoesNotDuplicateFinancialEffects`, `TestExecuteSecondRunDetectsUnchangedRecords`, `TestExecuteSecondRunDetectsUnchangedInvestment`, `TestExecuteResyncNeverOverridesManualInclusionDecision`, `TestSetInclusionIsIdempotent`, `TestApplyRetroactivelyCountsMatchedAndActuallyChanged`, `TestCombinedCategoryAndReconcileApplication`, `TestSnapshotIsIdempotentForTheSameDay`, `TestBackfillNeverOverwritesAnExistingSnapshot` |

## Divergences and intended changes (not implemented)

Nothing here was changed. Each item is pinned as current behavior and left for
its own issue.

1. **Plan installment sign** (open note 4 of `motores-de-dominio.md`):
   `scenarios.SignedAmount` negates a `debt_plan` amount instead of forcing the
   direction; an installment stored with a negative amount reads as an inflow.
2. **Ignored in the past: Timeline × net-worth backfill.** The Timeline drops
   the ignored entry from the walk (duplicate/chargeback), and the backfill
   reverses it from the anchor (money that really left). Today's balance is the
   same in both; on days before the entry the two charts differ by its amount
   (`TestIgnoredRowDivergesBetweenTimelineAndBackfill`). The header comment of
   `networth.Backfill` still says an ignored entry is "never reversed", which
   contradicts the code and the `cashDeltasDescending` comment.
3. **Allocation is not unique per entry.** The complete-event index covers only
   `settlement` and `reconciliation`; the same entry can be allocated to more
   than one scenario installment, and `RealizeEvent` does not check the sum of
   allocations against the entry's amount. A payment of 800 today suppresses
   two installments of 800 (`TestHypotheticalMovementsAreNotRealized`).
