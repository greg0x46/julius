# Schema documentation

## Document the schema with SchemaSpy

`docker-compose.schemaspy.yml` runs [SchemaSpy](https://schemaspy.org/) against
a development Postgres reachable from the machine (`host.docker.internal`) and
writes the HTML to `schemaspy/output/` (a git-ignored directory). It is not the
application's compose file. The password is required; user, database and port
default to `admin`, `julius` and `5432`:

```sh
SCHEMASPY_DB_PASSWORD=... docker compose -f docker-compose.schemaspy.yml up
# optional: SCHEMASPY_DB_USER, SCHEMASPY_DB_NAME, SCHEMASPY_DB_PORT
```

Then open `schemaspy/output/index.html`.

See the [legacy inventory](inventory.md) for conversion evidence and DDL snapshots.
