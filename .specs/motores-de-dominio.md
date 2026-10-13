# Domain Engines

> Architecture umbrella doc (not a feature doc): the reusable domain engines
> that several features build on, and the principles connecting them. It
> describes the domain at the *ideal* level; where the code diverges, that is
> stated explicitly. For the current, literal state of each feature see
> `.specs/contextos/<context>/reference.md` (index: `.specs/README.md`).

## Why this doc exists

Julius grew feature by feature, and each feature is documented well. What
no single place recorded is the layer **below** the features: a handful of
generic engines reused by many screens and endpoints. Without this map it is
easy to (a) reinvent an engine that already exists, generalized, in another
package, or (b) over-generalize something that is really feature-specific.

## What counts as a domain engine

A candidate is accepted only if it:

1. **Is reused across multiple contexts** — not used by a single screen or
   endpoint.
2. **Processes, not just stores.** A passive catalog (categories) does not
   qualify just for being widely referenced; it needs behavior (matching,
   computation, merging) that others reuse.
3. **Is not tied to a specific provider.** Pluggy is the main data provider
   today, but manual entries and CSV import already exist; an engine must make
   sense whatever sits behind it.
4. **Is described at the ideal level**, not the code's current state;
   divergences are explicit notes, not facts about the engine.

## The engines

### 1. Ledger (Portuguese: Lançamentos; name still provisional)

**Packages:** `internal/money`, `internal/transactions`, `internal/ledger`

The central cash-flow entity and the rules that decide what counts toward
totals: movement classification, effective amount, inclusion/exclusion
(`money`: `Classify`, `SelectEffectiveMoney`, `Considered`/`Ignored`,
`CategoryKind`). Origin-agnostic: Pluggy, manual entries and statement imports
(`origin` on `financial_transactions`, see `contextos/transacoes/reference.md`)
fill the same table and go through the same rules. Ingestion is a data
provider, not part of the engine.

Reporting exclusions assert different things.
**Ignored** means "this should not be here" — a chargeback, a duplicate.
**A category of kind `transfer`** means "this is real, but it is my own money
moving between my accounts": counting it would count the same money twice,
leaving one account and entering the other. The category is the only one whose
kind carries a rule; `expense`/`income` stay labels. Ignored wins on
precedence, as the user's most explicit statement (`money.Eligibility`).

An explicit own-account transfer link makes the ownership decision independently
of categories: both native movements remain intact, but linked principal gets
`linked_transfer` after ordinary eligibility checks and stays outside reporting
totals. The execution rate is recomputed from those actual native amounts;
only an explicitly recorded separate fee enters expenses. Unlinking removes the
annotation and resumes the existing decisions. Like transfer categories and
investment principal, linked transfer principal still satisfies `MovedCash`.


The order of checks in `money.Eligibility` is load-bearing, not cosmetic:
`MovedCash` reads the `transfer_category` reason as "the money really left the
account", so that reason is checked **last**, after everything that asks
whether any movement happened at all. A transfer the provider has not settled,
or whose movement type cannot be given a direction, has not moved anything yet;
reporting it as a cash movement made the timeline and net worth reverse it from
the anchor (the unclassified one with the sign flipped).

Balance note: on synced accounts, the provider-reported balance **is** the reference
for money in accounts, and **neither** path above corrects it — both are
*reporting* decisions, not claims that the money did not leave the bank. An
ignored entry and a transfer really moved money, so they stay inside the
balance; discounting them would report cash the user does not have. For the
same reason historical net-worth reconstruction reverses both when walking
backwards (`internal/networth` `cashDeltasDescending`), and card debt keeps
being reduced by the bill payment even when it is categorized as a transfer
(`transactions.consideredCardTransactionTotal`) — both pass `""` as the kind to
`money.Eligibility`, on purpose.

