# Financial API contracts

Living inventory of the backend/frontend boundary. Wire fields are defined by
`internal/httpapi` DTOs and validated by `frontend/src/api/contracts/` before
consumers see them. The shared synthetic fixture
`internal/httpapi/testdata/financial-contracts.json` pins representative JSON
from those DTOs; it is not production data or an API schema generator.

## Common semantics

- Money, quantities, unit prices and ratios are JSON **strings**, never JSON
  numbers. The response grammar is `-?(0|[1-9][0-9]*)(\.[0-9]+)?`: no exponent,
  comma, whitespace, leading plus, NaN or infinity. Go uses exact decimal;
  `money.CanonicalDecimal` preserves scale, emits fixed point and removes the
  sign of zero. There is no global two-decimal precision cap. Values above
  JavaScript's safe integer range and below one cent remain exact at the wire
  boundary. Formatting to cents must not feed back into the ledger.
- An amount without its currency is not a money pair. `null` means unknown or
  inapplicable, not zero, BRL or an empty string. Currency totals stay separated;
  BRL reports exclude foreign money without an FX source.
- Instants use RFC3339/RFC3339Nano with an offset, normally `Z`. Preserve the
  original string; JavaScript Date loses sub-millisecond precision. Calendar
  days use `YYYY-MM-DD`, without a timezone conversion. Account/bill dates and
  legacy investment date fields currently serialize Go `time.Time` as RFC3339
  (calendar dates there are midnight instants); workspace investment days and
  timeline days are date-only strings. Do not unify those representations
  implicitly. Frontend `isValidDate` currently accepts more than RFC3339 and
  `dateOnlyPattern` checks shape rather than calendar validity; server writes
  validate real dates.
- Nullable response fields are present with JSON `null`, unless explicitly
  optional below. Lists are arrays (`[]` when empty), not null. IDs are UUIDs
  except opaque provider `external_id` values and custody account IDs
  `integrated:<data-source UUID>`.
- Closed application enums are case-sensitive and runtime-validated. Provider
  vocabulary (`provider_status`, movement/asset types, subtype, rate type) is
  open text, nullable where the provider can omit it. A new provider status
  must not become a new frontend business rule.
- Under `/api`, responses are `no-store`; writes use the shared transport's
  origin/CSRF headers. Problems are parsed through the existing Problem contract.

## Critical read inventory

For exact field names/nullability, follow the DTO and parser columns; the
fixture verifies their serialized agreement. Domain details remain in the
linked context references.

