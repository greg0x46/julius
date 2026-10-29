# Goals, obligations and cash planning

[All documentation](../README.md) · [Investments](investments.md) ·
[Recurring payments](transactions.md#track-recurring-income-or-expenses)

## Set an investment target

1. Open Goals (`/metas`), add a goal, and enter its name, positive BRL target,
   optional target date and notes.
2. Review current value and progress. All goals measure the same global
   investment value, including gains/losses; they do not reserve money or assign
   holdings to individual goals. Cash-only and net-worth targets are not supported.
3. Add a future target date to see the estimated monthly contribution and
   projected value with no new contributions. Edit the target as your plan changes.

For example, two synthetic goals of BRL 5,000 and BRL 12,000 both compare with
the same invested total; their targets are not added to your wealth. Goals stay
in BRL even when reports use another reference currency. Missing FX on a foreign
holding can make progress unavailable; it is not silently excluded.

Contribution estimates assume end-of-month deposits and repetition of the
portfolio's historical monthly return, measured over up to the trailing year
with contribution/withdrawal timing accounted for. Negative returns are possible.
With fewer than 30 days of usable capital history, no measurable holdings or a
total loss, Julius assumes zero appreciation and discloses that assumption.
Partial return history is flagged. No contribution estimate appears for an
undated goal or a target date today/in the past. These are estimates, not promised
returns or scheduled transfers.

## Track a debt or receivable

1. Open Payables (`/pendencias`) and create a debt or receivable with its BRL
   total. If some was settled before tracking began, enter the initial remaining
   amount, rather than the amount already paid.
2. Open its detail and link actual transactions: outflows for a debt, inflows
   for a receivable. Candidates must be non-ignored BRL movements; one movement
   cannot settle multiple payables.
3. Review the settled and remaining amounts. Unlink a mistaken transaction;
   deleting the obligation preserves the real transactions.

An obligation records what is owed, not a second expense or income. Its remaining
amount changes through actual linked payments, not because a plan was created.
Credit-card bills are tracked separately; the total-owed figure also includes
open card debt. Avoid registering the same card debt again as a separate payable.

In the detail timeline, create a payment/receipt plan and generate installments
by cadence, start date and count or fixed amount. Link actual payments and
allocate them to installments to reflect realization. Review deviations and
readjust the plan when needed. Unrealized installments remain projected;
a planned payment alone never settles the obligation or changes today's cash.

## Read the cash forecast

Open Home (`/`) and select the chart period. Inspect a day for its events, the
first negative day and lowest balance. The forecast combines current bank cash,
known real movements, card bill due dates, unrealized plan installments and
unreconciled recurring commitments. Card purchases use the bill due date, with
estimated timing when bill information is missing. Bills due before today are
treated as paid; check overdue/unreported payments before trusting the curve.

Today's cash is distinct from today's end-of-day projected balance: commitments
and bills still due today can reduce the latter. Manual entries on connected
accounts do not alter provider cash. Investment appreciation does not become
spendable bank cash. Actual income/expense totals also differ from cash movements:
transfers affect account balances without being income or expense.

The daily allowance looks from today through month end, independently of the
chart's selected period. It caps a constant daily spend against every projected
low point and today's cash, assuming spending at each day's end. Future income
can cover known outflows but its surplus does not increase this allowance.
It is neither a reserved budget nor a guarantee: missing expenses, stale balances
and incorrect dates change the result.

Native cash is consolidated to BRL using reference-day FX, and real foreign
flows use reporting-day FX. The reference currency preference expresses that
BRL curve using one current rate across its dates; it does not forecast future
exchange rates. Missing conversions make the forecast unavailable and stale
rates are disclosed. Planning amounts (commitments, installments and hypothetical
events) remain BRL; native account amounts remain available in Accounts and cards.

## Try a hypothetical decision

1. Under Settings → Scenarios (`/configuracoes/cenarios`), create a named
   scenario. New standalone scenarios start outside the projection.
2. Open it and add hypothetical income/expense events with BRL amounts and
   expected dates, such as a synthetic BRL 600 trip expense next month.
3. Turn on its projection switch to include it in Home's active forecast;
   turn it off to return to the forecast without it. Delete unwanted planned
   events or the scenario when finished exploring.

This switch selects planning data; it does not create bank transactions, change
realized investment value, advance a goal or pay an obligation. Keep the
scenario outside the projection when you do not want that assumption included.
There is no separate side-by-side simulation screen. Reconcile commitments and
allocate actual installment payments as they happen so planned and real events
are not counted twice; a hypothetical entry is never proof that money moved.