Where these rules *decide* a number: income/expense totals (both paths apply)
and card debt (only the `ignored` path). There the question is "how much will
this bill charge me", which an inclusion decision legitimately shapes, so the
total comes from the cycle's eligible entries, not from the reported balance
(`transactions.CreditCardTransactionTotal`). Manual bank accounts instead derive native balance from opening balance +
posted undeleted transaction amounts + explicit adjustment deltas; their
inclusion/category annotations do not alter cash either. Synced balances retain
provider ownership. Cash on hand is BRL-only without a rate source and comes from
`transactions.CashOnHand`, a single implementation shared by the Timeline
(today's anchor) and Net Worth.

The matrix of each of these rules, its owning module and the test that pins it
is in [`invariantes-financeiros.md`](invariantes-financeiros.md).

### 2. Rules engine

**Package:** `internal/rules`

Composable, pure, stateless matching core (`Condition`/`Operator`/
`LogicOperator`/`Matches`): tests a `MatchCandidate` against field/operator/value
conditions with AND/OR logic. Consumed by Automation and, indirectly, by
Recurrences via Automation (see "Not engines" below).

### 3. Automation

**Package:** `internal/automation`

Connects a rule's outcome to an effect on real entries: a `Rule` with
conditions (via the rules engine) and one or more actions (`ignore`,
`set_category`, `reconcile`), applicable to new entries or retroactively.

### 4. Payables

**Package:** `internal/payables`

Tracks a **real target**: a total that exists (debt, receivable, or goal) and is
paid down/reached by linked real entries, with settled/remaining/status always
recomputed on read, never persisted.

Scope generalized beyond the current code: today `Kind` covers only
`debt`/`receivable`; the direction is to include goals (e.g. "save R$10,000 by
December") under the same engine — same total + real link + recomputed
progress mechanics, with no real debt behind it. That requires extending `Kind`
and the link-eligibility logic (`eligibility.go`), which today derives the
accepted flow direction from the debt/receivable `Kind`.

Name note: "Payables" stops describing the engine well once goals are included.
Kept for now; alternatives ("Payment plan", "Commitment", "Financial target")
are for a future refactor.

### 5. Scenarios

**Package:** `internal/scenarios` (read boundary: `internal/projections`)

The **hypothetical layer**: projections authored by the user, never synced
facts. One entity (`Scenario`/`ScenarioTransaction`), several kinds:

- **Attached to a `Payable`**: the installment plan of a specific payable —
  `debt_plan`/`receivable_plan` today; following the generalization of engine 4,
  a future `goal_plan` would be just another `Kind` of this family.
- **Free** (`standalone`): a "what if" without any `Payable` — "trip", "new job".
- **Recurring** (`recurring`): a recurring commitment (salary, rent); see
  "Not engines".

Core principle: **never contaminates real totals by default.** A scenario is
never "activated" as persisted state — it enters a report only when the caller
passes `scenario_id` explicitly or includes it in a simulation selection.
Activation is a query parameter, not a stored flag.

### 6. Timeline

**Package:** `internal/timeline`

The merge engine: joins real entries and the unified scenario projection flow
into a single `Series`, with an explicit `CertaintyTier` per `Entry`
(Realized / Confirmed / Projected / Hypothetical). Every presentation layer
(Home cards and chart) reads this same `Series` and never recomputes locally.

Sources: Ledger + Scenarios (**2**). Recurrences used to be a parallel entity
with its own occurrence engine; they are now `Scenario{Kind: recurring}`, so
`BuildSeries` talks to a single planning source (`projections.List`). The
engine does **not** know Payables directly — the link to a `Payable` is
encapsulated inside Scenarios (`Scenario.PayableID`/`Kind`), which decides
whether a `ScenarioTransaction` becomes tier Confirmed (attached to a Payable)
or Hypothetical (`standalone`).

**Divergence note:** `CertaintyTier` is produced and carried to the API, but no
screen shows it today — see `contextos/home/reference.md`, Notes. Principle 4
below still holds as a domain yardstick; what is missing is its UI expression.

## Not engines (features/entities composed from the engines above)

- **Recurrences** (`internal/recurrences`): a feature, not an engine — a
  cash-flow commitment known in advance (salary, rent), reconciled against real
  entries by composing the rules engine + Automation (a `Rule` with a
  `reconcile` action points at the commitment; without that rule and without a
  manual decision it never reconciles). The user's manual decision beats the
  rule — see `contextos/recorrencias/reference.md`, "Manual reconciliation". A
  recurring commitment **is** a `Scenario{Kind: recurring}` plus its row in
  `scenario_recurring_schedules`, and the scenario id is the identity that
  projection, automation target and occurrence decisions share. The
  `recurrences` package remains the occurrence and reconciliation engine
  (calendar, `Reconciler`), no longer a parallel entity with its own table.
- **Categorization** (`internal/categories`): category catalog + history.
  Widely referenced but a passive catalog with no matching/computation/merging
  logic of its own — fails criterion 2.
- **Net worth** (`internal/networth`): net worth snapshots/history. Not
  investigated in depth; fails criterion 3 (not reused across contexts today).
- **Sync/Pluggy** (`internal/pluggy`, `internal/syncsvc`, `internal/worker`):
  an ingestion provider, not a domain engine — violates criterion 3 directly.
  It feeds the Ledger together with manual entries
  (`internal/transactions/manual.go`) and statement import
  (`internal/statementimport`), neither of which has anything to do with Pluggy. Ingestion never writes user
  decisions (cross-cutting principle 2).

## Cross-cutting principles (apply to every new engine)

1. **Recompute on read; never persist a derived value.**
   `Payable.SettledAmount/RemainingAmount/Status` and
   `ScenarioTransaction.Status` are computed from source data on every read.
2. **Isolate external data from local annotation.** No planning entity
   (`ScenarioTransaction`) has a hard FK to real entries. Where a link exists
   it is an explicit link table (`payable_transaction_links` and
   `scenario_realizations`, the latter covering `settlement`, `allocation` and
   `reconciliation`) — the real entry never "knows" it is used for planning.
   The `reconciliation` row is the edge case that shows principle 1 too: what it
   stores is the user's *decision* about an occurrence (reconcile with X, or
   detach), not the reconciliation state — that is still recomputed on every
   read, consulting the decision before the automation rule.
3. **Hypothetical is explicit opt-in, never a global ambient.** Scenarios and
   projections enter a report only when the parameter asks — never a globally
   enabled "simulation mode" flag.
4. **Shared certainty vocabulary.** The Timeline's 4 tiers
   (Realized/Confirmed/Projected/Hypothetical) are the common yardstick — any
   new engine that produces "something that will still happen" should fit one
   of them instead of inventing a new degree of certainty.
5. **A day is a day.** Every engine that reasons in calendar days (timeline
   entries, scenario installments, bill due dates) truncates with
   `internal/dates.Day` — UTC midnight, from the caller's wall clock. Each
   engine had grown its own copy; one shared definition is what lets two engines
   that cannot depend on each other compare the same day with `==`. The
   deliberate exception is bill-cycle math (`transactions/cardtotal.go`), which
   closes in `time.Local` on purpose — a different convention, not a copy.

   `internal/dates.DaysInMonth` follows the same rule for the other calendar
   question engines share: how many days a month has, to fit a monthly cadence
   into a shorter month (closing on the 31st lands on the 30th in November, not
   on December 1st). It takes no `*time.Location` on purpose — the count is
   calendar arithmetic, not an instant, so no time zone changes it; the three
   copies it replaced disagreed only on route, never on the number.

## Open notes (for future evaluation, not decisions)

1. Final name for the Ledger engine.
2. Final name for generalized Payables (debt/receivable/goal).
3. Whether generalized Payables needs a growing closed `Kind` (`goal` as one more
   value) or a more open link-eligibility model — undecided.
4. Installment sign: `scenarios.SignedAmount` **negates** a `debt_plan` amount
   instead of forcing the flow direction, and nothing rejects an installment
   stored with a negative `Amount` — which would then read as an inflow in a
   debt plan. Behavior preserved from earlier code; choosing between validating
   on write or forcing the sign on read changes real totals, so it is recorded
   here and not resolved in passing during a refactor.