| API | Go DTO / conversion | TypeScript parser | Money / dates / nullable fields |
| --- | --- | --- | --- |
| `GET /api/accounts`, `/api/accounts/{id}` | `accounts_handlers.go`: `accountDTO` | `accounts.ts`: `parseAccount(List)` | Nullable balance, opening balance, credit/available limits and credit usage ratio; nullable identity/display/provider metadata. Close/due dates and provider/local updates are nullable instants; archived_at is a nullable instant string. Closing day and its source are both null or both present. |
| `GET /api/accounts/{id}/cards`, `/bills` | `accountCardDTO`, `accountBillDTO` | `parseAccountCardList`, `parseAccountBillList` | Card count is an integer; last movement is a nullable instant. Bill total/minimum payment, currency and close/due instants are nullable. |
| `POST /api/transactions/query` | `transactions_handlers.go`: `toResultDTO`, `toItemDTO` | `transactions.ts`: `parseTransactionQueryResult` | Nullable original/account-currency amounts and effective money; allocation amount is non-null, reportable amount nullable. Occurred/category/inclusion/confirmation times are instants (occurred/inclusion may be null). Group boundaries are nullable days. Pagination/counts are integers. |
| `GET /api/investments`, `/api/investments/{id}` | `investments_handlers.go`: `investmentDTO` | `investments.ts`: `parseInvestment(List)` | Provider amounts, balance, quantity, price, rates, taxes and yield are nullable decimals; due/as-of/provider-update instants nullable. Yield source is null iff yield value is null; unavailability reason explains absence. |
| `GET /api/investment-accounts` | `investment_portfolio_handlers.go`: `investmentAccountToDTO` | `investmentWorkspace.ts`: `parseInvestmentAccountList` | Non-null BRL cash balance, active boolean and creation/update instants; nullable source/display/linked bank IDs. Envelope is `{items: [...]}`. |
| `GET /api/investment-positions` | `investmentPositionToDTO` | `parseInvestmentPositionList` | Non-null quantity/current value; nullable average cost, exact total cost, current unit price and valued-on day. Asset/ticker/provider-link/notes nullable. Source/valuation basis are backend decisions. |
| `GET /api/investment-operations` | `investmentOperationToDTO` | `parseInvestmentOperationList` | Non-null amount/fees/taxes; nullable quantity/unit price, position/transfer/notes. Occurred-on is a day; creation/update are instants. Source and is_editable are authoritative. |
| `GET /api/investment-summary` | `investmentSummaryToDTO` | `parseInvestmentSummary` | Values/cash/unrealized gain are non-null decimals. Summary contains per-account aggregates. |
| `GET /api/investment-portfolio` | `investment_period_handlers.go`: `investmentPeriodDTO` (`investmentPeriodToDTO`) | `investmentPeriod.ts`: `parseInvestmentPeriod` (strict keys, enums, unavailable-position shape); fixture key `investment_period` | Totals and per-position start/end value, contributions, withdrawals, yield, capital base, costs, provisioned taxes are decimals in the reference currency (positions: null when `unavailable_reason` is set); totals carry `held`/`closed` parts (same money fields plus nullable `yield_rate`); `yield_rate`, `share` (group and position) are ratios, nullable; `to` is the effective day and `to_clamped` says a future `to` was reduced to today; `native` and `reported_profit` stay in the position's currency; `quantity`, `invested`, `costs`, `provisioned_taxes`, `reported_*` nullable (invested says why in `invested_unavailable_reason`). `from`, `source`, `account_id` echo the request and are null when omitted; `opening_on`, `to`, `reported_as_of` and `native_rates[].effective_on` are days. Groups are `[]` when empty; `partial` is true iff `positions_unavailable > 0`. Semantics: [investimentos](contextos/investimentos/reference.md#period-view). |
| `GET /api/goals` (also `POST`, `PUT`/`DELETE /api/goals/{id}`) | `goals_handlers.go`: `goalDTO` | `goals.ts`: `parseGoal(List)` | Target, current value and progress are non-null decimals (progress is a ratio and may exceed 1); target date a nullable day, notes nullable. `projection` is null or an object of decimal strings (`monthly_return`, `return_from` null without history; `months_remaining` ≥ 1). Always BRL (`currency_code`), never converted to the reference currency. Envelope is `{items: [...]}`. |
| `GET /api/investment-positions/{id}/yield` | `investment_yield_handlers.go`: `investmentYieldDTO` | No frontend consumer/parser today | Value/start/end/net flows and actual from day nullable on unavailable yield; to is a day, partial boolean. Optional days (omitted when no daily detail) contain exact decimal value/end value/net flows. |
| `GET /api/timeline` | `timeline_handlers.go`: `timelineResponseDTO` and converters | `timeline.ts`: `parseTimelineResponse` | Balance, entry/reportable amounts, day flows, period totals and scenario deltas are non-null decimals. Entry/point/first-negative dates are days; category/scenario IDs and first_negative nullable. Simulation is null unless explicitly selected; impacts default to []. |
| `GET /api/transactions/category-breakdown` | `transactions_handlers.go`: category spending DTOs | `spendingByCategory.ts` | Total/item amount are decimal strings; category_id nullable for uncategorized; date filters are nullable days. Backend selects eligibility and groups spend. |
| `GET /api/net-worth` | `networth_handlers.go`: breakdown/point/response DTOs | `netWorth.ts`: `parseNetWorthSeries` | Cash, investments and total are all-or-none decimals: absent (never zero) only when `valuation_status` (`complete`/`stale`/`unavailable`) is `unavailable`; `date` an instant at the day's midnight UTC; optional `valuation`/`fx` travel with each point. Wealth is cash + investments only (no liabilities) and every day is derived on read; latest and series come from the backend. |

