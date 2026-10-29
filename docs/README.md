# Documentation

Start with [Getting started](getting-started.md) to build Julius, create the
owner account, and sign in for the first time.

## Self-hosting

- [Installation](self-hosting/installation.md): published binaries, source builds,
  persistent storage, and running the service.
- [Configuration](self-hosting/configuration.md): required environment settings,
  SQLite, PostgreSQL, and optional integration schedules.
- [Security](self-hosting/security.md): HTTPS, public origin, secret storage,
  authentication, and password recovery.
- [Backups and upgrades](self-hosting/upgrading.md): database and key backups,
  restoration, safe updates, rollback, and legacy cutover caveats.

## Development

- [Development references](development/README.md): architecture, financial invariants,
  monetary precision, API contracts and context decisions.
- [Contributor conventions](../AGENTS.md) and [frontend testing](../frontend/TESTING.md).

## Further reading

- [Read your own financial data with Meu Pluggy](guides/open-finance-pluggy.md): Open Finance setup.
- [MCP setup and authorization](guides/mcp.md): expose the endpoint, connect a
  compatible client, approve changes and revoke access.
- [MCP interoperability checks](mcp.md): repeatable smoke validation and client
  compatibility evidence.
- [Feature reference](legacy-readme.md#features) in the temporary previous README.
- [Changelog](../CHANGELOG.md) and
  [published releases](https://github.com/greg0x46/julius/releases).

Return to the [project README](../README.md).
