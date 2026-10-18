# Investments

Investment accounts represent custody; positions represent assets. Targets
over the invested total live in the separate Goals area
([`metas`](../metas/reference.md)); positions carry no goal or grouping.

## Domain and APIs

`internal/investments` holds accounts, manual positions, operations,
valuations and reconciliation. APIs live under `/api/investment-accounts`,
`/api/investment-positions`,
`/api/investment-operations`, `/api/investment-reconciliations`,
`/api/investment-summary` and the [period view](#period-view)
`/api/investment-portfolio`. The legacy `/api/investments` queries remain
available for synced assets and their imported history.

Integrated investments keep the provider's identifiers, initially grouped by
connection. Reconciliations are local choices. A brokerage cash
balance already present in `financial_accounts` can be linked, without adding
its balance to net worth again.

An integrated account also accepts manual holdings (for example, a crypto
asset that Open Finance does not expose). They are created, edited and deleted
as in a manual account and listed next to the provider's positions, which stay
read-only. The ledger of an integrated account replays only operations that
carry a position and never computes cash, which remains whatever the
institution reports. The sync writes only provider data and series quotes, so
it never changes the quantity, cost, source or valuations of a manual
holding. The UI does not tag or split positions by origin; a synced position
is read-only and offers only **Ver detalhes**.

Manual operations use reais and decimal arithmetic. Purchases fold costs into
the weighted average cost; sales write off proportional cost. Corrections
reprocess history and reject negative cash or quantity. Valuations are manual
and dated; when unavailable, the cost is labeled as cost, without inventing a
quote or return. The ledger stores total cost and total valued amount; average
cost and unit quote are derived for display only, so value and gain suffer no
decimal drift. For assets with automatic quotes the value comes from the price
series (`market_quote`), computed on read: the quotes job never writes
operations to the ledger.

## Transfers and reports

Contribution and withdrawal transfer wealth. Purchase and sale swap investment
cash for a position or vice versa. Income received, fee and tax have their own
nature. An opening balance is not income; appreciation is not a contribution.

Reconciliations associate portions of bank entries with operations and, when
available, with the investment's imported history. The sum of the portions
cannot exceed the entry. Suggestions require confirmation; the provider's
`Investments` label is not enough to classify a contribution.

`transactions.Item.EffectiveMoney` keeps the full amount for the statement and
cash. `InvestmentTransferAmount` identifies the reconciled portion as a
contribution/withdrawal; `ReportableAmount` identifies the remainder eligible
for totals. A full reconciliation gets reason `investment_transfer` only if the
entry was still in totals. Ignored, transfer category and other reasons already
assigned take precedence and are kept. Spending by category uses the remainder;
the cash curve keeps the full movement. Monthly aggregates distinguish
contributions and withdrawals.

`investments.MonthlyMovements` derives monthly (`YYYY-MM` from `occurred_on`)
gross contributions (`Contributions`), withdrawals (`Withdrawals`), realized
income (`Income`, only income operations), fees (`Fees`), taxes (`Taxes`) and
realized gain (`RealizedGain`) from local operations (`source = manual`),
including in integrated accounts, optionally filtered by custody account. Fees
and taxes include costs from purchases and sales. These amounts do not subtract
reconciliations: linking a bank entry never reduces the month's gross
contribution or withdrawal. Imported movements and their reconciliation pivots
(`source = synced`) are excluded; creating or undoing those links does not
change local totals. Appreciation, purchase/sale principal and transfers
between custodians are excluded from the flows above. `GET
/api/investment-monthly-movements` (`?account_id=` optional) returns `items`
with `month`, `contributions`, `withdrawals`, `income`, `fees`, `taxes` and
`realized_gain`, plus `source: "manual"`, which says every row covers local
operations only; `GET /api/investment-operations` continues to return raw
operations.

`RealizedGain` (`realized_gain`; negative for a loss) is informative. For each
local sale it is the net proceeds, `amount − fees − taxes` (what the ledger
credits to the cash), minus the cost basis that sale wrote off, summed in the
month of the sale. The basis is the ledger's all-in one, read from the account
replay as the drop in the position's total cost: purchase and opening-balance
fees and taxes are capitalized in it, the cost frozen by a custody transfer is
carried to the destination, and a partial sale writes off a proportional share,
so selling a holding in pieces adds up exactly. This is the convention of the
position's yield (purchase at all-in cost, sale at its net value), and makes
`RealizedGain` already net of the purchase and sale fees and taxes. `Fees` and
`Taxes` keep itemizing those costs (the timeline books them as expenses, the
purchase ones on the purchase date) and must not be subtracted from it again.
It is not part of `Income`, is not in `ManualReportingEntries` and does not feed
the timeline totals.

It exists only for local operations (`source = manual`): they are the only ones
whose cost basis the ledger knows. Synced holdings have none and are not
covered (the response says `source: "manual"`), so a zero or a missing month
does not mean "no gain" for them.

For a manual position, the yield since inception (the Rendimento formula: value
at the end minus the net money put in, purchases at all-in cost and sales at net
value) is exactly its cumulative `RealizedGain` plus its unrealized gain
(`CurrentValue − TotalCost`), also for a holding that arrived by custody
transfer, whose frozen cost is both the money counted in and the basis a sale
writes off. Income operations belong to the account, not to a position, so they
are in neither the yield nor `RealizedGain`: they are `Income`.

Manual operations carry no currency and are in reais, so `MonthlyMovements`,
the gain included, sums a single currency. Manual custody accounts are BRL by
constraint, and the API refuses (400, `investment-unsupported-currency`) a
manual position whose asset is catalogued in another currency, in a manual or an
integrated custody, and a currency change on an asset held by manual positions.
Operations and transfers on a holding that predates the rule are refused the
same way; existing rows and reads are untouched. An integrated account stores no
currency (it is NULL), so its local cash records, which name no asset, cannot be
checked and are taken as reais. Multi-currency support for manual operations is
not provided: `MonthlyMovements` and `RealizedGain` assume one currency.

Appreciation and unrealized gain (`Summary.UnrealizedGain`) are never realized
income or realized gain: they are excluded from `ManualReportingEntries`,
`MonthlyMovements.Income`, `MonthlyMovements.RealizedGain` and timeline income
totals, although they change net worth. Reconciled contributions and
withdrawals preserve net worth; a fee reduces net worth by exactly its amount.

Editing or deleting a reconciled manual bank entry requires undoing the links
first. Ignoring a reconciled entry undoes its links (the operation is reported
on its own again), and an ignored entry cannot be reconciled. Reconciliation
does not change imported balances. Manual bank entries keep the rule of not
modifying the bank-reported balance.

## Net worth and limits

Net worth sums synced positions once, manual positions, and manual investment
cash not yet represented in a linked financial account. Manual holdings in an
integrated account count like any other manual holding (including in
`ManualNetWorth`), without double-counting the provider's positions. No appreciation is invented for historical
snapshots without data.

This version records operations; it does not place orders or move money at the
bank. It does not include tax computation, short positions, FX on manual
operations or scheduling of recurring contributions. Automatic quotes are
optional and described in [Automatic quotes](#automatic-quotes).

## Classification and asset catalog

Registration organizes an asset by **financial class** (`asset_class`) and
**instrument type** (`asset_type`), independent of institution or quote
service:

| Class | Available types |
| --- | --- |
| Fixed income (`fixed_income`) | Government bond, CDB, RDB, LCI, LCA, debenture, CRI, CRA, savings, fixed-income fund and ETF |
| Variable income (`variable_income`) | Stock, unit, stock BDR, FII, Fiagro, equity fund and ETF |
| Multimarket (`multimarket`) | Multimarket fund |
| Currency (`currency`) | Currency fund and foreign currency |
| Crypto assets (`crypto`) | Cryptocurrency, stablecoin, token and crypto ETF |
| Other (`other`) | COE, derivative, gold and commodity ETF |

PGBL and VGBL can be classified as fixed income, variable income or
multimarket depending on the plan's exposure. Every class allows describing
another type. Old or imported types are preserved; the migration classifies
recognizable descriptions and leaves funds and ETFs of unknown exposure in
Other for the user to review.

The API provides the options at `GET /api/investment-asset-classification`,
used by the form. Known types incompatible with the class are rejected.

The starter catalog is seeded automatically on every application startup, after
`db.Open` initializes and migrates the database and before workers or HTTP
serving start. Startup logs the number of new assets inserted (including zero);
a seed failure stops startup with an error. The manual command remains available:

```sh
go run ./cmd/julius seed investment-assets
# Uses JULIUS_DB; also accepts -db path-or-DSN after investment-assets.
```

The seed inserts 27 well-known assets: Brazilian stocks, a unit, FIIs, equity /
fixed-income / crypto ETFs, BDRs and BTC, ETH, SOL and USDC. It runs in one
transaction and is repeatable: restarts and catalog updates insert only missing
entries. It preserves existing assets, their names,
classes and quote preferences, including equivalent codes such as `PETR4.SA`.
The assets become available for selection in new positions even when there is
no position of them yet.

**New position** requires picking a catalog asset: the search matches ticker or
name ignoring case and accents, and the chosen asset is reused as-is (class,
type, currency and quote settings). A missing asset is registered in the same
drawer ("Novo ativo"), using the same fields as asset settings; choosing a
quotable type switches automatic quotes on with the matching market (for
example Crypto assets → Cryptocurrency uses `crypto`). If the typed code (or
name plus type, without a code) matches a catalog asset, that asset is selected
instead of creating a duplicate. Editing a position keeps the free-text fields. No accounts, balances, operations, transactions or prices are
created, and the seed makes no external calls.

CDBs and government bonds are available types; each issue/maturity must be
registered as its own instrument. The catalog is a registration convenience,
with no ranking or investment recommendation.

References: [investment characteristics (CVM)](https://www.gov.br/investidor/pt-br/investir/antes-de-investir/entenda-as-caracteristicas-dos-investimentos),
[fund classes (ANBIMA)](https://developers.anbima.com.br/en/documentacao/fundos/apis-de-fundos/),
[traded instruments (B3, May 2026)](https://www.b3.com.br/pt_br/noticias/levantamento-realizado-no-datawise-aponta-acoes-fundos-imobiliarios-etfs-e-bdrs-mais-negociados-em-maio.htm),
[ETFs with different exposures (B3)](https://b3.com.br/pt_br/noticias/b3-ultrapassa-marca-de-200-etfs-listados-e-estoque-financeiro-dobra-em-um-ano.htm),
[Ether](https://ethereum.org/developers/docs/intro-to-ether),
[Solana](https://solana.com/learn/introduction-to-solana-tokens) and
[USDC](https://www.circle.com/usdc).

## Automatic quotes

An asset with **automatic quotes** has the price of its manual positions fetched
automatically. The form offers an on/off switch and derives the **market** from
the instrument type, stored in `quote_source`: `b3` (stocks, units, FIIs, ETFs,
BDRs and the fractional market) or `crypto` (cryptocurrencies, quoted in
reais). `quote_symbol` is the instrument's canonical symbol in that market,
with no provider format. The user enters only the asset **code** (`ticker`) and
picks class/type; with an empty `quote_symbol`, the symbol is derived from the
code on save and stored on the asset. It only needs to be set when the quoted
code differs from the asset's code.

A fixed-income or crypto ETF traded on B3 uses `quote_source=b3` and keeps its
financial class. For types without supported quotes, the form directs the user to
manual valuations or the integrated institution's values.

- **B3:** `petr4`, `PETR4.SA` and `BVMF:PETR4` become `PETR4`.
- **Crypto:** `btc`, `BTC-USD` and `BTC/BRL` become `BTC`.

A code that cannot be read in the market's format is rejected on save (`400`),
with the reason.

Automatic refresh is enabled by default, daily at `19:00` in
`America/Sao_Paulo`. `enabled`, `time` and `timezone` are stored together in
`settings` and read by the scheduler without restarting the process. Changing
the time or zone reschedules the next run; disabling also suspends history
backfill. An active spot batch can finish; disabling is checked before the
history stage and subsequent work cycles.

At startup and when re-enabled, the scheduler fetches only missing prices for
today and missing history; a restart does not repeat requests already covered.
The daily run updates prices for today. Quote dates remain Brazilian calendar
days regardless of the scheduling timezone. Full scheduled runs and explicit
manual refreshes record finish time, trigger, status (`success`, `partial`,
`deferred` or `failed`) and counts in `quotes.last_refresh`; small spot refreshes
triggered by position saves are not recorded. A shared lock refuses overlapping
manual refreshes and serializes scheduled spot work. History backfill follows a
manual spot refresh in the background and is not part of its displayed outcome.
API changes wake the local scheduler immediately; writes by another process are detected within a minute.

`JULIUS_QUOTES_SCHEDULE` (`HH:MM` or `HH:MM Zone/IANA`) initializes only a
configuration that has not yet been saved. With no zone it uses `Local`; an
empty value uses the default schedule. Once saved, persisted settings take
precedence in full, including when disabled, and later environment changes are
ignored.

### Providers

`internal/marketdata` tries providers in priority order, with automatic
fallback. The order comes from `JULIUS_QUOTES_PROVIDERS` (comma-separated
names; the order is the priority and an omitted provider is off), by default
`yahoo,brapi,coingecko`. An unknown name prevents startup. Each provider serves
only the markets it knows, so by default:

| Market | Primary | Fallback |
| --- | --- | --- |
| `b3` | Yahoo Finance (`PETR4.SA`) | brapi |
| `crypto` | Yahoo Finance (`BTC-USD` × `BRL=X` rate) | CoinGecko (BRL pairs) |

Yahoo Finance is queried through the public chart endpoint
(`query1.finance.yahoo.com/v8/finance/chart`, the same one yfinance uses). Asset
not found, provider unavailable, timeout or rate limit cause the market's next
provider to be tried. Only when every provider of the market fails by rate limit
is the asset deferred: it is left for the next run and nothing is marked as
queried.

Each stored price (`investment_asset_quotes.source`) records the provider that
served it (`yahoo`, `brapi` or `coingecko`). The day's quote is cached in memory
for 10 minutes per instrument. On CoinGecko, the code (`BTC`) is resolved to the
coin id (`bitcoin`) by its search (the highest market-cap coin with that code
wins); this resolution stays in memory for the life of the process and is not
stored on the asset.

The brapi token (`PUT /api/settings/quotes`, `brapi_token`) is optional and only
matters when brapi is used as the B3 fallback.

Known Yahoo Finance limitations:

- There are no crypto pairs in reais (`BTC-BRL` returns `404`): the quote is the
  dollar price (`BTC-USD`) × the day's USD/BRL rate (`BRL=X`). On days without a
  rate (weekends), the previous rate applies.
- Prices come as floating point and are rounded to the places Yahoo itself
  indicates (`priceHint`).
- It is an undocumented public endpoint; when it fails, the fallback takes over.

### History

A position recorded with a past date needs prices on those dates. The backfill is
driven by what is missing, not by what changed: for each asset with automatic
quotes it compares the first day any manual position held it with the range
already queried (`investment_asset_quote_coverage`, one range per asset) and asks
providers only for the missing stretch, up to yesterday (today's price belongs to
the daily run). This covers a new position with an old date, a purchase
corrected to an earlier date, a market or symbol change, and the days of a missed
run. Saving a position, operation or asset wakes the scheduler (with a 2 s wait
to batch a burst of writes), and today's prices still missing are also fetched;
what is already covered triggers no request.

The range is keyed by the instrument (columns `source` = market and `symbol` =
canonical symbol), not by the provider: changing the provider order or falling
back does not refetch history; changing the asset's market or symbol does. The
stored range is what was **requested**, not what was obtained: non-trading days
are left without a price on purpose, and a stretch the provider does not serve
is not requested again on every run. The exception is history from a fallback
provider that served less than requested (for example, Yahoo down and a brapi
plan with only 3 months): only the served stretch is recorded, and the older
days are requested again on a later run, when the primary provider may be back.

| Provider | Markets (default role) | History | Rate limit |
| --- | --- | --- | --- |
| Yahoo Finance | `b3` and `crypto` (primary) | full, no period limit | no published number; the app uses 1 request per second, serialized, and honors `429`/`Retry-After` |
| brapi without token | `b3` (fallback) | only tickers PETR4, MGLU3, VALE3, ITUB4 | 20 requests per minute per IP (`RateLimit-*`); the app uses 1 every 3.2 s |
| brapi with token | `b3` (fallback) | per plan: 3 months (Free), 1 year (Startup), 10+ years (Pro) | monthly quota and per-account concurrency (1 on Free); the app serializes requests |
| CoinGecko (public API) | `crypto` (fallback) | up to 365 days back, 1 point per day (00:00 UTC) | per IP, no published number; the app uses 1 request every 6 s and honors `Retry-After` |

brapi only accepts relative periods (`range`), so the app asks for the smallest
that reaches the first day; if the plan refuses, it tries smaller ones. A past
day's price is the close adjusted for splits and reverse splits, never the one
adjusted for dividends: Yahoo's and brapi's `close` follow the same criterion.

Each provider has a limiter shared by the whole process (daily run, backfill and
CoinGecko code resolution). It spaces requests, serializes them and pauses the
provider when it asks (`429` with `Retry-After`, or a `RateLimit-Remaining: 0`
window until `RateLimit-Reset`). A long pause is not awaited with the request
open: the paused provider is skipped and the market's next one is tried.

### Price origin and precedence

Each row of the series (`investment_asset_quotes`, one price per asset per day)
has `source`, who wrote it (`pluggy`, `issue`, `yahoo`, `brapi`, `coingecko`),
for display and audit only, and `origin`, the kind of claim the price makes,
which decides who may replace whom:

| `origin` | What it is | Written by | Who may replace it |
| --- | --- | --- | --- |
| `sync` | price Open Finance reported during sync | `UpsertAssetQuote` (Pluggy snapshot, including the one rebuilt from `raw_imports`) | another sync |
| `issue` | issue unit price (PU) of a fixed-income bond, derived from the purchase | `insertAssetQuoteIfAbsent` | only a sync; never fills an already filled day |
| `market` | quote (spot or close) from a market provider | `UpsertConnectorQuotes` (daily run and backfill) | another `market` price, from any provider, or a sync |

The migration classifies existing rows by `source` (`pluggy` becomes `sync`,
`issue` becomes `issue`, the rest becomes `market`). Writers reject an empty or
unknown `origin` (`ErrInvalidInput`), and `UpsertConnectorQuotes` only accepts
`market`. The job considers an asset already quoted for the day
(`RefreshMissing`) only when there is a `market` price for today: the price the
sync reported does not prevent the market query, which in turn does not replace
it.

### Yield

`investment_asset_quotes` (the asset's daily price series) is the **only price
source**: Open Finance writes to it on every sync, the daily run of the quotes
job writes the day's price for assets with automatic quotes that some manual
position holds with positive quantity, and the backfill writes history. The job
**does not write `valuation` operations** to the ledger: the day's price only
enters the series.

The value of a manual position whose asset has automatic quotes is **derived on
read** (`Position.CurrentValue`, which feeds totals and net worth):
`quantity × latest series price`, rounded to cents, with
`valuation_basis = market_quote`, `current_unit_price` equal to the price and
`valued_on` equal to its day. None of this is persisted; an old quote still
applies to the displayed value, and `valued_on` shows its age.

The rule is "the most recent claim about the price wins; on a same-day tie, the
typed valuation wins": the quote only applies if the position has positive
quantity and there is no typed valuation, or the latest valuation is from a day
before the quote's. Otherwise the valuation applies (`manual_valuation`), and
keeps applying until a newer quote arrives. Assets without automatic quotes
remain valued only by the ledger's valuations (or by cost, with `cost_basis`).

Yield uses the same rule on any day: a manual position whose asset has automatic
quotes is valued at `quantity held at end of day × day's price` (latest price up
to 10 days earlier, rounded to cents like a synced position's), unless the last
typed valuation up to that day is equal to or newer than that price. So the
yield of any period within the history does not depend on a dated valuation
existing at the endpoints. The series stores nothing derived: the historical
value is always computed on read.

The 10-day cap applies only to yield. The position's current value uses the
latest quote of any age (`valued_on` shows its day), so the displayed value does
not jump on a provider outage. When the latest quote is older than 10 days and
newer than the last typed valuation, that day's yield is unavailable
(`saldo_indisponivel`): it does not fall back to a valuation the quote already
superseded.

## Period view

`GET /api/investment-portfolio?from=&to=&group_by=&source=&account_id=`
(`investments.BuildPortfolio`) is the backend authority for a period of the
book: positions as of `to`, their value, quantity and cost then, and what they
earned in the period. It reuses the yield machinery above (`PositionYield`
series, ledger replay, quotes, `valuation` conversion); manual and synced
positions follow the same formula, so the origin does not change the method.
The legacy `/api/investments` and `/yield` endpoints are untouched.

- **Period.** `from` and `to` (`YYYY-MM-DD`, `to` defaults to today; a future `to` is clamped to
  today and the response echoes the effective `to` with `to_clamped: true`; a
  `from` after the effective `to` is a 400) are both inside the period; the opening value is the one at
  the end of the day before `from`, and a flow dated `d` moves the position at
  the end of `d`. Without `from` the period is each position's life since
  inception (opening value zero). Per position, if the opening day has no price
  the period starts on the first priced day (`partial_start`, `opening_on`).
- **Which positions.** A position bought after `to` is left out; one open at
  `to` and closed since is reported with its as-of quantity and value. One
  closed at `to` (`closed_as_of_to`) stays in the numbers when it was worth
  something or moved in the period: its result is the realized gain. A position
  with no value and no movement in the period is omitted. Closed positions are
  never filtered out of totals.
- **Yield.** `yield = end_value − start_value − (contributions − withdrawals)`,
  and `end_value = start_value + contributions − withdrawals + yield` holds at
  position, group and total level. Contributions and withdrawals are the dated
  money flows of the position (a purchase at all-in cost, a sale net of its
  costs, a custody transfer at its frozen cost, a synced movement's amount); a
  dividend paid out by a synced holding is a withdrawal, hence yield. Manual
  `income` operations belong to the account, not to a position, and are not in
  it.
- **Held vs closed.** `totals` (portfolio and each group) carries `held`
  (positions with `closed_as_of_to` false: the yield of what is still held) and
  `closed` (true: the realized result of what is closed as of `to`), each with
  `start_value`, `end_value`, `contributions`, `withdrawals`, `yield`,
  `capital_base` and `yield_rate`. Each part reconciles and the two add up to
  the totals field by field (the total `capital_base` is their sum); each rate
  divides the part's own summed yield by its own summed weighted capital. The
  top-level totals are the consolidated result of the period.
- **Share.** Each available position has `share` (its end value over the
  portfolio's, null when unavailable), consistent with the group `share`s.
- **Percentage.** `yield_rate` is a ratio (0.05 = 5%): Modified Dietz,
  `yield / capital_base`, with `capital_base = V0 + Σ flow × (to − day) /
  (to − start)` in days. A group or the total divides its summed yield by its
  summed weighted capital (`capital_base`), never an average of child rates.
  Null when the base is not positive.
- **Invested.** `invested` is the cost of what is held at `to`: for a manual
  position the ledger's cost basis at `to`; for a synced one the net money put in
  up to `to` (zero once closed), available only when the history is complete
  (a first inflow is known, or a fixed income purchase is restored). Otherwise
  it is null with `invested_unavailable_reason`. Group `invested` sums the known
  ones; `invested_unknown_positions` counts the others.
- **Unavailable.** A position whose period cannot be stated has
  `unavailable_reason` (`sem_historico`, `historico_incompleto`,
  `saldo_indisponivel`, or `cambio_indisponivel` for a currency without a rate)
  and null figures. It is counted (`positions_total`, `positions_unavailable`,
  `partial: true`) and excluded from every sum and from the percentage, never
  counted as zero. With an explicit `from`, a synced position is computable
  even when its purchase predates the provider's history, as long as quotes
  price both ends (`history_complete: false`, `invested` null).
  Its `closed_as_of_to` is still set when the quantity is known to be zero, so
  the closed filter can hide it; it stays out of every sum either way. An
  unknown quantity says nothing and leaves it listed.
- **Costs and taxes.** Counted once: a manual position's purchase fees and
  taxes are inside its contributions and a sale's are netted from its
  withdrawals, so `costs` (the fees and taxes of the period's manual operations)
  is informational. The yield of a synced position is gross of the IR/IOF the
  provider provisions; `provisioned_taxes` (`taxes + taxes2`, only when the
  snapshot is not after `to`) is informational and not deducted. The provider's
  own `amount_profit` is `reported_profit` (native currency, with
  `reported_as_of`), for conference only.
- **Currency.** Every figure is converted to BRL with one rate per currency
  (the day `to`), the same for opening value, closing value and each flow, so
  the identity holds in BRL and the yield excludes exchange variation inside the
  period. `native` keeps the position's own amounts; `native_rates` lists the
  rates used. Money is then expressed in the reference currency like
  `/api/investment-summary` (`currency_code`, `fx`).
- **Scope.** `source` (`manual`|`synced`) is the only filter that changes what
  is measured; it is echoed. `group_by` (`none`, `account`, `class`) is
  subtotaled server-side: each group has its own totals and `share` of the
  portfolio's end value; the top-level `totals` do not depend on it, nor on
  `account_id`, which only narrows the listed groups. Cash is not part of any
  figure. `asset_class` comes from the asset catalog (`unclassified` when a
  position has no asset). Unavailable positions have no end value and so no part
  of `share`.

Response (`group_by=none` yields one group with key `all`):

```json
{
  "from": "2026-09-10", "to": "2026-09-28", "to_clamped": false, "group_by": "account",
  "source": null, "account_id": null, "currency_code": "BRL", "fx": {},
  "native_rates": {},
  "totals": {
    "held": {}, "closed": {},
    "start_value": "1050", "end_value": "600", "contributions": "0",
    "withdrawals": "600", "yield": "150", "capital_base": "955.2631578947",
    "yield_rate": "0.1570247934", "invested": "500",
    "invested_unknown_positions": 0, "costs": "0", "provisioned_taxes": "0",
    "positions_total": 1, "positions_unavailable": 0,
    "positions_partial_start": 0, "partial": false
  },
  "groups": [{
    "key": "<account id | asset class | all>", "label": "Corretora",
    "account_id": "…", "asset_class": null, "share": "1", "totals": {},
    "positions": [{
      "id": "…", "source": "manual", "account_id": "…", "account_name": "…",
      "asset_id": "…", "name": "…", "ticker": null, "asset_type": "Ação",
      "asset_class": "variable_income", "currency_code": "BRL",
      "quantity": "5", "closed_as_of_to": false, "unavailable_reason": null,
      "opening_on": "2026-09-09", "partial_start": false, "history_complete": true,
      "start_value": "1050", "end_value": "600", "contributions": "0",
      "withdrawals": "600", "yield": "150", "capital_base": "…", "yield_rate": "…", "share": "1",
      "native": {"start_value": "…", "end_value": "…", "contributions": "…",
                 "withdrawals": "…", "yield": "…", "invested": "500"},
      "invested": "500", "invested_unavailable_reason": null,
      "costs": "0", "provisioned_taxes": null,
      "reported_profit": null, "reported_as_of": null
    }]
  }]
}
```

Start, end, contributions, withdrawals, yield, capital base, `native`,
`opening_on` and `yield_rate` are null on an unavailable position; the shared
fixture (`investment_period`) pins the shape.

### Page

`/investimentos` (`pages/InvestmentsPage.tsx`) is a single view; there is no
manual/synced split in the UI (the legacy `/api/investments` listing is no
longer shown here, only its detail pages, reached from a position's name or
**Ver detalhes**). The shared `usePeriod()` window is mounted through
`PeriodNavigator` in the page `context`, like Home and Transações.
`presentation/investmentPeriod.ts#periodRequest` turns it into the request:
"Todo o período" omits `from` (and ends today); a window that starts after today sends
nothing and says "Este período ainda não começou". An end in the future is
sent as is: the server cuts it at today and answers `to_clamped`, which the
summary states ("Período em andamento"). The window moves the summary and the
positions together. The page tabs (`PageTabs`) choose how
the positions list is grouped: **Classe de ativo** (default), **Custódia** or
**Sem agrupamento**.

Components (`components/investments/`) and hook:

- `useInvestmentPortfolioPeriod` (`hooks/`) reads the endpoint with
  `group_by` (the UI never sends `source`); the previous answer stays while a new one loads
  (`aria-busy`, dimmed rows). Its query key lives under the `investmentWorkspace`
  root, so every ledger or asset write refreshes it.
- `InvestmentPeriodView` owns the loading/error states and the list controls:
  search and **Mostrar encerradas** shape the rows, the tab's grouping is sent to
  the backend. The section header states how many positions the server measured.
- `InvestmentPeriodSummary` shows **Carteira em `to`** (`totals.end_value`),
  Rendimento (`totals.yield` together with `yield_rate`), Aportes, Resgates and
  Custos (only when non-zero). The box carries no explanatory prose: the caveats
  (cash excluded, IR/IOF provisioned and not deducted, period in progress,
  positions that start late via `positions_partial_start`) sit in the popover of
  an ⓘ icon, whose accessible name includes them. The disclosure **Detalhes do
  cálculo** expands the five backend components and, under them, where the
  result comes from: "Posições mantidas" (`totals.held`) plus
  "Encerradas no período" (`totals.closed`, the realized result; hidden when
  all zeros). When `partial`, a tag reads "Parcial · N de M posições" and its
  popover gives the reasons by count. A null `yield_rate` is "—" with its
  reason, never 0%.
- `InvestmentPositionsPeriod` renders the groups: a table from `xl` up (one
  `<tbody>` per group, the group's subtotal as its first row), stacked rows and
  collapsible headers below it. Groups start **collapsed**: the header carries the
  subtotal and `share` (the figure with a thin bar, so concentration by class or
  custody reads at a glance), and a toggle expands it to the rows (searching keeps
  matching groups open). Columns: Ativo (ticker and quantity beneath), Classe (with
  the instrument type beneath) and Custódia (each hidden when it is the grouping),
  Valor atual (`end_value`), Valor investido (`invested`), Rendimento (R$ and
  `yield_rate`) and Participação (the position's `share`, "—" when the backend has
  none). On compact screens, Investido and Participação are
  labelled facts under the asset instead of one dense sentence. Group toggles
  are buttons with `aria-expanded`.
- `InvestmentAccountsDrawer`, opened by the **Contas** button in the page header,
  keeps account management reachable whatever the grouping: the list of accounts
  with each one's current cash (**Disponível hoje**) and menu (Ver movimentações,
  Editar/Excluir conta or Vincular caixa), and, per account, its movement history
  (correct/delete). Creating (movimentação, posição, conta) is only in the page's
  **Adicionar** menu; account menus hold management only. **Ver
  movimentações** in an account group's or a manual position's menu opens the same
  drawer on that account, or filtered to that position. Account groups and
  position rows carry the same menus (`useInvestmentMenus`); a synced position
  offers **Ver detalhes**, whose page lists its movements.

Display rules:

- Group money and percentages are the server's. Search (name or ticker,
  accent-insensitive) and **Mostrar encerradas** are client-side row filters
  only: totals and subtotals stay those of the full group, and the UI says so
  ("A busca só esconde linhas…", "Totais e subtotais incluem N encerradas
  ocultas"). A hidden closed position with no figures is
  not in any total, so it gets its own note ("N encerrada oculta está sem valor
  no período e fora dos totais") rather than being counted among those. `source` stays a backend filter but
  the page does not use it.
- The main figures are the consolidated period result (`totals`), which
  includes positions closed as of `to`; the held/closed split comes from the
  server and the client never sums it.
- A position the backend could not measure shows "—" and a badge with the
  reason (Sem histórico, Histórico incompleto, Sem saldo, Sem câmbio); an
  unknown `invested` is "—" with `invested_unavailable_reason`. Other badges:
  Encerrada, Início parcial (`partial_start`), Histórico incompleto
  (`history_complete = false`). A foreign position also shows its native value.
- `reported_profit` (with `reported_as_of`), the period components, costs and
  the provisioned IR/IOF are in the popover of the position's Rendimento, as
  "Informado pela instituição" for conference; they never feed a total.
- Cash (saldo para investir) is not part of the period figures; each account's
  cash in the accounts drawer is today's balance.

## Combined actions, review and filters

`POST /api/investment-operations/batch` takes `operations` (1 to 100 items) in
the same shape as a single operation, and an optional `reconciliation`. The link
refers to the first operation of the batch. The whole write is transactional: an
invalid purchase or a rejected link leaves no orphan contribution or income. The
UI offers contribution with purchase and reinvested income. Quantity and price
compute the gross amount in decimal when `amount` is not given.

Integrated accounts accept local records of contribution, withdrawal, income,
fee and tax, without a position. These records feed reports and reconciliation;
they never recompute the provider's balances or positions. Buys, sells,
opening balances with a position and manual quotes apply only to the manual
holdings of the account itself; buys and sells do not change the institution's
cash. A cash-only opening balance is rejected, and transfers between
custodies stay restricted to manual accounts (`ErrIntegratedReadOnly`). Buys,
sells, balances and quotes of the provider's positions remain institution data.

A bank entry can also be reconciled directly with a movement imported from an
integrated account, with no manual record: `POST /api/investment-reconciliations`
without `operation_id` derives an operation `source = synced` in the
connection's custody (contribution for `inflow`, income for `INTEREST`,
withdrawal for the other `outflow`), with the movement's amount and date. The
derived operation is read-only, is reused by later portions of the same
movement (even after the portion that registered the movement is undone),
disappears when the last link is undone, and never enters reports on its own. A
movement without direction, without date or outside BRL is rejected.

A transfer between custodies carries the average cost the source position had on
the transfer date; that cost is stored on the destination entry and later
corrections to the source purchases do not propagate it. A manual position may
start empty to receive its first purchase. The last manual unit quote stays
valid after purchases and sales; the remaining quantity is valued by it, keeping
its date. Corrections reprocess history in date, creation and identifier order.

Reconciliation checks direction, BRL currency and the remaining capacity of each
entry, operation and imported movement. A bank movement linked to an account
transfer (source, destination or fee) cannot be reconciled until that transfer
is unlinked. Both workflows lock the bank movement before checking links.
For integrated accounts, the imported
movement must belong to the same connection. Concurrent portions are serialized
during validation. UI suggestions use equality of the available amount and up to
five days of difference; they require confirmation. Fees, taxes and income can
also be linked, avoiding duplicating revenue and expenses. Unlinking restores
the available portion.

`POST /api/transactions/query` accepts `filters.origin` (`manual`, `synced`).
No history is reclassified automatically.

Positions accept the filters `account_id`, `source`,
`source_id` and `include_closed`. Operations accept `account_id`, `position_id`,
`source`, `source_id`, `from`, `to` and `reconciliation_state`
(`linked`, `unlinked`). Typed valuations are dated `valuation` operations,
queryable through the same operation history; the quotes job never creates them.

The investments area's total includes the effective cash of linked accounts. In
net worth that same cash is already in the bank portion, so it is not summed a
second time.

Imported positions in other currencies keep their original currency and value;
the summary, net worth and the period view convert them with the day's rate
(see [Reference valuation](#reference-valuation)).

## Database representation

Exact amounts, prices and quantities use Postgres `NUMERIC` and SQLite decimal
text. Issue/purchase/due dates are calendar dates (`DATE` in Postgres, date text
in SQLite), normalized on the provider's BRT wall clock; quote identity retains
its existing provider-day semantics. Trade dates retain the existing movement-day
rule (midnight UTC is already a day, other instants use BRT). Observation/as-of
values remain instants. Investment due dates and imported trade dates use
calendar-date adapters in presentation and reconciliation reads, accepting SQLite
date-only text as well as legacy timestamps. Integrated custody IDs remain opaque
`integrated:<source id>` text; records with UUID identities use native Postgres
UUIDs. `db.Open` rejects the old schema generation; conversion is separate work.

Position responses expose nullable `total_cost`, the exact acquisition cost
from the backend ledger (`average_cost` is a display value). The page no longer
derives a gain from it: yield, invested amount and percentage come from the
[period view](#period-view), which was the migration of the former browser-side
aggregates.

## API contract verification

Wire semantics and calculation ownership are inventoried in
[`contratos-api.md`](../../contratos-api.md). Shared synthetic responses are
checked against Go DTOs and the production TypeScript parsers by the existing
backend/frontend CI tests.

## Monetary precision

Investment cash fields keep decimal strings through editing
and submission. Period figures arrive as decimal strings and ratios and are only
formatted in the browser; BRL rounding happens only at display
([policy](../../precisao-monetaria.md)).

## Reference valuation

`GET /api/investment-summary` converts its existing BRL base into
`finance.reference_currency` using current `internal/exchange` FX. Totals,
per-account value/cash share that rate and carry the chosen `currency_code`
plus full `fx` provenance. Missing rates make the summary unavailable rather than
showing zero or an identity conversion. Stale stored rates are disclosed.

Positions and operations, including their currencies and amounts, stay native.
Synced foreign positions and foreign custody cash are valued in BRL with the
day's `internal/exchange` rate (`investments.BuildSummary`, `ManualNetWorth`); a
currency without a rate fails the summary rather than being omitted. The period
view converts every position with one rate per currency (day `to`) in the
backend and shows the native value of a foreign position beside it. No
positions, targets or provider balances are
rewritten when switching reference currency.
