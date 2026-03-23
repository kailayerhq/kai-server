// Package repo provides multi-repo management backed by Postgres.
package repo

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

// PgRegistry returns standard Handle objects with RepoID set.
// The DB field points to the shared Postgres connection pool.

// PgRegistryConfig configures the Postgres-backed repo registry.
type PgRegistryConfig struct {
	ConnStr string // Postgres connection string
}

// PgRegistry manages repos in a shared Postgres database.
type PgRegistry struct {
	db      *sql.DB
	connStr string
	mu      sync.RWMutex
	repos   map[string]*Handle // cache of repo handles
}

// NewPgRegistry creates a registry backed by Postgres.
func NewPgRegistry(cfg PgRegistryConfig) (*PgRegistry, error) {
	db, err := sql.Open("postgres", cfg.ConnStr)
	if err != nil {
		return nil, fmt.Errorf("opening postgres: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(3)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	return &PgRegistry{
		db:      db,
		connStr: cfg.ConnStr,
		repos:   make(map[string]*Handle),
	}, nil
}

// Get returns a handle to the specified repo.
func (r *PgRegistry) Get(ctx context.Context, tenant, repo string) (*Handle, error) {
	key := tenant + "/" + repo

	r.mu.RLock()
	h, ok := r.repos[key]
	r.mu.RUnlock()
	if ok {
		return h, nil
	}

	// Look up repo UUID from control plane's repos table
	var repoID string
	err := r.db.QueryRowContext(ctx,
		"SELECT r.id FROM repos r JOIN orgs o ON r.org_id = o.id WHERE o.slug = $1 AND r.name = $2",
		tenant, repo,
	).Scan(&repoID)
	if err != nil {
		return nil, ErrRepoNotFound
	}

	h = &Handle{
		Tenant: tenant,
		Name:   repo,
		RepoID: repoID,
		DB:     r.db,
	}

	r.mu.Lock()
	r.repos[key] = h
	r.mu.Unlock()

	return h, nil
}

// Create looks up an existing repo (repos are created by the control plane).
func (r *PgRegistry) Create(ctx context.Context, tenant, repo string) (*Handle, error) {
	return r.Get(ctx, tenant, repo)
}

// Exists checks if a repo exists.
func (r *PgRegistry) Exists(ctx context.Context, tenant, repo string) (bool, error) {
	_, err := r.Get(ctx, tenant, repo)
	if err == ErrRepoNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// List returns all repo names for a tenant.
func (r *PgRegistry) List(ctx context.Context, tenant string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT name FROM repos WHERE tenant = $1 ORDER BY name", tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		repos = append(repos, name)
	}
	return repos, rows.Err()
}

// ListTenants returns all tenants.
func (r *PgRegistry) ListTenants(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT DISTINCT tenant FROM repos ORDER BY tenant")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenants []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

// Delete removes a repo and all its data.
func (r *PgRegistry) Delete(ctx context.Context, tenant, repo string) error {
	key := tenant + "/" + repo

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete in order: edges, enrich_queue, node_publish, ref_history, objects, segments, refs, repos
	for _, table := range []string{"edges", "enrich_queue", "node_publish", "ref_history", "objects", "segments", "refs"} {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE repo_id = $1", table), key); err != nil {
			return fmt.Errorf("deleting %s: %w", table, err)
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM repos WHERE id = $1", key); err != nil {
		return fmt.Errorf("deleting repo: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	r.mu.Lock()
	delete(r.repos, key)
	r.mu.Unlock()

	return nil
}

// Acquire is a no-op for Postgres (no LRU eviction needed).
func (r *PgRegistry) Acquire(h *Handle) {}

// Release is a no-op for Postgres (no LRU eviction needed).
func (r *PgRegistry) Release(h *Handle) {}

// Close closes the shared database connection.
func (r *PgRegistry) Close() error {
	return r.db.Close()
}

// DB returns the shared database connection.
func (r *PgRegistry) SharedDB() *sql.DB {
	return r.db
}

// EnsureSchema applies the data plane schema to the database.
func (r *PgRegistry) EnsureSchema(schema string) error {
	_, err := r.db.Exec(schema)
	return err
}
