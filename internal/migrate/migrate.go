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

// Embedded so the scratch runtime image (no shell, no sqlite CLI) can apply
// schema on boot.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Run applies pending migrations via goose (forward-only, run-once).
func Run(ctx context.Context, dbURL string) error {
	db, err := sql.Open("sqlite", dbURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("sub fs: %w", err)
	}
	gooseProvider, err := goose.NewProvider(goose.DialectSQLite3, db, sub)
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}

	results, err := gooseProvider.Up(ctx)
	// Report applied files even on failure so a bad migration is diagnosable.
	for _, r := range results {
		fmt.Printf("migrate: applied %s\n", r.Source.Path)
	}
	if err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
