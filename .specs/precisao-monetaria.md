# Monetary precision at application boundaries

Living policy for application arithmetic, input and output; database types and
migrations are separate (#34). This is shared by financial invariants (#29),
database decoding (#31) and report eligibility (#32); precision never changes
which entries a report includes.

## Currencies and operations

BRL is the reporting currency for payables, scenarios and manual investments.
Accounts and imported positions retain their native currency (three-letter
codes, including USD, EUR and JPY). Keeping a currency does not imply an FX rate
exists or that it enters BRL totals. There is no ISO minor-unit lookup: foreign
amounts retain their supplied precision, including currencies with zero or three
minor-unit decimals. Crypto quantities and unit prices are decimals, not BRL
cash amounts.

| Operation | Precision and rounding |
| --- | --- |
| HTTP money input/output, native balances, movements, fees, taxes, prices and quantities | Application clients send decimal strings and responses return strings, without binary floating point. No rounding to minor units. Fixed-point responses preserve value and available scale; insignificant trailing zeros can differ after database/provider normalization. Signed zero is normalized. |
| Addition, subtraction, multiplication, minimum, clamping, reconciliation and aggregation | Exact decimal arithmetic at the operands' precision. Round only the final display, never each input before summing. |
| Money editing | Decimal strings throughout; pt-BR comma and grouping. Show at least two fractional digits, preserving extra digits on blur and on unchanged saves. |
| BRL read-only display | Two fractional digits, nearest cent; ties away from zero (`1.005` → `1.01`, `-1.005` → `-1.01`). A value rounded to zero has no sign or tint. |
| Other currency read-only display | Currency-code prefix, pt-BR grouping, at least two fractional digits; no rounding of extra digits. This is a precision-preserving display convention, not an ISO cash settlement rule. |
| Equal BRL installments | Non-negative amounts only; divide exactly and floor to cents. The last backend installment absorbs the entire remainder, including fractional cents. No preliminary rounded division. |
| FX | Direct rate multiplication is exact; inverse conversion rounds half away from zero to 12 decimal places (`exchange.DivisionPrecision`). Original money is unchanged. |
| Investment valuation from quotes | Quantity × quote, rounded half away from zero to BRL cents. Typed valuations retain their precision. Derived values are never persisted. |
| Investment unit cost, proportional cost, ratios | Decimal division to 16 places (shopspring default), half away from zero. Issue unit prices use 12 places. Card utilization rounds the resulting ratio to 4 places. These are derived figures, not money inputs. |
| Provider quotes | Preserve provider decimal text; Yahoo prices round half away from zero to its instrument `priceHint` (default 2); crypto USD × BRL conversion uses at least 2 places and the instrument hint. |
| CSV Flash cash amounts | The file contract supplies cents; normalized to 2 places. This does not constrain general HTTP input. |
| Amount-rule percentage tolerance | Parse reference and percentage as exact decimals; calculate `abs(reference) × percentage × 0.01` without rounding. Inclusive comparison at the exact boundary. |

For compatibility, older decimal-typed Go request fields also accept raw JSON
numbers: the decimal decoder reads the original token exactly, without a float
conversion. New clients use strings; frontend response parsers reject money
numbers.

JavaScript numbers may be used for chart coordinates and visual percentages,
never to produce a monetary write or an authoritative monetary total. Cents-only
range filters intentionally accept two decimals; they do not edit stored money.

## Ownership and verification

Go uses `shopspring/decimal` and `money.CanonicalDecimal`; TypeScript uses the
BigInt decimal helpers in `presentation/decimal.ts` and the formatting helpers
in `presentation/money.ts`. `testdata/money_precision.json` contains shared
fractional, negative, large-value and rounding-tie examples consumed by both
suites. API round-trip tests exercise actual write/read handlers, including
recurrences and reconciliation responses. No currency precision is enforced by a
schema change in this work.
