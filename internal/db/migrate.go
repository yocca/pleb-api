// Package db owns the Postgres schema: embedded migrations and the connection pool.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Connect opens a pgx pool and verifies it can reach the database.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrate runs a goose command ("up", "down", "reset", "status", ...) against the database.
func Migrate(ctx context.Context, pool *pgxpool.Pool, command string) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	return migrate(ctx, sqlDB, command)
}

func migrate(ctx context.Context, sqlDB *sql.DB, command string) error {
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, mustSub(migrations, "migrations"))
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	switch command {
	case "up":
		_, err = provider.Up(ctx)
	case "down":
		_, err = provider.Down(ctx)
	case "reset":
		_, err = provider.DownTo(ctx, 0)
	case "status":
		var results []*goose.MigrationStatus
		results, err = provider.Status(ctx)
		for _, r := range results {
			fmt.Printf("%-8s %s\n", r.State, r.Source.Path)
		}
	default:
		return fmt.Errorf("unknown migrate command %q (want up, down, reset or status)", command)
	}
	return err
}

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
