// Package testdb gives integration tests a freshly migrated PostGIS database.
//
// By default it starts one postgis/postgis container per test binary with
// testcontainers. Set PLEB_TEST_DATABASE_URL to an existing server's admin URL
// (for example the docker compose db) to skip the container.
package testdb

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/yocca/pleb-api/internal/db"
)

const image = "postgis/postgis:16-3.4"

var (
	serverOnce sync.Once
	serverURL  string
	serverErr  error
	counter    atomic.Int64
)

// adminURL returns a URL for a server where the tests may create databases.
func adminURL() (string, error) {
	serverOnce.Do(func() {
		if u := os.Getenv("PLEB_TEST_DATABASE_URL"); u != "" {
			serverURL = u
			return
		}
		ctx := context.Background()
		// The container is left for testcontainers' reaper to remove when the test binary exits.
		c, err := postgres.Run(ctx, image,
			postgres.WithDatabase("postgres"),
			postgres.WithUsername("postgres"),
			postgres.WithPassword("postgres"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).WithStartupTimeout(90*time.Second)),
		)
		if err != nil {
			serverErr = fmt.Errorf("start postgis container: %w", err)
			return
		}
		serverURL, serverErr = c.ConnectionString(ctx, "sslmode=disable")
	})
	return serverURL, serverErr
}

// New creates an empty database, applies all migrations and returns a pool to it.
// The database is dropped when the test ends.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	pool, _ := NewWithURL(t)
	return pool
}

// NewWithURL is New but also returns the database's connection URL.
func NewWithURL(t testing.TB) (*pgxpool.Pool, string) {
	t.Helper()
	pool, dbURL := NewUnmigrated(t)
	if err := db.Migrate(context.Background(), pool, "up"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool, dbURL
}

// NewUnmigrated creates an empty database without applying migrations.
func NewUnmigrated(t testing.TB) (*pgxpool.Pool, string) {
	t.Helper()
	ctx := context.Background()
	admin, err := adminURL()
	if err != nil {
		t.Fatalf("test database: %v", err)
	}

	name := fmt.Sprintf("pleb_test_%d_%d", os.Getpid(), counter.Add(1))
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}
	conn.Close(ctx)

	u, err := url.Parse(admin)
	if err != nil {
		t.Fatalf("parse admin url: %v", err)
	}
	u.Path = "/" + name
	dbURL := u.String()

	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		conn, err := pgx.Connect(context.Background(), admin)
		if err != nil {
			return
		}
		defer conn.Close(context.Background())
		_, _ = conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	return pool, dbURL
}

// Exec runs SQL statements, failing the test on error.
func Exec(t testing.TB, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", strings.TrimSpace(sql), err)
	}
}
