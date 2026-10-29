<p align="center">
  <img src="./frontend/src/assets/julius.png" alt="Julius" width="160" />
</p>

# Julius

**Know where your money goes. See where it's headed.**

Julius is a self-hosted personal finance app that brings your accounts, transactions, investments, and financial plans together in one place. Powered by Open Finance, it helps you understand your money today and plan what comes next.

## Features

- **Open Finance** — sync accounts, cards, transactions, and investments via Pluggy; start with manual accounts and CSV imports, too.
- **Transactions & automation** — categories, configurable rules, own-account transfers, recurring commitments, and payables.
- **Financial planning** — projections, daily spending allowance, future balance, and explicitly hypothetical scenarios selected when you need them.
- **Goals** — investment targets, progress, and estimated monthly contributions to meet a target date.
- **Investments & net worth** — portfolio positions, performance, and cash plus investments over time.
- **Automatic market data** — scheduled quote updates and historical prices for supported B3-listed stocks, units, FIIs, ETFs, BDRs, and crypto assets, with fallback across Yahoo Finance, brapi, and CoinGecko according to market coverage.
- **Multi-currency** — account currencies and consolidated reports in BRL using FX rates; missing rates leave affected valuations unavailable rather than guessing a total.
- **MCP** — optional integration for AI clients, with scoped financial reads and separately authorized financial writes requiring owner approval.

## Why Julius?

Self-hosted and owner-controlled, Julius runs as one Go binary with an embedded React frontend. It uses SQLite by default, with optional PostgreSQL, encrypts stored secrets, and has no public sign-up: you create the owner account yourself.

## Getting started

From a checkout, with Go 1.26.10+, Node.js 24.15+ (or 26+), npm, and OpenSSL installed:

```sh
./build.sh

# Generate the encryption key once, outside the repository; refuse to overwrite it.
mkdir -p "$HOME/.config/julius"
(umask 077; set -C; openssl rand -base64 32 > "$HOME/.config/julius/master.key")
export JULIUS_MASTER_KEY_FILE="$HOME/.config/julius/master.key"
export JULIUS_PUBLIC_URL="http://localhost:4200"

./julius auth init -db ./julius.db
./julius -db ./julius.db
```

The initialization command prompts for the owner's email and password. Open <http://localhost:4200> and sign in. Pluggy is optional: start with manual accounts, or follow the [Meu Pluggy setup guide](docs/guides/open-finance-pluggy.md) to connect your banks and configure Julius under **Configurações → Open Banking**.

Keep the key safe and back it up separately from the database; you need the same key on every restart. Set exactly one of `JULIUS_MASTER_KEY_FILE` or `JULIUS_MASTER_KEY` (a Base64-encoded 32-byte key). For remote access, set `JULIUS_PUBLIC_URL` to the browser-facing HTTPS origin and serve Julius through HTTPS; local HTTP is for development only.

## Documentation

- **Getting Started** — [quick start](#getting-started).
- **User Guides** — [personal Open Finance setup with Meu Pluggy](docs/guides/open-finance-pluggy.md), [feature details in the temporary migration reference](docs/legacy-readme.md#features).
- **Self-Hosting** — [authentication and encryption](docs/legacy-readme.md#authentication-and-encryption-key), [PostgreSQL](docs/legacy-readme.md#running-with-postgres), and [releases and installation](docs/legacy-readme.md#publish-and-install-a-version).
- **Development** — [commands and conventions](AGENTS.md), [domain documentation](.specs/README.md), and [frontend testing](frontend/TESTING.md).

See the [documentation index](docs/README.md) for guides and references.

The [previous README](docs/legacy-readme.md) is preserved in full as a temporary migration reference, including authentication, MCP, database, release, and deployment instructions. Subsequent documentation issues must migrate useful sections into durable guides and add or update their own README links. Remove the temporary reference once that migration is complete.

## Tech stack

Go with `net/http`, React and TypeScript with Vite and TanStack Query, SQLite or PostgreSQL, and Pluggy for Open Finance connectivity.

## Contributing

Read [AGENTS.md](AGENTS.md) for development and contribution conventions. Report bugs and discuss changes in [GitHub Issues](https://github.com/greg0x46/julius/issues). Feature pull requests target `stage`.

## Acknowledgments

Thanks to [Pluggy](https://pluggy.ai) and the Brazilian Open Finance ecosystem for making owner-authorized access to financial data possible.

*Made for people who believe every cent counts.*
