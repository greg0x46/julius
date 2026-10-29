# Context decisions

Shared decisions consolidated from the legacy context references and financial
audit. Read [architecture](architecture.md), [financial invariants](financial-invariants.md)
and [API contracts](api-contracts.md) first. These sections preserve technical
boundaries; the linked context inventories retain route, screen and tool details
until their separate migration. Unimplemented proposals are not current contracts.

## Transactions and categorization

- Category precedence is `manual > rule > learned > automatic`. Learned
  categories copy the latest manual choice for normalized description plus
  movement type (including installment-suffix normalization); they apply to
  subsequent arrivals, not retroactively to old entries. Manual installment
  siblings survive rule propagation.
- On sync insert, card-payment heuristics and provider categories precede
  learned categories and rules. Changed updates rerun rules, preserving manual
  decisions; unchanged hashes do not replay them.
- Manual entries are soft-deleted. Category/inclusion event tables are
  append-only and restrict hard deletion. Ignoring an entry invokes
  `ledger.OnIgnored` to remove dependent payable settlements/allocations,
  recurrence links and investment reconciliations. Ignoring/deleting a
  transfer participant unlinks the transfer without deleting its counterparts.
- Own-account transfer links annotate native legs without rewriting their
  amounts. Execution FX is derived from the actual native principal; only an
  explicit separate fee is expense. Transfer participants and investment
  reconciliations are mutually exclusive.
- Reporting `occurred_at` uses each provider installment's date. Under
  `paid_at`, cards use the bill due date, otherwise transaction date plus one
  clamped month; grouping/ordering retain occurrence dates. Projected cash
  timing separately uses bill/cycle forecasts. Card debt is cycle-specific,
  not the provider account balance.

Inventory: [transactions](../../.specs/contextos/transacoes/reference.md),
[categories](../../.specs/contextos/categorias/reference.md),
[automation](../../.specs/contextos/automacao/reference.md).

## Planning and reconciliation

- Payable summaries use only settlements of the accounting-source scenario.
  One transaction can settle at most one payable. Combined payable/card debt is
  composed by HTTP, because neither engine owns both amounts.
- Manual payable, recurrence and standalone links share
  `money.BRLLinkIneligibility`: ignored, direction, effective BRL, then the
  consumer's conflict check. Status, category and zero magnitude do not gate
  these links. Investment reconciliation has different operation-specific
  direction, currency and capacity rules; it must not reuse this policy blindly.
- Automatic recurrence matching uses the occurrence's month and optional
  account; category is not an implicit filter. Conditions decide the match.
  Manual links do not require the schedule account/month (available candidates
  have a 45-day reach). A detached override wins over a linked override or rule;
  manually linked transactions leave the automatic candidate pool. Schedule
  calendar edits release decisions whose dates cease to be occurrences.
- Complete-event uniqueness covers settlement/reconciliation, not allocations.
  Allocation suppression differs: `ListPlanInstallments` suppresses nonzero
  sums, while `projections` marks any positive linked allocation realized.
  Negative/cancelling allocations distinguish them; the schema does not enforce
  positivity. Preserve this distinction until a deliberate rule change.
