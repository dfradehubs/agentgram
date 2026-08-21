package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "modernc.org/sqlite"

	"github.com/dfradehubs/agentgram-api/internal/config"
)

// RunSQLiteMigrations applies Postgres-authored SQL after a small dialect rewrite.
// Laptop/single-node only: JSONB becomes TEXT, gen_random_uuid() becomes a hex blob.
func RunSQLiteMigrations(cfg config.DatabaseConfig, migrationsPath string) error {
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o755); err != nil && filepath.Dir(cfg.Path) != "." && filepath.Dir(cfg.Path) != "" {
		return fmt.Errorf("sqlite data dir: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "agentgram-sqlite-migrations-*")
	if err != nil {
		return fmt.Errorf("temp migrations: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	entries, err := os.ReadDir(migrationsPath)
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(migrationsPath, e.Name()))
		if err != nil {
			return err
		}
		converted := convertPostgresToSQLite(string(raw))
		if err := os.WriteFile(filepath.Join(tmpDir, e.Name()), []byte(converted), 0o644); err != nil {
			return err
		}
	}

	dsn := cfg.Path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer sqlDB.Close()

	driver, err := sqlite.WithInstance(sqlDB, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("sqlite migrate driver: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+tmpDir, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("create sqlite migrator: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run sqlite migrations: %w", err)
	}
	return nil
}

func convertPostgresToSQLite(sqlText string) string {
	repls := []struct{ old, new string }{
		{"JSONB", "TEXT"},
		{"jsonb", "TEXT"},
		{"TIMESTAMPTZ", "DATETIME"},
		{"timestamptz", "DATETIME"},
		{"DEFAULT gen_random_uuid()", ""},
		{"DEFAULT NOW()", "DEFAULT CURRENT_TIMESTAMP"},
		{"DEFAULT now()", "DEFAULT CURRENT_TIMESTAMP"},
		{"NOW()", "CURRENT_TIMESTAMP"},
		{"TRUE", "1"},
		{"FALSE", "0"},
		{"BOOLEAN", "INTEGER"},
		{"SERIAL", "INTEGER"},
		{"UUID", "TEXT"},
		{"uuid", "TEXT"},
	}
	out := sqlText
	for _, r := range repls {
		out = strings.ReplaceAll(out, r.old, r.new)
	}
	return out
}
