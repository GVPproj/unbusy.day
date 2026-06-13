package migrate

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// migrations/*.sql is embedded so the scratch runtime image (no shell, no
// psql) can apply schema on the Fly release_command.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// runMigrations applies pending migrations exactly once each via goose,
// recording versions in goose_db_version. Forward-only: no Down sections.
func Run(ctx context.Context, dbURL string) error {
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("sub fs: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, sub)
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	results, err := provider.Up(ctx)
	// Report applied files even on failure so a bad migration is diagnosable
	// from Fly release logs alone.
	for _, r := range results {
		fmt.Printf("migrate: applied %s\n", r.Source.Path)
	}
	if err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