- Plan-generated installments use month rollover; recurrence schedules clamp.
  Plan sign and allocation-capacity limitations remain documented in the
  [matrix divergences](financial-invariants.md#divergences-and-intended-changes-not-implemented).
  Fixed-amount generation/readjustment uses exact quotient/remainder and rejects
  more than 10,000 installments before replacing a plan.

Inventory: [payables](../../.specs/contextos/pendencias/reference.md),
[scenarios](../../.specs/contextos/cenarios/reference.md),
[recurrences](../../.specs/contextos/recorrencias/reference.md).

## Investments, wealth and goals

- Manual holdings can coexist with provider holdings in integrated custody.
  Provider rows stay read-only; sync never overwrites manual quantity, cost or
  valuations. Integrated custody cash stays provider-owned; a linked bank
  balance is counted only once. Manual operations are BRL-only, including in
  integrated custody; foreign-currency assets are refused for manual positions.
- Replay owns exact total cost/value. Average cost and unit price are display
  figures. Purchases capitalize fees/taxes and sales remove proportional cost;
  corrections reject negative cash/quantity. Selling out ends typed valuation,
  so repurchase restarts at cost.
- The latest price claim wins; typed valuation wins a same-day quote tie.
  Automatic quotes derive values on read and never write valuation operations.
  Quote origin controls replacement: sync can replace any origin, market only
  market, and issue price fills only an empty day. Provider attribution is
  separate from origin. History coverage tracks requested instrument ranges,
  with partial fallback history retaining only the served stretch.
- Principal reconciliation removes only the allocated part from reporting,
  preserving full bank cash. Manual income/fees/taxes report once, accounting
  for bank reconciliation. Monthly local contributions/withdrawals are gross,
  independent of links. Realized sale gain is informational, net of all-in
  cost and sale costs; appreciation and sale gain are not ledger income.
- Portfolio periods use one as-of methodology for manual/synced positions.
  Closed holdings remain in results. Modified Dietz rates divide summed yield
  by summed weighted capital, never averaged child percentages. Fees/taxes are
  counted once. Missing history/value/FX excludes flagged positions from sums;
  it never substitutes zero. Grouping/display filters do not change totals.
- Wealth is cash plus investments, excluding liabilities and future plans.
  History starts at the first cash transaction, reverses native cash and
  replays holdings, then values each day. Unavailable days are gaps; today's
  valuation failure is an error. Timeline drops ignored historical entries,
  while wealth reverses them; this intentional current difference is pinned.
- Goals measure only `invested_value` today, in BRL regardless of reference
  currency. Schema-admitted other metrics are rejected by the domain. Progress
  can exceed one; goals never add to totals. Future-date projections reuse
  trailing portfolio return and solve end-of-month contributions. Under 30
  days of capital/no measurable history/total loss leaves return unknown and
  assumes zero appreciation explicitly; projections are estimates.

Inventory: [investments](../../.specs/contextos/investimentos/reference.md),
[wealth](../../.specs/contextos/patrimonio-liquido/reference.md),
[goals](../../.specs/contextos/metas/reference.md).

## Ingestion and operational boundaries

- Provider identity is `(source_id, external_id)`; updates keep IDs and user
  decisions. An external ID reassigned across accounts/holdings is rejected.
  Changed records commit data/hash/decisions/events/counters together; failure
  leaves the old hash so a later sync can retry. Cross-origin similarities
  (Pluggy/manual/file) are never silently merged.
- Workers claim runs atomically, heartbeat every 15 seconds and verify
  ownership in writes. A 90-second expired lease fails an interrupted run;
  unclaimed runs fail after 15 minutes. Recovery does not retry automatically.
  Multiple processes require Postgres; polling serializes runs per instance.
- CSV adapters parse without persistence; preview writes nothing. Confirmation
  revalidates file hash, format/version and the compared-data fingerprint in
  one transaction. Identity includes account, normalized row, final balance
  and repeated-row ordinal. Explicit conflict choices survive replays; duplicate
  rows never advance the balance anchor. An ambiguous or potentially newer
  invalid row blocks balance advancement, while valid movements can still be
  imported by explicit partial choice. Gaps warn; no movements are invented.
- FX coverage is informative, not a guarantee of pair/date availability.
  Reporting distinguishes unsupported currency, missing historical data and
  provider failure. Native balances remain usable. Cached stale observations
  disclose provenance; failed lookups do not invent rates.
- Authentication is single-owner, separate from the encryption master key.
  Password reset revokes sessions without changing the key; a password cannot
  recover a lost key. HTTP access/security logs exclude financial payloads and
  secrets. MCP uses the same financial services and requires scoped, fresh
  owner approval for financial mutations.
- Home reads backend timeline figures. Its exact client daily allowance is
  the minimum over day k of `max(0, min(balance_k, cash_now)) / k`,
  floored to cents (zero with no points). This is a
  display metric, not a persisted budget. Chart minima and labels use exact
  comparisons; numeric conversion is confined to geometry.

Inventory: [sync](../../.specs/contextos/sincronizacao-open-banking/reference.md),
[statement import](../../.specs/contextos/importacao-extratos/reference.md),
[Home](../../.specs/contextos/home/reference.md),
[settings/authentication](../../.specs/contextos/configuracoes-autenticacao/reference.md).
See [self-hosting security](../self-hosting/security.md) and [MCP](../mcp.md)
for maintained operational guides.

## Migration boundaries

The old financial audit's shared link and calendar owners are now recorded
above and in the matrix; its regression examples remain in tests. The statement
import design contains superseded identity proposals and extension ideas; the
implemented context reference above is authoritative. The daily spending-cap
backlog is a proposal, distinct from today's Home allowance. Neither proposal
is promoted to an implemented reference.

The [schema inventory](../../.specs/reorganizacao-schema/inventario.md) and its
DDL snapshots retain historical conversion evidence; the
[migration reference](../../internal/db/migrations/README.md) owns the current
storage/conversion contract. Do not copy obsolete snapshot tables (such as
stored wealth observations) into new architecture rules. `.specs/` remains for
the separate cleanup and feature-documentation migration.
