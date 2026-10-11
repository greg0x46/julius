# Account statement import — plan (first delivery implemented)

The current state of the first delivery is in
[`contextos/importacao-extratos/reference.md`](contextos/importacao-extratos/reference.md).
This plan remains to track extensions: new formats, review of balance
corrections on repeated movements, reconciliation between file accounts and
Pluggy, and validating the Postgres migration in an integrated environment.

## Goal and scope

Allow importing transactions from files of different institutions into the same
Ledger engine. The first adapter is the Flash CSV. The first delivery covers
BRL checking-type accounts, manual upload, preview, confirmation, history and
safe re-import. Other formats must require a new adapter, without changing the
persistence rules or the review interface. Not included in this delivery:
credit card bills, OFX, and automatic reconciliation with Pluggy transactions.

The real file received was used only to analyze the format. Do not version it
or use it as a fixture; it contains personal financial data.

## Evidence from the Flash CSV received

- UTF-8 encoding with BOM, comma separator and header `Data, Hora,
  Movimentação, Valor, Meio de Pagamento, Saldo`.
- 19 rows; dates between 08/09/2026 and 25/09/2026; all six columns filled.
  Signed amount in reais with a decimal comma and a non-breaking space
  (`-R$ 49,90`); time in `HH:mm`.
- There are outflows by `Cartão` and `Pagamento PIX`, and inflows by
  `Depósito`. In this sample, the difference between the balances of adjacent
  rows matches the most recent row's amount. The file is in descending order,
  but the parser must not depend on that order.
- The file name contains `06-2026` although the rows are from September 2026.
  Date, period and balance must come from the content, never the file name.
- No column provides a transaction or account identifier. A statement with
  balances does not prove by itself that the account is a credit card; treat it
  as a checking-type account until there is evidence otherwise.

## Importer contract

1. `Detector` recognizes the format by content, header and encoding; an optional
   explicit format can resolve ambiguities. The adapter version is part of the
   result.
2. `Parser` takes bytes and produces normalized rows with line number, instant in
   the `America/Sao_Paulo` zone, original description, exact signed decimal
   amount, currency, payment method, balance after the movement and any
   errors/warnings. No adapter writes to the database.
3. The common service takes the target account, normalized rows and file hash.
   It computes identities, classifies rows as new, already imported or invalid,
   and produces the same preview that is revalidated at confirmation. The Flash
   parser converts `dd/mm/yyyy`, `HH:mm`, `R$` and the decimal comma without
   `float`.
4. The whole file must fail if header, encoding, structure or account
   association are ambiguous. Invalid rows must appear with number and reason in
   the preview; confirmation only with an explicit choice to import the valid
   ones, or after fixing the file. Never create silently incomplete
   transactions.

## Identity, duplicates and balance

- The imported account has a persistent local identity, chosen on first import
  and reused afterwards. Do not derive the account from the file name. In the
  first delivery, create a file account or pick one previously created for
  files; do not write directly into a Pluggy account, as that would allow
  duplicate transactions and a balance dispute.
- Use `data_sources.provider = 'file'` and a stable local identifier, distinct
  from `format = 'flash_csv'`. The Pluggy schedule and sync button must select
  only Pluggy sources; the worker must not claim file runs.
- Without a native ID, an entry's provisional identity is a versioned hash of
  account + date/time + normalized description + amount + payment method, with
  an ordinal for identical occurrences. Ignore the file name, file hash and row
  position to allow overlapping/reordered files. The ordinal preserves
  multiplicity but does not identify a description/amount correction: the
  preview must flag collisions or possible corrections for review, without
  automatically replacing already categorized transactions. `Saldo` goes into
  the content hash, not the identity, so that data can be corrected without
  creating another transaction. The same account + file can be re-sent without
  inserting duplicates.
- Adjacent balances must be checked when the statement allows. A continuity gap
  becomes a warning, since there may be a cut or missing movements; never invent
  entries to close the difference. The account balance only advances with the
  balance of the most recent known transaction, storing its reference instant;
  importing an old file does not regress it.
- Show possible matches with manual/Pluggy entries as a warning for review; do
  not merge by similarity. Preserve the category, inclusion and link decisions of
  existing entries.

## Persistence and API

- Reuse `financial_accounts`, `financial_transactions`, `data_sources`,
  `sync_runs`, `raw_imports` and `normalization_events` to integrate querying,
  automation, balance and audit trail. Create equivalent SQLite and Postgres
  migrations to represent the file origin, import metadata and the account's
  `balance_as_of`, and adjust `raw_imports` fields/checks for upload. The raw CSV
  is stored only at confirmation, associated with the run, with its SHA-256
  hash; do not record it in logs or API responses.
- Extract from the Pluggy sync the common account/transaction persistence,
  normalized hash, events, counters and categorization/automation hooks. The file
  adapter must not simulate Pluggy API responses. A confirmation must be atomic:
  file, run, account, transactions and events go in together, or nothing does.
  Post-write hooks need a retry strategy that does not duplicate entries.
- `POST /api/statement-imports/preview`: multipart with file, optional format and
  optional target account; returns detected format, period, currency, counts, rows
  with errors/warnings and a limited sample. Persists nothing.
- `POST /api/statement-imports`: re-sends the file and confirms the account,
  version/format and hash seen in the preview; revalidates everything under a
  transaction and returns run + effective counts. A mismatch between preview and
  confirmation must return a readable conflict, not import blindly.
  History list/detail may reuse the runs API with a type filter, as long as they
  do not confuse import with sync.
- Limit upload size and row count; require the authentication already used by
  financial routes; error messages without financial content.

## Frontend experience

Flow: select CSV → detect format → pick/create account and check period, balance,
new/repeated/invalid → confirm → see result and history. Show an explicit warning
when there are invalid rows, possible duplicates with other origins, or a
balance difference. Identify the entry's origin as file in the detail and in the
existing filters, keeping the categorization and inclusion actions.

## Implementation steps and acceptance

1. **Domain base and migrations:** register file sources/accounts and
   distinguish manual runs from scheduled ones. Acceptance: Pluggy keeps
   syncing; a file source is never sent to the worker.
2. **Flash parser and preview:** isolated parser, exact amounts, dates and time
   zones, per-row errors, balance validation and duplicate detection. Acceptance:
   synthetic fixture with header/BOM and inflow/outflow variations; a test for a
   file name with a misleading period and reordered rows.
3. **Confirmation and audit:** atomic transaction, stable identity, re-import,
   overlapping files, balance with reference date, events and hooks. Acceptance:
   repeated upload inserts zero transactions; an old file does not regress the
   balance; a failure midway leaves no partial import; SQLite and Postgres obey
   the same contract.
4. **Interface:** preview and confirmation with errors and history, including
   conflict states. Acceptance: full flow on a new and an existing file account,
   with counts equal to those written after revalidation.

Before implementing, confirm with additional samples whether Flash always exports
the same header/columns and whether each row's balance represents the balance
after the movement. The first CSV supports these hypotheses but does not
guarantee all of the institution's export formats.
