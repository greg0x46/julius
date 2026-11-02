# Recurrences

> Living reference — what exists and works today. Keep it updated when the
> feature changes. Not a domain engine — a feature built on the rules engine +
> Automation (see "Not engines" in `docs/development/architecture.md`).

## What it is

A cash-flow commitment known in advance (salary, rent), registered manually or
drafted from historical patterns, independent of any specific real transaction, and reconciled against real
transactions through an automation rule.

## State vs. ideal

Matches the design: a recurring commitment **is** a `Scenario{Kind: recurring}`
plus its row in `scenario_recurring_schedules`, and the scenario id is the
identity shared by projection, automation `reconcile` target and occurrence
decisions. `internal/recurrences` still exists as the occurrence and
reconciliation engine (calendar, `Reconciler`), reading those tables. Its
package doc comment in `model.go` still describes the older, parallel design.

## Backend

In the Timeline, an occurrence becomes an entry only if it was **not**
reconciled, with tier **Projected**; a reconciled one produces no entry because
the real transaction already carries it. The candidates an occurrence tries to
reconcile are scoped to its month and, when specified, the commitment's
account. Category is not an implicit constraint: uncategorized and differently
categorized transactions can match the automation rule. Month scoping prevents
one month's transaction from reconciling another month's occurrence.

On a credit-card account the projected entry is dated like the card's real
entries: on the bill's due date (`CardDueDates.ProjectedEntryDate`, falling back
to the occurrence day when the card has no bill history), not on the schedule
day. Only the projected date moves. The occurrence day stays the event key and
the date the reconciliation matches on, so a purchase still reconciles the
occurrence of its own month. `projections.List` reads occurrences up to two
months before the window for these commitments, since a bill can fall due
inside the window for a purchase made before it.

`internal/recurrences` — `RecurringCommitment` (`Kind` income/expense, `Amount`,
`CategoryID`, `AccountID`, `Cadence` monthly/annual, `DayOfMonth`/`MonthOfYear`,
`StartDate`/`EndDate`, `IsActive`). No matching criteria of its own: occurrences
come only from the schedule (`OccurrencesInRange`); reconciliation against real
transactions happens at read time via `internal/rules` (`Matches`/
`MatchCandidate`), driven by `internal/automation` rules with a
`reconcile` action (`ListActiveReconcileTargets` — without that rule and without a
manual decision, the commitment never reconciles).
Candidates expose description, card number, account name/institution, absolute
effective amount and day of month, matching the fields used during transaction
automation application.

The commitment store supports both SQLite and native Postgres UUID/boolean
columns. Nullable schedule categories read as an empty `CategoryID`; writes
still require a category. `TestCommitmentLifecycleBothDialects` covers creation,
reads, updates, activation and uncategorized schedule reads on both dialects.

### Historical discovery

`GET /api/recurrence-suggestions` returns read-only BRL drafts from
`recurrences.Discover`. It reads the shared ledger history, including Pluggy,
CSV and manual entries, without FX conversion. No transactions, schedules,
automation rules or reconciliation decisions are created. The current UTC day
is the analysis cutoff; identical data and cutoff produce identical ordered
results and evidence IDs. The recurring page exposes these results through **Discover recurrences**, alongside
manual creation. Dismissed and accepted patterns are hidden persistently.

Heuristics are deliberately conservative:

- Group normalized descriptions by account, card number and cash-flow direction.
  Normalize case, punctuation, whitespace and full dates (`YYYY-MM-DD` or
  `DD/MM/YYYY`); retain merchant identifiers. Reject installment metadata and
  installment-like descriptions before grouping. Installment metadata is honored
  even when the provider omits the card number.
- Use posted, undeleted, included transactions with effective BRL money. Exclude
  ignored/duplicate decisions, transfer categories, linked transfer principal,
  investment reconciliations (including partial principal), zero amounts and
  future entries. Identical same-day amounts in a group count once, with the
  lexicographically first transaction ID as evidence.
- Monthly patterns need at least four distinct months, one observation per
  month, at least 75% coverage and no gap longer than one missing month. The
  latest observation must be within two calendar months of the cutoff.
