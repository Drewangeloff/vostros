package repository

import (
	"context"
	"embed"
	"fmt"
	"log"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

func RunMigrations(ctx context.Context, pool *pgxpool.Pool, migrationsFS embed.FS) error {
	// Keep the lock and all schema changes in one transaction. This serializes
	// concurrent starts and also works through transaction-pooling proxies.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("starting migrations: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(784637267)`); err != nil {
		return fmt.Errorf("locking migrations: %w", err)
	}
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    )`); err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("reading migrations directory: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var applied []string
	for _, entry := range entries {
		name := entry.Name()
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, name).Scan(&exists); err != nil {
			return fmt.Errorf("checking migration %s: %w", name, err)
		}
		if exists {
			continue
		}
		content, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}
		if _, err = tx.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("executing migration %s: %w", name, err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			return fmt.Errorf("recording migration %s: %w", name, err)
		}
		applied = append(applied, name)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing migrations: %w", err)
	}
	for _, name := range applied {
		log.Printf("applied migration: %s", name)
	}
	return nil
}
