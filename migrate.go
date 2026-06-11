package main

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// migrations/*.sql is embedded so the scratch runtime image (no shell, no
// psql) can apply schema on the Fly release_command. ReadDir returns names
// lexically sorted, which is the apply order (0001, 0002, …).
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// runMigrations applies every migrations/*.sql in order. Each file is sent as
// one simple-protocol Exec so multi-statement files (e.g. the DO $$ … $$ block
// in 0003) run whole, the same way `psql -f` applies them. Migrations are
// additive + idempotent, so re-applying the full set is safe.
func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	for _, e := range entries {
		sql, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return fmt.Errorf("read %s: %w", e.Name(), err)
		}
		if _, err := pool.Exec(ctx, string(sql), pgx.QueryExecModeSimpleProtocol); err != nil {
			return fmt.Errorf("apply %s: %w", e.Name(), err)
		}
	}
	return nil
}