- Annual patterns need at least three consecutive yearly observations in the
  same month (at least two years of evidence). The latest must be within
  fourteen calendar months. Two annual payments never suffice.
- Choose the day anchor with the smallest total calendar deviation, clamping
  short months; choose the smaller anchor on a tie. Every observation must be
  within three days. Multiple different purchases in one period and irregular
  dates reject the group, even when their descriptions match.
- Use the exact median magnitude as the positive estimate, with observed minimum
  and maximum amounts as an uncertainty range, not a forecast interval. Reject
  a maximum above twice the minimum. A range above 2% of the median is
  `variable`; otherwise it is `fixed`. Fixed patterns with no missing periods,
  deviations of at most one day and six monthly observations (or three annual
  observations) receive `high` confidence; others receive `medium`. Confidence
  is a heuristic label, not a probability. Variable patterns always use medium.

Each result contains `draft` (the existing recurring write fields),
`currency_code: "BRL"`, `amount_kind`, exact decimal strings `minimum_amount` and
`maximum_amount`, `confidence`, `reasons`, `transaction_ids`, `card_number`, a stable
`key` and `evidence` rows (ID, date, description and exact amount).
The schedule starts at the next anchored occurrence after the cutoff. The latest
observed description supplies the name. An active category held by a strict
majority supplies `category_id`; otherwise it is empty and `category_required`
explains that the owner must choose one before submitting the draft to
`POST /api/recurring-scenarios`. Calendar, frequency and amount reason codes
explain the evidence; `missing_months` marks tolerated gaps.

Transactions explicitly reconciled to recurring scenarios are removed from
history. Existing schedules, including paused or ended schedules, suppress a
candidate with the same direction, cadence, annual month and compatible account
when either its normalized name matches, or its category matches, its day is
within three days and its amount lies within 10% of the observed range. This
also avoids equivalent drafts for renamed schedules. A schedule without an
account covers any account. Distinct card/account contexts remain distinct
suggestions. Drafts are recomputed; only dismissal and acceptance decisions are
stored in `recurrence_suggestion_decisions`, keyed by normalized merchant pattern,
account, card, direction and cadence. New observations do not revive a dismissed
pattern. Decision rows contain no foreign keys to real transactions.

Synthetic detection, evidence, precision, exclusions, determinism and read-only
checks live in `discovery_test.go` and `discovery_store_test.go`; the store tests
run on SQLite and PostgreSQL. HTTP tests pin the draft contract, empty array,
no-store response and database failure handling.

### Read-only rule suggestions

`internal/recurrences/suggestions.Generate` accepts a proposed commitment with
an eventual scenario ID and at least two historical observations. Each observation
includes its intended occurrence date and the transaction. Dates must follow the
proposed calendar, but can predate its start. `FromSuggestion` accepts a discovery
`recurrences.Suggestion`, the eventual target ID, an optional owner-selected
category and history containing all evidence IDs. A missing category still
requires owner input. Rule generation and preview remain read-only services. The review endpoint and
recurrence drawer use them before approval.

Drafts use the existing `automation.Write` contract and remain inactive. Conditions
are AND-combined: equal descriptions or their longest shared word sequence,
the observed amount midpoint and outward-rounded percentage tolerance, and the
smallest observed day window. Consistent account labels (name, falling back to
institution) and card numbers add exact
conditions. One historical account also restricts the proposed schedule by ID.
Only a shared active historical category adds an optional `set_category` action;
existing manual category decisions retain precedence when that action is saved
and applied later. Saving still requires normal target/category existence checks.

`Preview` takes the draft, an occurrence period, complete transaction history for
the affected calendar months (including all evidence), existing rules/schedules
and manual overrides. Missing evidence and duplicate transaction IDs are rejected.
Include overrides outside the period when they claim loaded transactions. It
uses the current matcher, cash-movement filter and `Reconciler`; it does not apply
actions. It backtests the proposed calendar as active over the historical period,
without changing the actual schedule's future start/end or activation. Existing
competing schedules retain their actual date bounds and activation.
Results include every supplied transaction's matched/unmatched/ambiguous
status, failed conditions, reasons and competing rule/scenario IDs, plus the actual
resolved occurrences. Multiple matches retain the current earliest-match winner
while requiring review. Matching entries outside discovery evidence are possible
false positives, not automatically accepted evidence. Existing active rules are
reported on condition overlap; existing scenario conflicts use their actual
resolution and account restrictions. An existing rule reserving the target is
reported even when inactive.
Optional category actions also flag condition matches outside reconciliation's
account or occurrence-month scope, since category automation has no implicit
schedule restriction. These warnings do not change the recurrence resolution.

