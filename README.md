<p align="center">
  <img src="./frontend/src/assets/julius.png" alt="Julius" width="160" />
</p>

# Julius

**Know where your money goes. See where it's headed.**

Julius is a self-hosted personal finance app that brings your accounts, transactions, investments, and financial plans together in one place. Powered by Open Finance, it helps you understand your money today and plan what comes next.

## Features

- Portuguese/English navigation with a saved browser preference; page translations are incremental.

- **Open Finance** — sync accounts, cards, transactions, and investments via Pluggy; start with manual accounts and CSV imports, too.
- **Transactions & automation** — categories, configurable rules, own-account transfers, recurring commitments, and payables. A [read-only API](docs/development/contexts/recurrences.md#historical-discovery) suggests recurring drafts from historical BRL transactions.
- **Financial planning** — projections, daily spending allowance, future balance, and explicitly hypothetical scenarios selected when you need them.
- **Goals** — investment targets, progress, and estimated monthly contributions to meet a target date.
- **Investments & net worth** — portfolio positions, performance, and cash plus investments over time.
- **Automatic market data** — scheduled quote updates and historical prices for supported B3-listed stocks, units, FIIs, ETFs, BDRs, and crypto assets, with fallback across Yahoo Finance, brapi, and CoinGecko according to market coverage.
- **Multi-currency** — account currencies and consolidated reports in BRL using FX rates; missing rates leave affected valuations unavailable rather than guessing a total.
- **MCP** — optional integration for AI clients, with scoped financial reads and separately authorized financial writes requiring owner approval.

## Why Julius?

Self-hosted and owner-controlled, Julius runs as one Go binary with an embedded React frontend. It uses SQLite by default, with optional PostgreSQL, encrypts stored secrets, and has no public sign-up: you create the owner account yourself.

## Getting started

Follow [Getting started](docs/getting-started.md) to build Julius, create the owner account, and sign in locally. Pluggy is optional: start with manual accounts and CSV imports.

For a hosted instance or prebuilt Linux binary, see [installation](docs/self-hosting/installation.md). Pluggy is optional: to connect your banks, follow the [Meu Pluggy setup guide](docs/guides/open-finance-pluggy.md). Remote access requires HTTPS; keep the encryption key safe and backed up separately from the database.

## Documentation

- **Getting Started** — [first build, owner account, and first login](docs/getting-started.md).
- **User Guides** — [accounts and cards](docs/guides/accounts.md), [transactions and recurring payments](docs/guides/transactions.md), [investments and market prices](docs/guides/investments.md), [financial planning](docs/guides/financial-planning.md), and [connecting banks with Meu Pluggy](docs/guides/open-finance-pluggy.md).
- **Self-Hosting** — [installation](docs/self-hosting/installation.md), [configuration and PostgreSQL](docs/self-hosting/configuration.md), [security and authentication](docs/self-hosting/security.md), and [backups, restore, and upgrades](docs/self-hosting/upgrading.md).
- **MCP** — [setup and authorization](docs/guides/mcp.md), and [interoperability checks](docs/mcp.md).
- **Development** — [commands and conventions](AGENTS.md), [architecture and financial references](docs/development/README.md), and [frontend testing](frontend/TESTING.md).

See the [documentation index](docs/README.md) for all guides.

## Tech stack

Go with `net/http`, React and TypeScript with Vite and TanStack Query, SQLite or PostgreSQL, and Pluggy for Open Finance connectivity.

## Contributing

Read [AGENTS.md](AGENTS.md) for development and contribution conventions. Report bugs and discuss changes in [GitHub Issues](https://github.com/greg0x46/julius/issues). Feature pull requests target `stage`.

## Acknowledgments

Thanks to [Pluggy](https://pluggy.ai) and the Brazilian Open Finance ecosystem for making owner-authorized access to financial data possible.

*Made for people who believe every cent counts.*
