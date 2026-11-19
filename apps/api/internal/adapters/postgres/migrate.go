// Package postgres implementa repositório, unidade de trabalho e fila sobre PostgreSQL
// usando database/sql + lib/pq.
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationLockID serializa migrações concorrentes (várias instâncias subindo juntas).
const migrationLockID = 7_420_001

// Migrate aplica, em ordem e cada uma em sua transação, as migrações ainda não aplicadas.
func Migrate(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationLockID); err != nil {
		return fmt.Errorf("lock de migração: %w", err)
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, migrationLockID) //nolint:errcheck

	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}

	files, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)

	for _, file := range files {
		version, err := migrationVersion(file)
		if err != nil {
			return err
		}
		var applied bool
		if err := conn.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version,
		).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		body, err := migrationsFS.ReadFile(file)
		if err != nil {
			return err
		}
		if err := applyMigration(ctx, conn, version, string(body)); err != nil {
			return fmt.Errorf("migração %s: %w", file, err)
		}
	}
	return nil
}

func applyMigration(ctx context.Context, conn *sql.Conn, version int, body string) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return err
	}
	return tx.Commit()
}

// migrationVersion extrai 1 de "migrations/0001_init.sql".
func migrationVersion(file string) (int, error) {
	base := strings.TrimPrefix(file, "migrations/")
	prefix, _, ok := strings.Cut(base, "_")
	if !ok {
		return 0, fmt.Errorf("nome de migração inválido: %s", file)
	}
	return strconv.Atoi(prefix)
}