Missing or single-word shared descriptions and unrestricted accounts flag broad
drafts. Wrapped day windows or evidence paid outside its intended occurrence's
month are unsupported and require review. A wrapping condition is valid in the
rules engine but does not relax month-scoped reconciliation. Ordinary month-end
clamping, including leap February, remains supported.

Both operations accept values only and cannot persist financial data, category
decisions or resolved links. Preview preserves manual links/detachments and reports
cross-scenario competition without imposing a new global rule ordering.
`internal/recurrences/suggestions/suggestions_test.go` covers draft validation,
exact boundaries, noisy text, false positives, overlaps, eligibility, manual
precedence, calendar boundaries and repeatable reads without input mutation.

### Review and approval

`POST /api/recurrence-suggestions/review` accepts `key`, `draft` (the recurring
write contract) and optional `conditions`. Omit conditions to generate them;
subsequent previews use the edited AND conditions. An active non-transfer
category is required. The response supplies conditions, a preview token,
warning codes, `needs_review` and historical rows with status, reasons, failed
conditions and competing rule/scenario IDs. Each row identifies discovery evidence.
The review period covers complete months between the first and last observation;
manual links outside that period are still included when resolving competition.

`POST /api/recurrence-suggestions/accept` accepts `items` (1–100 review requests,
each including `token`, edited `conditions` and `acknowledge`). It returns a
result for each item with `key`, `scenario_id`, `status` and `error`. Each item
uses its own serializable transaction: create the recurring scenario and schedule,
create an active reconcile-only rule, and record the acceptance receipt. A failed
item leaves no partial writes and does not undo successful items. Retrying the
same accepted key and token returns its original scenario ID. No category action
or retroactive automation pass runs. Ledger rows, balances, categories and manual
reconciliation decisions remain unchanged. Existing projection and occurrence
services consume accepted schedules and rules normally.

Approval recomputes discovery and preview against current history, schedules,
rules and overrides. Changed history or preview rejects the old token; a covered,
dismissed or no-longer-detected pattern cannot be accepted. An edited proposal
with an equivalent existing name, direction, cadence and compatible account is
also rejected. Independent items can be approved together; a new overlap caused
by an earlier item requires another review. Warnings require acknowledgment;
cross-month day windows remain unsupported even after acknowledgment. Invalid
categories, schedules and conditions receive validation feedback. Database errors
return safe failure messages.

`POST /api/recurrence-suggestions/dismiss` accepts `key` and stores the preference
without creating a schedule or rule. The UI removes the suggestion after success.
There is no automatic creation or dismissal undo control in this flow.

`workflow_test.go` tests both dialects for approval, replay, dismissal, stale
history, active categories, ambiguity, unsupported windows, rollback and ledger
preservation. HTTP tests cover the review contract and mixed bulk results. Frontend
tests cover review gating, edits invalidating previews, acknowledgment, individual
and bulk approval, stale errors and dismissal. Browser journeys approve a real
suggestion on desktop and mobile and verify unchanged ledger data.

### Manual reconciliation

Manual link eligibility shares `money.BRLLinkIneligibility` with payables:
ignored inclusion, required direction and effective BRL money, in that order.
Recurrences still maps income/expense to direction and checks whether the
transaction is already reconciled afterward. Status and category do not gate
manual links.

On top of that automatic resolution sits a layer of **user decision**, and only
it is persisted (`scenario_realizations` with `relation_type = 'reconciliation'`,
one row per occurrence, unique by `(scenario_id, occurrence_date)` and by
`transaction_id` among full events). The resolved state is still recomputed on
every read — principle 1 of `docs/development/architecture.md` — and the table is
the explicit link table principle 2 asks for.

`Reconciler` (`reconciliation.go`) is the only place that resolves an
occurrence, used by both the Timeline and the HTTP layer. Order:

