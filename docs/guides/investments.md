# Investments and market prices

[All documentation](../README.md) · [Financial planning](financial-planning.md)

## Add and maintain holdings

1. Open Investments (`/investimentos`). Connected institutions supply their
   positions through [Pluggy sync](accounts.md#connect-an-institution).
   For local tracking, use **Add** to create an investment account and position.
2. Select a catalog asset by ticker/name, or register a missing asset in the
   position form with its class, type and currency. Enter any opening quantity,
   cost/value and date, or start empty and record a purchase later.
3. Use **Add → Movement** to record contributions, purchases, sales,
   withdrawals, income, fees or taxes. Use the account/position movement history
   to correct or delete local records.

Manual operations support BRL assets and amounts only, including manual holdings
inside connected custody. Provider positions stay read-only; open their details
to inspect imported movements. Local holdings in connected custody do not change
institution-reported cash or positions. In manual custody, purchases consume
investment cash and sales replenish it; corrections cannot leave negative cash
or quantity. Record funding first, or use the combined contribution-and-purchase
option. Reinvested income also has a combined option.

Julius records activity; it does not place orders, transfer bank money, calculate
tax liabilities or schedule recurring contributions.

## Value a position

For an asset without automatic quotes, record a dated valuation movement with
the position's total value. Without a valuation, Julius labels its acquisition
cost as cost rather than inventing a market return.

For supported assets, enable automatic quotes on the asset in Settings →
Investment assets or when registering it. The market follows instrument type:
B3-listed stocks, units, FIIs, ETFs, BDRs and fractional tickers use B3; supported
crypto tickers use the crypto market. Prices are in BRL. Other exchanges and
fixed-income issue pricing are not automatic market quote sources in this build;
use manual valuations or institution data for unsupported instruments.

A newer market quote supersedes a typed valuation; a typed valuation wins on the
same day. Check the valuation date: the latest current quote can remain displayed
during an outage, even when too old to calculate a period return.

## Schedule or request prices

In Settings → Investment assets, find **Automatic quotes**, enable daily updates,
choose the time and timezone, then save. The default is enabled at 19:00 in
America/Sao_Paulo; saved changes take effect without restarting Julius.
The server must be running. Quote dates remain Brazilian calendar days even if
you choose another scheduling timezone.

Startup/re-enabling fills missing prices for today and missing history; covered
requests are not repeated just because the server restarted. Saving backdated
positions or correcting operations wakes that work while automatic updates are
enabled. Disabling suspends scheduled work and automatic history backfill; an
active price batch may finish.

Use **Update now** for an explicit refresh, even with scheduling disabled.
Review **Last update** for success, partial failure or assets deferred by rate
limits; retry later when deferred. Its result describes current-price work,
with history requested separately in the background. Concurrent full refreshes
are refused. Repeated requests may reuse a ten-minute current-price cache.

## Understand providers and historical coverage

| Market | Default primary | Default fallback |
| --- | --- | --- |
| B3 instruments | Yahoo Finance | brapi |
| Crypto assets | Yahoo Finance, USD price converted to BRL | CoinGecko, BRL prices |

Only providers supporting the chosen market are tried. Not found, outages,
timeouts and rate limits trigger fallback. Julius respects provider throttling;
when no provider succeeds and rate limiting is involved, the asset is deferred.
Existing prices remain. The optional brapi token is saved in the advanced quote
settings; without it only a restricted ticker set is available. The server
operator can change provider priority with `JULIUS_QUOTES_PROVIDERS`.

History fills missing ranges from the first manual holding date through yesterday.
Non-trading days do not require a new quote. Yahoo has no configured period cap;
brapi coverage depends on token/plan, and public CoinGecko history reaches back
at most 365 days. Provider coverage is not a guarantee that every asset/date has
a price. A shorter fallback history leaves older ranges eligible for later work.
Changing provider priority alone does not refetch already covered history.

Historical market closes reflect splits/reverse splits, not dividend-adjusted
prices. Returns use a price on/before the day, at most ten days old, unless a
valid typed valuation takes precedence. Older quotes can leave returns unavailable
even while a current holding value remains visible. Imported institution prices
are preserved against market-provider replacement on the same day.

## Review performance for a period

Choose a period in Investments and group by asset class, custody or neither.
The summary and rows share that period; future end dates stop at today. Expand
**Calculation details** and the information popovers for limitations and sources.

- Portfolio value covers positions, excluding uninvested cash. **Accounts**
  shows today's available cash, independently of the selected period.
- Yield is ending value minus opening value minus net contributions. The
  percentage uses Modified Dietz, allowing for when money entered or left;
  deposits are not earnings. Manual account-level income is separate from
  position performance. Purchase/sale costs are already reflected in yield.
- Closed positions still contribute to totals. Search and hiding closed rows
  only change visible rows. Enable **Show closed** to inspect their results.
- Missing history, balances or FX produce unavailable figures, not zero returns.
  A partial portfolio total covers only measurable positions. A late priced
  start is disclosed. Institution-reported profit is a comparison figure;
  provisioned taxes are informational and not deducted from the period return.

Foreign synced positions retain native amounts. Period figures use one FX rate
per currency at the period end for both values and flows, excluding exchange
variation within that period. Consolidated figures can then be expressed in your
reference currency. Missing FX is disclosed rather than guessed.

## Avoid counting a bank movement twice

In Transactions, open an eligible BRL bank entry and its investment-link section.
Select a local investment operation or imported investment movement, or create
a new operation there. Review suggestions before confirming; matching is not
automatic. Link only the
relevant portion: a fully linked contribution/withdrawal leaves bank income/
expense totals, while an unmatched remainder keeps its ordinary treatment.
Fees, taxes and income can also be linked to avoid duplicate reporting.

Linking never changes provider balances. Linked brokerage cash already counted
as bank cash is not counted again in wealth. Undo links before editing/deleting
a reconciled manual bank transaction; ignoring it also releases the links.
