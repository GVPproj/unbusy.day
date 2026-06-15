// Package migrate embeds the SQL migrations and applies them via goose.
package migrate

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

// migrations/*.sql is embedded so the scratch runtime image (no shell, no
// sqlite CLI) can apply schema on boot in the machine that mounts the volume.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Run applies pending migrations exactly once each via goose,
// recording versions in goose_db_version. Forward-only: no Down sections.
func Run(ctx context.Context, dbURL string) error {
	db, err := sql.Open("sqlite", dbURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("sub fs: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, sub)
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