1. `detached` override → not reconciled, and the rule can **not** claim it back.
   This is what makes "detach" work: the rule is re-evaluated from scratch on
   every read, so deleting a record would not be enough;
2. `linked` override → the transaction picked by hand (`OriginManual`);
3. otherwise the automation rule, scoped to the occurrence's month
   (`CandidatesByMonth`, moved from `internal/timeline`) — `OriginRule`.

In step 3, transactions already linked by hand are removed from the candidate
pool: without that, a transaction reconciled in March would also be matched by
the rule in April and the same money would suppress two projections.

Manual-link eligibility lives in `eligibility.go`, mirroring
`internal/payables`: not ignored, BRL, direction compatible with the `Kind`, and
not reconciled to another occurrence. It deliberately differs from
`CandidatesByMonth` by **not** requiring the commitment's account or limiting
the transaction to the occurrence's month (manual links have a 45-day reach).
Neither path requires the commitment's category.
The transaction-side reconciliation read uses the same cash-movement eligibility
as occurrence resolution: transfer-category payments retain both their current
reconciliation and available occurrences, while ignored transactions offer neither.

A transaction marked ignored loses its link (`UnlinkIfPresent`, in
`ledger.OnIgnored` together with `payables`, also fired by sync updates); a detached occurrence is unaffected,
because the decision is about the occurrence.

## HTTP routes

The `{id}` of every route below is the scenario id — the same one projection,
the automation `reconcile` target and occurrence decisions use.

`GET/POST /api/recurring-scenarios`,
`PUT/PATCH/DELETE /api/recurring-scenarios/{id}`. A `PUT` that changes the
calendar deletes the manual decisions (links and detachments) whose date is no
longer an occurrence of the new schedule, releasing their transactions.

The write collection is a sibling of `/api/scenarios`, not a sub-collection
`/api/scenarios/recurring`: a literal segment where `/api/scenarios/{id}`
expects a wildcard makes `ServeMux` reject the pair
`/api/scenarios/recurring/{id}` × `/api/scenarios/{id}/…` on the same method —
a panic at registration, not a runtime error. An occurrence remains a
sub-resource of the scenario, because it belongs to the scenario and not to the
write form.

Reconciliation — an occurrence's address is the scenario plus the day, since an
occurrence has no stored id:
`GET /api/scenarios/{id}/occurrences`,
`GET .../occurrences/{date}/candidates`,
`PUT .../occurrences/{date}/reconciliation` (`{"state":"linked",
"transaction_id":…}` or `{"state":"detached"}`),
`DELETE .../occurrences/{date}/reconciliation` (back to automatic).
On the transaction side, read-only:
`GET /api/transactions/{id}/reconciliation` — writes go through the endpoints
above, so there is a single place that decides what a valid reconciliation is.

## Frontend

`/recorrencias` — each commitment expands to show its occurrences in the period,
with the transaction that reconciled them and the actions Conciliar com… /
Desconciliar / Voltar ao automático.
`/transacoes` — "Conciliação" section in the detail drawer.

Discovery on `/recorrencias` opens a block above the list: a flush card shaped like
the list itself, with a summary line (candidate count and the monthly outflow and
inflow of the monthly candidates; phones show only the outflow) and the candidates
below. From `xl` they are a table (name, type, amount, category, recurrence,
evidence count); below, stacked rows. Amounts use `Money` (outflow in the text
colour, inflow green); a variable amount shows the average with its observed range.
Only non-default states carry a `StatusTag`: medium confidence and "Revisada".
A candidate opens the review drawer from its "Revisar" button (table) or its
row, which carries a "Revisar ›" cue (stacked). The selection checkbox sits
beside it and stays disabled until the candidate was reviewed; while nothing is
reviewed, the summary says how to unlock it (open Revisar). "Select reviewed"
and "Create selected (n)" are in the card summary on wide screens and in a
`BottomActionBar` on phones. While that bar shows, `Page` (`hasBottomActionBar`)
drops the title-row actions, so "Descobrir" and "Nova recorrência" are hidden
until the reviews are created or reset by refreshing the discovery.

