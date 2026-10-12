package db

import "embed"

//go:embed migrations/archive/sqlite/*.sql migrations/archive/postgres/*.sql
var legacyMigrationsFS embed.FS
