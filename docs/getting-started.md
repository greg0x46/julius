# Getting started

Build Julius, create its single owner account, and sign in locally. Pluggy is
optional; you can start with manual accounts and CSV imports.

## 1. Build

Install Git, Go 1.26.10 or newer, Node.js 24.15+ on the 24.x line or 26+,
npm, and OpenSSL. Then run:

```sh
git clone https://github.com/greg0x46/julius.git
cd julius
./build.sh
```

The script installs frontend dependencies, builds the UI, and embeds it into
`./julius`. Node.js is only needed at build time. For a prebuilt Linux binary,
see [installation](self-hosting/installation.md).

## 2. Configure the key and local origin

Generate the encryption key **once**, outside the repository. This command
refuses to overwrite an existing key:

```sh
mkdir -p "$HOME/.config/julius"
(umask 077; set -C; openssl rand -base64 32 > "$HOME/.config/julius/master.key")
export JULIUS_MASTER_KEY_FILE="$HOME/.config/julius/master.key"
export JULIUS_PUBLIC_URL="http://localhost:4200"
```

Keep the same key on every restart and back it up separately from the database.
See [security](self-hosting/security.md) for secret storage and remote HTTPS.

## 3. Create the owner and start

In the same interactive terminal:

```sh
./julius auth init -db ./julius.db
./julius -db ./julius.db
```

Initialization prompts for an email, a new password, and password confirmation.
Password entry is hidden; the password must be non-empty. Choose a strong,
unique password. There is no public sign-up.

The database is created and migrated automatically. Keep `julius.db` in a
persistent, writable directory; relative paths resolve from the working
directory. Use only one Julius process with SQLite.

## 4. Sign in

Open <http://localhost:4200> and sign in with the owner credentials. Use exactly
that origin: opening `127.0.0.1` instead of `localhost` makes writes fail with
`invalid-origin`.

Create a manual account or import a CSV to begin. To connect a bank, configure
Pluggy credentials under **Configurações**; see the
[Pluggy reference](guides/open-finance-pluggy.md).

After stopping with Ctrl+C, restart with the same database, key, and public
origin. In a new terminal, export the two variables again; do not generate a
new key or repeat `auth init`.

For a hosted installation, continue with [installation](self-hosting/installation.md)
and [configuration](self-hosting/configuration.md). Before storing data you
need to keep, set up [backups](self-hosting/upgrading.md#back-up).
