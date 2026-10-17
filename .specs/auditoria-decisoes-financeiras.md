# Shared financial decision audit (ARCH-03)

This audit covers `money`, `transactions`, `ledger`, `timeline`, planning
(`payables`, `scenarios`, `projections`, `recurrences`) and `investments`.
Only equivalent decisions are consolidated; similar arithmetic alone is not
evidence of a shared rule. Existing financial behavior is preserved.

## Confirmed duplication

| Decision | Evidence / consumers | Owner and resolution |
| --- | --- | --- |
| Manual BRL link eligibility | `payables.EligibilityForLink` and `recurrences.EligibilityForReconciliation` independently checked ignored inclusion, required direction and effective BRL money, in that order. Neither checks status, category, amount positivity or account. | `money.BRLLinkIneligibility` owns these checks. Consumers retain kind-to-direction mapping and their distinct already-linked/already-reconciled checks and API reason types. |
| Calendar month length | `money.monthLength` calculated UTC days between month boundaries; recurrence and card-cycle readers already used `dates.DaysInMonth`. | `dates.DaysInMonth`; money period bucketing now uses it too. Timezone conversion still belongs to `money.PeriodFor`. |

Regression tests: `TestBRLLinkEligibilityAcrossPlanningModules` pins the
shared decision and each consumer's reason precedence;
`TestMonthBucketsAgreeWithRecurringProjections` checks period ends against
recurrence projection dates and Timeline totals across short/leap months.

## Already shared rules / negative findings

| Area inspected | Current owner and evidence | Finding |
| --- | --- | --- |
| Classification, effective money and totals eligibility | `money.Classify`, `SelectEffectiveMoney`, `Eligibility`, `MovedCash`; transaction list/single-item reads use `buildView`; Timeline reads those Items. Payable snapshots also call money rules. | Already shared. Timeline signs cash amounts while transaction totals accumulate direction-specific magnitudes: different output representations of the same classification, not independent classification decisions. |
| Ledger writes and annotations | `ledger.Service` and `ApplyNewTransactionDecisions` orchestrate transactions, learned categories and automation in one DB transaction. | No duplicate financial calculation found in ledger. Orchestration stays there. |
| Account balances and reporting conversion | `transactions.AccountBalance` / `CashOnHand`; `reporting.Convert` for transaction and Timeline reporting. | Already shared. Manual posted movements versus provider anchors are distinct source policies; cash versus reportable flow is intentionally different. |
| Payable remaining and settlement | `payables.Summarize` / `summaryFrom`; both single and batched historical reads reuse the same calculation. | Already shared; no separate planning or investment copy found. |
| Plan direction and recurring reconciliation | `scenarios.SignedAmount`, `recurrences.NewReconciler` / `OccurrencesInRange`; projections compose these and Timeline consumes projections. | Already shared. Generated installments use Go month rollover; recurring schedules clamp to month end. These are different existing calendar policies, not equivalent rules. |
| Investment principal and income/cost reporting | `investments.ReconciledTransactionAmounts`, `ManualReportingEntries`; transactions applies the principal clamp in `view.applyInvestmentTransfer`, Timeline consumes Items and manual reporting entries. | Already shared. Investment operation replay, cost basis and yield cash events have different purposes from bank cash/reporting; no equivalent rule to extract across those engines was verified. |

Existing cross-engine tests pin totals/cash separation and planning opt-in:
`TestInvestmentPrincipalIsNotIncome`, `TestTransfersCreateNoIncomeOrExpense`,
`TestHypotheticalMovementsAreNotRealized` and
`TestBuildSeriesMixedCurrencyReportingAgreesWithLedger`.

## Similar decisions with different behavior (unchanged)

Allocation suppression is **not equivalent** today. `scenarios.ListPlanInstallments`
suppresses an installment when the sum of allocations is nonzero;
`projections.allocationRealizations` marks it realized when any linked allocation
is positive. The baseline schema does not require positive allocation amounts,
and `scenarios.CreateRealization` does not validate them, so negative or
cancelling allocations distinguish these readers. Ownership remains in the
respective readers until a separate behavior decision is made; no consolidation
is safe under this issue's preserve-behavior requirement.

Investment reconciliation accepts operation-specific direction/currency and
allocation limits (`investments.reconciliations`), unlike the BRL planning-link
policy. Its bank-line reader uses SQL amount fallback and the stored signed
value, rejects zero, and checks BRL before ignored inclusion; it is not
equivalent to `money.SelectEffectiveMoney` plus the planning-link checks.
Those checks remain owned by investments. Historical ignored-entry
behavior and plan installment signs also remain unchanged; see the
[invariant matrix](invariantes-financeiros.md#divergences-and-intended-changes-not-implemented).

## Regression coverage

The new cross-module tests preserve eligibility reasons for both directions,
including ignored/direction/currency/conflict precedence and linkable zero
amounts. Calendar regression fixtures create a manual ledger expense of
BRL 25.50 and an unreconciled monthly expense of BRL 100 on day 31. In
28-, 29-, 30- and 31-day months, the occurrence lands on the reporting bucket's
last day: real expense remains BRL 25.50, projected expense totals BRL 125.50,
and a BRL 1,000 cash anchor ends at BRL 874.50.
