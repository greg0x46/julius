# Public demo

`JULIUS_DEMO=true` runs the instance as an open showcase of synthetic data:

- There is no login. The API behaves as in open mode whatever the stored
  preference says, and `GET /api/auth/session` adds `"demo": true`, which makes
  the UI show a notice.
- Nothing that changes access or reaches a provider can be written: `/api/auth/*`,
  `/api/settings/*`, `POST /api/sync-runs` and writes to `/api/data-sources`
  answer 403 `demo-mode`. Ordinary data stays editable and the `Origin` check
  stays on. The sync worker and the quote refresh do not run.
- `julius demo seed [-db …]` fills a **fresh** database with a synthetic
  household: accounts, six months of transactions, recurring commitments, debts,
  goals and a small portfolio with prices. Dates are relative to the day it runs.
  It needs the same `JULIUS_PUBLIC_URL` and key variables as the server, creates
  the owner with a random password nobody learns, turns authentication off and
  refuses a database that already has an owner: it never deletes anything.

To reset a demo, start from an empty database (a new SQLite file or a new
Postgres database), seed it and restart the server, for instance daily:

```sh
export JULIUS_MASTER_KEY_FILE=demo.key JULIUS_PUBLIC_URL=https://demo.example.com
julius demo seed -db demo.db
JULIUS_DEMO=true julius -db demo.db -addr 127.0.0.1:4200
```

Give the demo its own key and database; never reuse those of a real instance.