## States and enums

| Contract | Values / meaning |
| --- | --- |
| Accounts | origin `manual` / `synced`; account_type `BANK` / `CREDIT` / null. Closing source `manual` / `informado` / `estimado` / null; day is integer 1–31. Credit usage is balance / positive limit, rounded to four decimals, not clamped. |
| Transactions | classification `inflow` / `outflow` / `unclassified`; origin `manual` / `synced`. Inclusion `considered` / `ignored` is the user's decision, separate from provider status; inclusion origin `manual` / `rule`. Category kind `income` / `expense` / `transfer`, origin `manual` / `rule` / `learned`. |
| Provider status and eligibility | `PENDING` **and** `POSTED` may count in reports; pending does not mean excluded. Other statuses and null fail status eligibility. Manual-bank balance separately includes posted movements only. `totals_eligibility.included` is true iff reason is null. Reasons: `ignored`, `unclassified`, `ineligible_status`, `missing_money_pair`, `zero_value`, `transfer_category`, `investment_transfer`. Ignored always reports ignored. Backend precedence and allocation decide the answer; the frontend must not rerun eligibility. |
| Money source / grouping | effective_money source `account_currency` / `transaction_currency`; account-currency pair wins when complete. Grouping `none` / `day` / `week` / `month` / `year`, with `undated` for a group's kind. |
| Investment custody / positions | backend custody kind `manual` / `integrated` (frontend also accepts historical `synced`); position source `manual` / `synced`; valuation basis `cost_basis` / `manual_valuation` / `provider_balance` / `market_quote`. Cost basis is not evidence of market appreciation. |
| Investment operations | kind `initial_balance`, `deposit`, `withdrawal`, `buy`, `sell`, `income`, `fee`, `tax`, `valuation`, `transfer_out`, `transfer_in`; source `manual` / `synced`. Read-only behavior follows is_editable rather than a frontend inference. |
| Investment yield | source `informado` / `calculado` / null; unavailable reasons `sem_historico`, `historico_incompleto`, `saldo_indisponivel`. Unknown return must not be presented as zero profit. |
| Timeline | tier `realizado` / `confirmado` / `projetado` / `hipotetico`; source `real` / `investment` / `recorrente` / `plano_pagamento` / `cenario`. Hypotheticals require explicit scenario_ids. Cash amount and reportable amount are different readings; transfer cash must not become income/expense. |

## Writes and compatibility

Clients send decimal strings. Manual transaction writes take a signed amount
and occurred_at day. Investment operation writes take an occurred_on day;
amount/quantity/unit_price/fees/taxes use exact decimals. Existing Go request
decoders also accept literal JSON numbers without an intermediate float;
`investmentDecimal` treats absent/null/empty text as not informed. That leniency
is not the response format. `investmentText` distinguishes an omitted update
(keep current value) from explicit null (clear link/note).

Runtime parsers reject extra keys. Historically optional response fields are
transaction `source_provider`, `investment_transfer_amount`, `reportable_amount`,
timeline entry `reportable_amount`, position `total_cost`; the backend-only yield endpoint also omits `days` when not requested.
Transaction allocation defaults to zero and absent reportable amounts use
effective_money; timeline absent reportable amounts use amount. Current Go
responses always carry allocation/reportable fields. Missing total_cost stays
unknown and never falls back to multiplying the displayed average cost.

