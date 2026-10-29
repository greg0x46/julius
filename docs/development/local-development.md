# Local development

After configuring the key and preparing the account with
`go run ./cmd/julius auth init` (or `auth migrate` for an old database),
the script below starts the backend and Vite together. The frontend gets hot
reload and `/api` requests are forwarded to the backend automatically:

```sh
./dev.sh
```

Open the URL the script prints on the `Frontend:` line (`http://127.0.0.1:5173`
by default). The script uses exactly that origin as the default
`JULIUS_PUBLIC_URL`, and the backend strictly compares the browser's
`Origin` header against it: opening `http://localhost:5173` instead, or having
already exported another value in `JULIUS_PUBLIC_URL`, results in a 403
`invalid-origin` on login and on every POST/PUT/DELETE. If you prefer another
host, change `VITE_DEV_HOST` (the script derives the origin from it) instead of
exporting the URL by hand. `Ctrl-C` stops both processes. To override host and
ports:

```sh
JULIUS_DEV_ADDR=localhost:8100 VITE_DEV_HOST=localhost VITE_DEV_PORT=5174 ./dev.sh
```

For machine-specific configuration, create a `.env.local` at the repository
root (ignored by git) with `KEY=value` lines; `dev.sh` loads that file before
starting. E.g. `JULIUS_DB`, `JULIUS_MASTER_KEY_FILE`, ports and, to
reach Vite through another domain, `__VITE_ADDITIONAL_SERVER_ALLOWED_HOSTS=<host>`.

If the dependencies are not installed yet, run `cd frontend && npm install`
once before starting the script.
