// Package postgres implements the store.Store interface backed by PostgreSQL + pgvector.
package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alash3al/stash/internal/store"
)

// querier is the minimal interface satisfied by *pgxpool.Pool and pgx.Tx.
// It allows the same Store code to work with both connection pools and transactions.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store implements store.Store using PostgreSQL.
type Store struct {
	db   querier       // used for all queries (pool or tx)
	pool *pgxpool.Pool // nil for tx-backed stores; used for lifecycle
	cfg  Config
}

// Config holds PostgreSQL-specific configuration.
type Config struct {
	// DSN is the PostgreSQL connection string.
	DSN string

	// VectorDim is the dimension of all vectors stored in this store.
	// All vectors must have this exact dimension.
	VectorDim int

	// IndexedMetadata lists JSONB paths to create B‑tree indexes on.
	// Example: []string{"kind", "entity_id"} creates indexes on
	// (metadata->'kind') and (metadata->'entity_id').
	IndexedMetadata []string

	// MaxResultSize is the hard cap on Limit in List and Search.
	// If a caller requests a larger limit, it is silently truncated.
	// Zero means the default (10000).
	MaxResultSize int
}

const (
	defaultMaxResultSize = 10000
)

// New creates a new PostgreSQL-backed Store.
func New(cfg Config) (store.Store, error) {
	if cfg.VectorDim <= 0 {
		return nil, errors.New("postgres: VectorDim must be positive")
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse DSN: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}

	s := &Store{db: pool, pool: pool, cfg: cfg}

	// Verify connection and extensions
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.Health(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: health check failed: %w", err)
	}

	// Create pgvector extension if not present
	if _, err := pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector"); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: enable pgvector: %w", err)
	}

	// Run migrations
	if err := s.Migrate(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: migrate: %w", err)
	}

	return s, nil
}

