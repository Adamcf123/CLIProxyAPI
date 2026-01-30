package metricspersist

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Migrate(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	// Enforce retention policy at startup to keep DB size stable.
	if _, err := Cleanup(db, defaultRetentionDays); err != nil {
		return fmt.Errorf("cleanup old metrics: %w", err)
	}

	return nil
}
