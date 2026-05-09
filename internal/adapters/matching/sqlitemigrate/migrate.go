package sqlitemigrate

import (
	"context"
	"database/sql"
	"fmt"
)

type Migration struct {
	Version int
	Name    string
	Up      func(context.Context, *sql.Tx) error
}

func Run(ctx context.Context, db *sql.DB, component string, migrations []Migration) error {
	if component == "" {
		return fmt.Errorf("component is empty")
	}

	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
	component TEXT NOT NULL,
	version INTEGER NOT NULL,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (component, version)
);
`); err != nil {
		return err
	}

	applied, err := appliedVersions(ctx, db, component)
	if err != nil {
		return err
	}

	seen := make(map[int]struct{}, len(migrations))
	for _, migration := range migrations {
		if migration.Version <= 0 {
			return fmt.Errorf("migration %q has invalid version %d", migration.Name, migration.Version)
		}
		if migration.Name == "" {
			return fmt.Errorf("migration %d has empty name", migration.Version)
		}
		if migration.Up == nil {
			return fmt.Errorf("migration %d %q has nil up function", migration.Version, migration.Name)
		}
		if _, ok := seen[migration.Version]; ok {
			return fmt.Errorf("migration version %d is duplicated", migration.Version)
		}
		seen[migration.Version] = struct{}{}
		if applied[migration.Version] {
			continue
		}

		if err := apply(ctx, db, component, migration); err != nil {
			return err
		}
	}

	return nil
}

func appliedVersions(ctx context.Context, db *sql.DB, component string) (map[int]bool, error) {
	rows, err := db.QueryContext(ctx, `
SELECT version
FROM schema_migrations
WHERE component = ?
`, component)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return applied, nil
}

func apply(ctx context.Context, db *sql.DB, component string, migration Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := migration.Up(ctx, tx); err != nil {
		return fmt.Errorf("apply migration %s:%d %q: %w", component, migration.Version, migration.Name, err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO schema_migrations (component, version, name)
VALUES (?, ?, ?)
`, component, migration.Version, migration.Name); err != nil {
		return err
	}

	return tx.Commit()
}

func TableExists(ctx context.Context, tx *sql.Tx, table string) (bool, error) {
	var name string
	err := tx.QueryRowContext(ctx, `
SELECT name
FROM sqlite_master
WHERE type = 'table' AND name = ?
`, table).Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func TableHasColumn(ctx context.Context, tx *sql.Tx, table string, column string) (bool, error) {
	tableName, err := quoteIdentifier(table)
	if err != nil {
		return false, err
	}

	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(`+tableName+`)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}

	return false, nil
}

func quoteIdentifier(identifier string) (string, error) {
	if identifier == "" {
		return "", fmt.Errorf("identifier is empty")
	}

	for _, char := range identifier {
		if char >= 'a' && char <= 'z' {
			continue
		}
		if char >= 'A' && char <= 'Z' {
			continue
		}
		if char >= '0' && char <= '9' {
			continue
		}
		if char == '_' {
			continue
		}

		return "", fmt.Errorf("identifier %q contains unsupported character %q", identifier, char)
	}

	return `"` + identifier + `"`, nil
}