// Health checks the store's connectivity.
func (s *Store) Health(ctx context.Context) error {
	if s.pool == nil {
		return errors.New("postgres: health check not available in transaction")
	}
	return s.pool.Ping(ctx)
}

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies schema migrations.
func (s *Store) Migrate(ctx context.Context) error {
	if s.pool == nil {
		return errors.New("postgres: migrate not available in transaction")
	}
	// Get current version
	var currentVersion int
	err := s.pool.QueryRow(ctx, `
		SELECT version FROM schema_version ORDER BY version DESC LIMIT 1
	`).Scan(&currentVersion)
	if err != nil {
		// Distinguish between "table doesn't exist" (expected on first run)
		// and real errors (connection failure, permission denied, etc.)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
			// undefined_table - expected on first run, version is 0
			currentVersion = 0
		} else if errors.Is(err, pgx.ErrNoRows) {
			// No rows - table exists but empty, version is 0
			currentVersion = 0
		} else {
			// Real error - connection failure, permission, etc.
			return fmt.Errorf("postgres: check schema version: %w", err)
		}
	}

	// Apply migrations in order
	migrations := []struct {
		version int
		sql     string
	}{
		{1, mustReadMigration(1)},
	}

	for _, mig := range migrations {
		if mig.version <= currentVersion {
			continue
		}

		if _, err := s.pool.Exec(ctx, mig.sql); err != nil {
			return fmt.Errorf("postgres: migration %d: %w", mig.version, err)
		}

		// Record version
		_, err = s.pool.Exec(ctx, `
			INSERT INTO schema_version (version) VALUES ($1)
			ON CONFLICT (version) DO NOTHING
		`, mig.version)
		if err != nil {
			return fmt.Errorf("postgres: record version %d: %w", mig.version, err)
		}
	}

	// Set vector column dimension (required for HNSW index).
	// The migration creates the column as dimensionless; alter it to the configured dimension.
	alterSQL := fmt.Sprintf("ALTER TABLE record_vectors ALTER COLUMN vector TYPE vector(%d)", s.cfg.VectorDim)
	if _, err := s.pool.Exec(ctx, alterSQL); err != nil {
		return fmt.Errorf("postgres: set vector dimension: %w", err)
	}

	// Create HNSW index on vectors.
	if _, err := s.pool.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_record_vectors_vector_hnsw
		ON record_vectors
		USING hnsw (vector vector_cosine_ops)
	`); err != nil {
		return fmt.Errorf("postgres: create HNSW index: %w", err)
	}

	// Create indexes on configured metadata paths
	if err := s.createMetadataIndexes(ctx); err != nil {
		return fmt.Errorf("postgres: create metadata indexes: %w", err)
	}

	return nil
}

func mustReadMigration(version int) string {
	// Try exact filename first
	filename := fmt.Sprintf("%04d_initial.sql", version)
	data, err := migrationsFS.ReadFile("migrations/" + filename)
	if err != nil {
		// If not found, search for any migration file with this version
		entries, err := migrationsFS.ReadDir("migrations")
		if err != nil {
			panic(fmt.Sprintf("postgres: read migrations dir: %v", err))
		}

		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), fmt.Sprintf("%04d_", version)) && strings.HasSuffix(entry.Name(), ".sql") {
				data, err = migrationsFS.ReadFile("migrations/" + entry.Name())
				if err != nil {
					panic(fmt.Sprintf("postgres: read migration %s: %v", entry.Name(), err))
				}
				return string(data)
			}
		}
		panic(fmt.Sprintf("postgres: migration %d not found", version))
	}
	return string(data)
}

func (s *Store) createMetadataIndexes(ctx context.Context) error {
	for _, path := range s.cfg.IndexedMetadata {
		// Validate path components - only alphanumeric and underscore allowed
		parts := strings.Split(path, ".")
		for _, part := range parts {
			if !isValidIdentifier(part) {
				return fmt.Errorf("postgres: invalid metadata path component %q: must be alphanumeric + underscore", part)
			}
		}

		// Convert dotted path to JSONB expression:
	// metadata->>'key' for single-level, metadata->'a'->>'b' for nested.
	// Uses -> for intermediate levels (returns JSONB) and ->> for the
	// final level (returns text) so the B-tree index is on a stable type.
	expr := "metadata"
	for i, part := range parts {
		if i < len(parts)-1 {
			expr += fmt.Sprintf("->'%s'", part)
		} else {
			expr += fmt.Sprintf("->>'%s'", part)
		}
	}

	sql := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS idx_records_metadata_%s 
		ON records ((%s))
	`, strings.ReplaceAll(path, ".", "_"), expr)

		_, err := s.pool.Exec(ctx, sql)
		if err != nil {
			return fmt.Errorf("postgres: create index on metadata.%s: %w", path, err)
		}
	}
	return nil
}

// isValidIdentifier checks if a string is a valid SQL identifier (alphanumeric + underscore).
func isValidIdentifier(s string) bool {
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return len(s) > 0
}

// Close releases the connection pool.
func (s *Store) Close() error {
	if s.pool == nil {
		return errors.New("postgres: close not available in transaction")
	}
	s.pool.Close()
	return nil
}

// maxResultSize returns the configured max result size.
func (s *Store) maxResultSize() int {
	if s.cfg.MaxResultSize > 0 {
		return s.cfg.MaxResultSize
	}
	return defaultMaxResultSize
}

// validateRecord checks a record for basic validity.
func (s *Store) validateRecord(r store.Record) error {
	if r.ID == "" {
		return errors.New("record ID cannot be empty")
	}
	for name, vec := range r.Vectors {
		if len(vec.Values) != s.cfg.VectorDim {
			return fmt.Errorf("vector %q has dimension %d, expected %d", name, len(vec.Values), s.cfg.VectorDim)
		}
		if vec.Model == "" {
			return fmt.Errorf("vector %q missing model identifier", name)
		}
	}
	return nil
}
