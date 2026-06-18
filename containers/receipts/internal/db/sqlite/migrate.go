package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type migration struct {
	version int
	name    string
	body    string
}

// migrate applies every embedded migration whose version exceeds the current
// schema version, in ascending order, each in its own transaction. It is
// idempotent and safe to call on every startup.
func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY NOT NULL,
			name       TEXT    NOT NULL,
			applied_at TEXT    NOT NULL
		)`); err != nil {
		return fmt.Errorf("sqlite: ensure schema_migrations: %w", err)
	}

	var current int
	if err := db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("sqlite: read schema version: %w", err)
	}

	migs, err := loadMigrations()
	if err != nil {
		return err
	}
	for _, m := range migs {
		if m.version <= current {
			continue
		}
		if err := applyMigration(ctx, db, m); err != nil {
			return fmt.Errorf("sqlite: migration %d (%s): %w", m.version, m.name, err)
		}
	}
	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("sqlite: glob migrations: %w", err)
	}
	migs := make([]migration, 0, len(entries))
	seen := make(map[int]bool, len(entries))
	for _, path := range entries {
		name := strings.TrimPrefix(path, "migrations/")
		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			return nil, fmt.Errorf("sqlite: migration %q is missing a version prefix", name)
		}
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return nil, fmt.Errorf("sqlite: migration %q has a non-numeric version: %w", name, err)
		}
		if seen[version] {
			return nil, fmt.Errorf("sqlite: duplicate migration version %d", version)
		}
		seen[version] = true
		body, err := migrationFS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("sqlite: read %q: %w", name, err)
		}
		migs = append(migs, migration{version: version, name: name, body: string(body)})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].version < migs[j].version })
	return migs, nil
}

func applyMigration(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	for _, stmt := range splitStatements(m.body) {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("exec: %w (rollback: %v)", err, rbErr)
			}
			return fmt.Errorf("exec: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.version, m.name, time.Now().UTC().Format(time.RFC3339)); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("record: %w (rollback: %v)", err, rbErr)
		}
		return fmt.Errorf("record: %w", err)
	}
	return tx.Commit()
}

// splitStatements splits a DDL script into individual statements on ';',
// dropping blank fragments. Line comments are stripped first so a ';' inside a
// comment does not split a statement. Our migrations contain no ';' inside
// string literals, so this is correct and driver-agnostic.
func splitStatements(script string) []string {
	var sb strings.Builder
	for _, line := range strings.Split(script, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	parts := strings.Split(sb.String(), ";")
	stmts := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		stmts = append(stmts, p)
	}
	return stmts
}