`total_cost` is now exposed from the existing exact ledger value; no persisted
schema or financial rule changes. Deploy backend and the bundled frontend
together: an older frontend's strict position parser rejects this extra key.
The new frontend reads older responses, but manual yield without total_cost
shows “Sem custo de aquisição registrado.”. Any future name/type/nullability,
closed enum or response-field change needs the shared fixture, both parsers and
this inventory reviewed together; additive keys can also affect older clients.

## Calculation ownership and frontend inventory

| Consumer | Authoritative input / local arithmetic |
| --- | --- |
| Transactions / category spending | Backend classification, effective money, eligibility, reportable amount and filtered/group totals. Never sum the displayed page to rebuild totals or infer eligibility from provider_status. |
| Home period / projection | Backend period_totals, points, starting/lowest balance. Existing daily allowance (`presentation/dailyAllowance.ts`) is an explicit client metric over backend daily balances; formula/limits are documented in [Home](contextos/home/reference.md). It is not a persisted budget or ledger total. |
| Patrimônio | Backend cash, investments and total. Chart conversion to Number is visual scaling only. |
| Goals | Backend current_value, progress and projection; the page never recomputes them from other endpoints. Percentages use Number for display only. |
| Investment book | Backend positions, summary values, effective cash, source/valuation_basis and yield endpoints. Exact manual acquisition cost is total_cost; average_cost and unit price are display values and must not reconstruct cost/value. |
| Investment period view | Backend `GET /api/investment-portfolio`: positions as of `to`, start/end value, contributions, withdrawals, yield, Dietz rate, invested, subtotals by account/class and their shares, all computed server-side. Clients display; they never re-sum, re-weigh percentages or reconcile. Unavailable positions arrive flagged and excluded from sums. |
| Investment page (`InvestmentsPage`) | Everything it states about a period (patrimônio, rendimento, rentabilidade, aportes, resgates, custos, invested, subtotals, shares) is the `GET /api/investment-portfolio` response; the client keeps no yield, cost or movement arithmetic. Search and the closed switch only choose which rows are listed; grouping and origin are request parameters. Percentages are written from the backend ratio string on decimal text (`presentation/investmentPeriod.ts`). `to` is sent as chosen: the server cuts a future end at today (`to_clamped`); the only request-side rule is sending nothing for a window that starts after today. The held/closed split of the result and each position's share are server figures too. The accounts drawer shows each account's current `cash_balance` from the workspace, not an as-of value. |
| Payables / input previews | Remaining/settled/status come from the backend. PayablesPage sums visible remaining values; form previews and allocation suggestions assist entry, while server writes revalidate capacity, currency and direction. Presentation money helpers must not become ledger rules. |

The duplicated acquisition-cost calculation has been removed. Other local
metrics above are inventoried rather than silently changing their scope or
substituting historical yield for the existing workspace net-return display.
Future consumers should use backend totals; migrating these existing metrics
requires agreeing on their coverage, fees and unknown-state semantics first.

## Automated verification

`TestFinancialContracts` serializes actual Go response DTOs/converters and
compares JSON against the checked-in shared fixture. Go field/tag/type changes
fail until explicitly reviewed. `financialContracts.test.ts` parses that same
file through production runtime parsers and checks lossless equality, required
nulls, enum rejection, date representations and pending/posted states.
`TestFinancialContractMoneyRoundTrip` and its TypeScript counterpart exercise
large, tiny, signed and scaled amounts. Type assertions also pin critical
TypeScript monetary/nullability/classification types.

Both tests run through CI's existing `go test -p 1 -count=1 ./...` and
`npm test -- --run` steps, with frontend typecheck. No generator/dependency or
additional CI service is needed. Coverage is representative, not an exhaustive
schema for every route: category breakdown keeps its existing focused tests; the
yield endpoint has Go handler/domain tests but no frontend consumer. These
are listed above for inventory completeness.
Update instructions are in [frontend testing](../frontend/TESTING.md).