The review drawer (the `form-drawer` skin: full-screen on phones) shows the
pattern found (confidence, observed range, reasons), the recurring fields, the
condition editor, the preview with its warnings and acknowledgment, and the
transactions the rule finds as stacked rows with a status tag (matched and
ambiguous rows; the months' unrelated transactions are left out, except evidence
the rule misses). It is the only place with "Dispensar" and the single-candidate
"Criar recorrência e regra". Opening it already simulates the rule when the
pattern has a category; without one, the owner picks it and chooses "Atualizar
prévia". A valid (acknowledged) preview marks the candidate as reviewed, and
closing the drawer keeps that. A status line names what is pending
(simulating, category, changed data, warnings to confirm), and the footer's
primary button is "Atualizar prévia" until the preview is valid, then "Criar
recorrência e regra". Changing any field invalidates the preview, and
"Atualizar prévia" simulates again. Only current, valid, acknowledged reviews can be
selected for bulk creation. Failed items stay visible, with their error above the
list, for another review.

`/automacoes` — a reconcile automation can also apply a category using the same
conditions. New rules default that optional action to the commitment's
category; it is editable and clearable. Reconciliation eligibility is unchanged.

## Monetary precision

Commitment and occurrence amounts, including transaction reconciliation options,
use exact decimal strings in HTTP responses. Fractional cents are preserved;
the UI rounds only read-only BRL display ([policy](../monetary-precision.md)).

## MCP reads

`list_recurring_commitments` exposes active/inactive schedules through
`recurrences.List` and the REST formatter. `list_projection_events` reads dated
occurrences through `projections.List`, including realization origin/state;
`get_timeline` suppresses reconciled occurrences through the existing engine.
Amounts stay BRL for schedules/events. Active non-standalone commitments are
selected by default; hypotheticals need explicit IDs.

Tool inputs, output bounds and dev validation: [MCP tool reference](../mcp-tools.md).

## MCP writes

`create_recurring_commitment`, `update_recurring_commitment` and
`delete_recurring_commitment` share the REST schedule request conversion and
`recurrences` store flows. Updates replace schedule fields, including activation;
calendar, positive exact BRL amount, category/account IDs are validated at preview.
Derived occurrences/reconciliation use the existing engine. Calendar edits remove
manual links/detachments whose dates cease to be occurrences; real transactions
remain intact. Deleting removes the schedule and its decisions, and fails with an
actionable conflict while an automation reconcile action targets it.

All require `mcp:write` and separate browser owner approval. Previews include the
current schedule, manual decisions and targeting rules; changes invalidate the
approval. Update/delete recheck the preview and mutate within one serializable
transaction, including removal of occurrence decisions. Active schedules enter base MCP forecasts, with realized occurrences
suppressed as before. Synthetic lifecycle, date/amount/ID validation, month-end
clamping, orphan cleanup, stale previews, linked-rule conflicts and authorization
are covered by `internal/httpapi/mcp_planning_writes_test.go` and
`internal/mcp/planning_writes_test.go` on SQLite and Postgres.

## Validate discovery in dev

1. Sign in and create a synthetic BRL manual account. Add six posted expenses
   with the same description and amount on the same day of each of the last six
   months, including the current month. Do not use installments or transfers.
2. Open `/recorrencias` and choose **Discover recurrences**. Expect one monthly
   candidate with no status tag (high confidence) and "6 transações" as evidence;
   opening it simulates the rule and lists the six matching transactions.
3. Change two amounts within the supported range (maximum no more than twice
   minimum). Reload discovery; expect a variable estimate, the "Confiança média"
   tag and the observed amount range ("varia de … a …").
4. Open the candidate; when it has no category, select an active one and choose
   **Update preview**. Inspect the schedule, conditions and matching transactions.
   Edit a field and verify creation is disabled until another preview succeeds.
5. Create the recurrence and rule. Expect the suggestion to disappear, one new
   recurrence and one active reconcile rule, with no historical category changes.
   Repeat on a narrow viewport. Discovering or previewing alone must create nothing.
6. With two other synthetic patterns, review both and create the selected items.
   Alternatively dismiss one from its review and reload discovery; it must remain
   hidden. On a phone the bulk actions appear in the bottom bar once a candidate
   is reviewed (opened with a valid preview).
