# Installation

Julius runs as one Go binary with its frontend embedded. SQLite is included;
no separate database server or frontend runtime is required.

## Install a published binary

The [GitHub releases](https://github.com/greg0x46/julius/releases) provide a
Linux amd64 binary. Download `julius-linux-amd64` and `SHA256SUMS` from the
**same release** into an empty directory. On Linux, verify before running:

```sh
sha256sum -c SHA256SUMS
chmod +x julius-linux-amd64
mv julius-linux-amd64 julius
```

Require `julius-linux-amd64: OK`. Releases also include
`julius-linux-amd64.sigstore.json`, the build provenance attestation. For other
platforms, build from source using [getting started](../getting-started.md#1-build).

## Prepare persistent storage

Run as a dedicated, unprivileged operating-system user. Give it a persistent
directory for the database and write access to that directory: SQLite also
creates journal files next to the database. Keep the binary separate from data
so replacing it does not replace your database.

Follow [getting started](../getting-started.md#2-configure-the-key-and-local-origin)
to create the master key and owner. For a hosted deployment, use your HTTPS
origin instead of the local URL in that guide. Use the same database path or
DSN for initialization and every subsequent start.

## Keep it running

Use your host's service supervisor to run Julius with an explicit working
directory, persistent database path, and the environment described in
[configuration](configuration.md). Supply the same key after every restart.
The binary does not automatically load `.env.local`; that file belongs to the
development workflow.

The default listener is `localhost:4200`; `-addr` changes it. For a reverse
proxy on the same host, for example:

```sh
./julius -db ./julius.db -addr 127.0.0.1:4200
```

Configure [HTTPS and access protection](security.md#https-and-public-access)
before allowing remote access. Stop Julius with SIGTERM or Ctrl+C and wait for
it to exit before replacing files or taking an offline backup.

SQLite supports one process only. For multiple instances, use
[PostgreSQL](configuration.md#postgresql) and the same external master key on
each instance. Before updating, follow [upgrading](upgrading.md).
