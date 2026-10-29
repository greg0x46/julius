# Statement import

## What exists

Under **Contas e cartões → Importar extrato**, the user uploads a CSV, picks an
existing file account or creates a new one, reviews the per-row preview and
confirms. The screen shows period, balance, new / already imported / invalid
rows, warnings and recent history. Invalid rows are skipped only when the user
explicitly chooses to import the valid ones. The screen offers a template CSV
download, with fictitious sample rows, for anyone who has to build the file in
the accepted format.

The first accepted format is the Flash CSV with columns `Data`, `Hora`,
`Movimentação`, `Valor`, `Meio de Pagamento` and `Saldo`. The parser uses the
`America/Sao_Paulo` time zone and exact BRL decimals. Detector and parser live
in `internal/statementimport`; new formats are adapters registered there, and
each adapter also supplies its own template CSV, built from the same header the
parser checks. The detector accepts what the parser accepts (BOM, CRLF, quoted
fields, spaces after commas). The file name never determines the period.

## Data and rules

- File accounts use `data_sources.provider = 'file'` and appear alongside the
  other accounts. The schedule, the worker and the Open Banking screen process
  only `pluggy` sources.
- The preview writes nothing. Confirmation re-uploads the file and checks
  SHA-256, format, version and row classification; any change requires a new
  preview.
- Each movement's identity is derived from account, instant, description,
  amount, payment method, final balance and ordinal among equal rows. This
  avoids duplicates when re-uploading the file or uploading overlapping periods:
  a repeat records only a new run and a new import record, with no entries,
  decisions or balance change. A row with the same textual data but a different
  final balance is flagged as ambiguous and requires a decision; the earlier
  row is not changed. The account balance does not advance in that ambiguous
  import. The old identity (without balance) is still recognized when
  re-uploading already imported movements whose balance matches.
- Exact normalized matches (balance and ordinal included) are skipped as
  duplicates regardless of the CSV hash. Legacy identities and explicit
  "import as another transaction" identities are recognized by their normalized
  data too, preserving repeated-row multiplicity across overlapping files.
- For balance conflicts, the preview supplies `conflicts`, keyed by incoming
  line number, with the existing transaction's id, timestamp, description,
  amount, currency, payment method, balance and original filename when available.
  The UI shows shared details once and highlights both balances, stacked on
  phones and side by side on wider screens. Each conflict requires an explicit
  choice: keep existing / ignore incoming, or import as another transaction.
  Existing entries are never changed. An import choice uses the file hash and
  line number for its distinct identity; an ignore choice records an unchanged
  normalization event linked to the existing transaction. Re-uploading after
  either choice is automatically skipped, without another decision.
  Duplicate rows remain ineligible for the account balance anchor on replay
  after either choice, including when the account has no balance timestamp yet.
  The preview fingerprint includes the compared existing data; changing that
  data or the classification before confirmation requires a new preview.
- Identical rows within the same file are distinct movements (ordinal 1, 2, ...).
  Known limit: a later file with more identical rows than the earlier one
  imports only the excess.
- The identity includes the account: the same file in two accounts yields
  independent entries.
- The account balance advances only with a row later than the current balance
  and with an unambiguous final balance from a new row. When importing only
  the valid rows, an invalid row with an unreadable date or one not older than the last valid row
  prevents the balance update (the valid movements are still imported), because
  the balance could end up stale; older invalid rows do not interfere.
  Continuity gaps are shown as warnings; the system never creates movements to
  close a gap.
- Confirmation writes run, raw CSV, account, movements and events in one
  database transaction; any failure undoes everything (including rows already
  inserted and the new account) and the same upload can be repeated. The raw
  file stays in the database for audit and is not included in logs or
  responses. The run is separate from Open Banking syncs.
  Learned categorization and automation apply to the new entries.
- File transactions show origin **Arquivo** in the statement and detail; the
  origin filter distinguishes Arquivo, Conexão automática and Manual.

## API

- `POST /api/statement-imports/preview` — multipart upload, `file`, optional
  `account_id`, optional `format`; returns rows, counts, format, version, hash,
  the preview fingerprint and structured conflict details.
- `POST /api/statement-imports` — same file, existing account or new account
  name, expected hash/format/version/fingerprint and `allow_partial`; returns
  account, run and the counts written.
- `GET /api/statement-imports/accounts` — BRL file accounts.
- `GET /api/statement-imports` — up to 100 recent imports.
- `GET /api/statement-imports/template` — template CSV (optional `format`,
  default `flash_csv`), as attachment `modelo-extrato-flash.csv`; unknown format
  returns 422.

File read errors return 422 with the parser's message, an upload over the limit
returns 413, and a database failure returns 503 without technical detail (the
cause goes only to the log).

Uploads accept up to 2 MiB and 10,000 rows. The first format is limited to
BRL checking-type accounts. There is no automatic reconciliation with Pluggy
account transactions; exact coincidences with Pluggy transactions or with a
manual entry of the same account appear as a warning, never merged
automatically: the file row is written and the others are not changed. The amount is compared in every spelling it may be stored with
(`-10`, `-10.0`, `-10.00`), because Pluggy and manual entries preserve the
source scale.
