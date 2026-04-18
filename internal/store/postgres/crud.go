package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/alash3al/stash/internal/store"
	"github.com/jackc/pgx/v5"
)

// Put stores a record, creating or replacing it.
func (s *Store) Put(ctx context.Context, r store.Record) error {
	if err := s.validateRecord(r); err != nil {
		return fmt.Errorf("postgres: %w", err)
	}

	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	r.UpdatedAt = now

	if s.pool != nil {
		// Pool-backed: wrap in own transaction
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("postgres: begin tx: %w", err)
		}
		committed := false
		defer func() {
			if !committed {
				tx.Rollback(ctx)
			}
		}()

		if err := upsertRecord(ctx, tx, r); err != nil {
			return err
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("postgres: commit: %w", err)
		}
		committed = true
		return nil
	}

	// Tx-backed: use s.db directly (already in a transaction)
	return upsertRecord(ctx, s.db, r)
}

// Get retrieves a live record by ID.
// Returns ErrNotFound if the record is missing or soft‑deleted.
func (s *Store) Get(ctx context.Context, id string) (store.Record, error) {
	var r store.Record
	var metadata map[string]any

	err := s.db.QueryRow(ctx, `
		SELECT id, content, metadata, created_at, updated_at
		FROM records
		WHERE id = $1 AND deleted_at IS NULL
	`, id).Scan(&r.ID, &r.Content, &metadata, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return store.Record{}, store.ErrNotFound
		}
		return store.Record{}, fmt.Errorf("postgres: get record: %w", err)
	}

	r.Metadata = metadata
	r.Vectors, err = s.loadVectors(ctx, id)
	if err != nil {
		return store.Record{}, fmt.Errorf("postgres: load vectors: %w", err)
	}

	return r, nil
}

// Delete soft‑deletes a record by ID.
func (s *Store) Delete(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE records
		SET deleted_at = $2
		WHERE id = $1 AND deleted_at IS NULL
	`, id, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("postgres: soft delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// Purge hard‑deletes a record by ID.
func (s *Store) Purge(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM records WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("postgres: hard delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// PutMany stores multiple records atomically.
func (s *Store) PutMany(ctx context.Context, rs []store.Record) error {
	now := time.Now().UTC()
	for i := range rs {
		if rs[i].CreatedAt.IsZero() {
			rs[i].CreatedAt = now
		}
		rs[i].UpdatedAt = now
		if err := s.validateRecord(rs[i]); err != nil {
			return fmt.Errorf("postgres: validate record %q: %w", rs[i].ID, err)
		}
	}

	if s.pool != nil {
		// Pool-backed: wrap in own transaction
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("postgres: begin tx: %w", err)
		}
		committed := false
		defer func() {
			if !committed {
				tx.Rollback(ctx)
			}
		}()

		for _, r := range rs {
			if err := upsertRecord(ctx, tx, r); err != nil {
				return err
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("postgres: commit: %w", err)
		}
		committed = true
		return nil
	}

	// Tx-backed: use s.db directly (already in a transaction)
	for _, r := range rs {
		if err := upsertRecord(ctx, s.db, r); err != nil {
			return err
		}
	}
	return nil
}

// upsertRecord writes a single record using the given querier (transaction or pool).
func upsertRecord(ctx context.Context, q querier, r store.Record) error {
	metadata := r.Metadata
	if metadata == nil {
		metadata = make(map[string]any)
	}

	_, err := q.Exec(ctx, `
		INSERT INTO records (id, content, metadata, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, NULL)
		ON CONFLICT (id) DO UPDATE
		SET content = $2, metadata = $3, updated_at = $5, deleted_at = NULL
	`,
		r.ID, r.Content, metadata, r.CreatedAt, r.UpdatedAt)
	if err != nil {
		return fmt.Errorf("postgres: upsert record: %w", err)
	}

	// Delete existing vectors
	_, err = q.Exec(ctx, "DELETE FROM record_vectors WHERE record_id = $1", r.ID)
	if err != nil {
		return fmt.Errorf("postgres: delete old vectors: %w", err)
	}

	// Insert new vectors using pgvector string format: [0.1,0.2,0.3]
	for name, vec := range r.Vectors {
		vecStr := formatVector(vec.Values)
		_, err = q.Exec(ctx, `
			INSERT INTO record_vectors (record_id, name, model, vector)
			VALUES ($1, $2, $3, $4::vector)
		`, r.ID, name, vec.Model, vecStr)
		if err != nil {
			return fmt.Errorf("postgres: insert vector %q: %w", name, err)
		}
	}

	return nil
}

// formatVector converts a float32 slice to pgvector string format: [0.1,0.2,0.3]
func formatVector(values []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", v), "0"), "."))
	}
	b.WriteByte(']')
	return b.String()
}

// parseVector converts a pgvector string format [0.1,0.2,0.3] back to []float32.
func parseVector(s string) ([]float32, error) {
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	values := make([]float32, len(parts))
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
		if err != nil {
			return nil, fmt.Errorf("postgres: parse vector value %q: %w", p, err)
		}
		values[i] = float32(f)
	}
	return values, nil
}

// loadVectors loads all vectors for a record.
func (s *Store) loadVectors(ctx context.Context, recordID string) (map[string]store.Vector, error) {
	vectorsByRecord, err := s.loadVectorsBatch(ctx, []string{recordID})
	if err != nil {
		return nil, err
	}
	return vectorsByRecord[recordID], nil
}

// loadVectorsBatch loads vectors for multiple records in batch.
func (s *Store) loadVectorsBatch(ctx context.Context, recordIDs []string) (map[string]map[string]store.Vector, error) {
	if len(recordIDs) == 0 {
		return make(map[string]map[string]store.Vector), nil
	}

	// Build query with IN clause
	placeholders := make([]string, len(recordIDs))
	params := make([]any, len(recordIDs))
	for i, id := range recordIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		params[i] = id
	}

	query := fmt.Sprintf(`
		SELECT record_id, name, model, vector::text
		FROM record_vectors
		WHERE record_id IN (%s)
	`, strings.Join(placeholders, ", "))

	rows, err := s.db.Query(ctx, query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]map[string]store.Vector)
	for rows.Next() {
		var recordID, name, model, vecStr string
		if err := rows.Scan(&recordID, &name, &model, &vecStr); err != nil {
			return nil, err
		}
		values, err := parseVector(vecStr)
		if err != nil {
			return nil, err
		}
		if result[recordID] == nil {
			result[recordID] = make(map[string]store.Vector)
		}
		result[recordID][name] = store.Vector{Values: values, Model: model}
	}

	return result, rows.Err()
}
