package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/alash3al/stash/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Search performs vector or text similarity search.
func (s *Store) Search(ctx context.Context, q store.Query) ([]store.Record, error) {
	if err := s.validateQuery(q); err != nil {
		return nil, err
	}

	// Neither set: use List for filter-only queries
	if q.Vector == nil && q.Text == "" {
		return nil, store.ErrInvalidQuery
	}

	// Handle vector search (may include text as additional filter)
	if q.Vector != nil {
		return s.searchVector(ctx, q)
	}

	// Text-only search
	return s.searchText(ctx, q)
}

// searchVector performs vector similarity search.
// If q.Text is also set, adds a full-text search filter (hybrid search).
func (s *Store) searchVector(ctx context.Context, q store.Query) ([]store.Record, error) {
	whereParts := []string{"deleted_at IS NULL"}
	var params []any
	currentParam := 1

	// Add filter predicate if present
	if q.Filter != nil {
		filterWhere, filterParams, err := translatePredicateWithoutDeleted(q.Filter, currentParam)
		if err != nil {
			return nil, err
		}
		if filterWhere != "" {
			whereParts = append(whereParts, filterWhere)
			params = append(params, filterParams...)
			currentParam += len(filterParams)
		}
	}

	// Add text search filter if present (hybrid vector+text search)
	if q.Text != "" {
		whereParts = append(whereParts, fmt.Sprintf("to_tsvector('english', r.content) @@ plainto_tsquery('english', $%d)", currentParam))
		params = append(params, q.Text)
		currentParam++
	}

	// Add vector name filter
	whereParts = append(whereParts, fmt.Sprintf("v.name = $%d", currentParam))
	params = append(params, q.VectorName)
	currentParam++

	whereClause := "WHERE " + strings.Join(whereParts, " AND ")

	// Add query vector
	params = append(params, q.Vector)
	queryParam := currentParam

	sql := fmt.Sprintf(`
		SELECT r.id, r.content, r.metadata, r.created_at, r.updated_at
		FROM records r
		INNER JOIN record_vectors v ON r.id = v.record_id
		%s
		ORDER BY v.vector <=> $%d
		%s
	`, whereClause, queryParam, s.buildLimitOffset(q.TopK, 0))

	rows, err := s.db.Query(ctx, sql, params...)
	if err != nil {
		return nil, fmt.Errorf("postgres: vector search: %w", err)
	}
	defer rows.Close()

	return scanRecords(ctx, s, rows)
}

// searchText performs full‑text search.
func (s *Store) searchText(ctx context.Context, q store.Query) ([]store.Record, error) {
	sql, params, err := s.queryToSQL(q, 1)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(ctx, sql, params...)
	if err != nil {
		return nil, fmt.Errorf("postgres: text search: %w", err)
	}
	defer rows.Close()

	return scanRecords(ctx, s, rows)
}

// List returns live records matching the filter.
func (s *Store) List(ctx context.Context, f store.Filter) ([]store.Record, error) {
	where, params, err := translatePredicate(f.Where, 1)
	if err != nil {
		return nil, err
	}

	orderBy, err := translateOrder(f.Order)
	if err != nil {
		return nil, err
	}

	sql := fmt.Sprintf(`
		SELECT id, content, metadata, created_at, updated_at
		FROM records
		WHERE %s
		%s
		%s
	`, where, orderBy, s.buildLimitOffset(f.Limit, f.Offset))

	rows, err := s.db.Query(ctx, sql, params...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list: %w", err)
	}
	defer rows.Close()

	return scanRecords(ctx, s, rows)
}

// Iterate streams live records matching the filter via channels.
// Uses server-side cursors for pool-backed stores and pagination for tx-backed stores.
func (s *Store) Iterate(ctx context.Context, f store.Filter) (<-chan store.Record, <-chan error) {
	if _, isPool := s.db.(*pgxpool.Pool); isPool {
		return s.iterateWithCursor(ctx, f)
	}
	return s.iterateWithPagination(ctx, f)
}

// iterateWithCursor uses PostgreSQL cursors for efficient streaming.
func (s *Store) iterateWithCursor(ctx context.Context, f store.Filter) (<-chan store.Record, <-chan error) {
	recordCh := make(chan store.Record)
	errCh := make(chan error, 1)

	go func() {
		defer close(recordCh)
		defer close(errCh)

		where, params, err := translatePredicate(f.Where, 1)
		if err != nil {
			errCh <- err
			return
		}

		orderBy, err := translateOrder(f.Order)
		if err != nil {
			errCh <- err
			return
		}

		// Deterministic ordering is required for cursor and pagination correctness.
		// _row_id (BIGSERIAL identity column) is monotonically increasing and
		// faster to sort than TEXT id.
		if orderBy == "" {
			orderBy = "ORDER BY _row_id"
		}

		// Use a cursor for iteration
		cursorName := safeCursorName("store_iter_")

		// Declare cursor WITH HOLD so it survives transaction
		// Note: cursor names cannot be parameterized, must use string formatting
		// but safeCursorName() ensures safe identifier
		declareSQL := fmt.Sprintf(`DECLARE "%s" CURSOR WITH HOLD FOR
			SELECT id, content, metadata, created_at, updated_at
			FROM records
			WHERE %s
			%s`, cursorName, where, orderBy)

		_, err = s.db.Exec(ctx, declareSQL, params...)
		if err != nil {
			errCh <- fmt.Errorf("postgres: declare cursor: %w", err)
			return
		}

		// Ensure cursor is closed on exit
		cursorClosed := false
		closeCursor := func() {
			if !cursorClosed {
				cursorClosed = true
				closeSQL := fmt.Sprintf(`CLOSE "%s"`, cursorName)
				if _, closeErr := s.db.Exec(ctx, closeSQL); closeErr != nil {
					// Try to send close error if channel not full
					select {
					case errCh <- fmt.Errorf("postgres: close cursor: %w", closeErr):
					default:
						// Error channel already has an error
					}
				}
			}
		}
		defer closeCursor()

		batchSize := 100
		if f.Limit > 0 && f.Limit < batchSize {
			batchSize = f.Limit
		}

		for {
			// Check context before each operation
			select {
			case <-ctx.Done():
				closeCursor()
				errCh <- ctx.Err()
				return
			default:
			}

			// Use a separate context for the query with timeout
			// If caller's ctx has a deadline, use it; otherwise use no timeout
			queryCtx, queryCancel := context.WithCancel(ctx)
			fetchSQL := fmt.Sprintf(`FETCH %d FROM "%s"`, batchSize, cursorName)
			rows, err := s.db.Query(queryCtx, fetchSQL)
			queryCancel()

			if err != nil {
				closeCursor()
				errCh <- fmt.Errorf("postgres: fetch from cursor: %w", err)
				return
			}

			records, err := scanRecords(ctx, s, rows)
			rows.Close()
			if err != nil {
				closeCursor()
				errCh <- err
				return
			}

			if len(records) == 0 {
				return // No more rows
			}

			// Send records with context check for each
			sentCount := 0
			for _, r := range records {
				select {
				case <-ctx.Done():
					rows.Close()
					closeCursor()
					errCh <- ctx.Err()
					return
				case recordCh <- r:
					sentCount++
				}
			}

			if f.Limit > 0 && (sentCount < len(records) || sentCount >= f.Limit) {
				// Either couldn't send all records (context) or hit limit
				return
			}
			if len(records) < batchSize {
				return // Partial batch means we're done
			}
		}
	}()

	return recordCh, errCh
}

// iterateWithPagination uses OFFSET-based pagination for tx-backed stores.
func (s *Store) iterateWithPagination(ctx context.Context, f store.Filter) (<-chan store.Record, <-chan error) {
	recordCh := make(chan store.Record)
	errCh := make(chan error, 1)

	go func() {
		defer close(recordCh)
		defer close(errCh)

		offset := f.Offset
		batchSize := 100
		if f.Limit > 0 && f.Limit < batchSize {
			batchSize = f.Limit
		}

		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			default:
			}

			where, params, err := translatePredicate(f.Where, 1)
			if err != nil {
				errCh <- err
				return
			}

			orderBy, err := translateOrder(f.Order)
			if err != nil {
				errCh <- err
				return
			}

			if orderBy == "" {
				orderBy = "ORDER BY _row_id"
			}

			limit := batchSize
			if s.cfg.MaxResultSize > 0 && limit > s.cfg.MaxResultSize {
				limit = s.cfg.MaxResultSize
			}

			sql := fmt.Sprintf(`
				SELECT id, content, metadata, created_at, updated_at
				FROM records
				WHERE %s
				%s
				LIMIT %d OFFSET %d
			`, where, orderBy, limit, offset)

			rows, err := s.db.Query(ctx, sql, params...)
			if err != nil {
				errCh <- fmt.Errorf("postgres: iterate batch: %w", err)
				return
			}

			records, err := scanRecords(ctx, s, rows)
			rows.Close()
			if err != nil {
				errCh <- err
				return
			}

			if len(records) == 0 {
				return
			}

			for _, r := range records {
				select {
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				case recordCh <- r:
				}
			}

			offset += len(records)
			if f.Limit > 0 && offset >= f.Limit {
				return
			}
			if len(records) < batchSize {
				return
			}
		}
	}()

	return recordCh, errCh
}

// Count returns the number of live records matching the predicate.
func (s *Store) Count(ctx context.Context, p *store.Predicate) (int64, error) {
	where, params, err := translatePredicate(p, 1)
	if err != nil {
		return 0, err
	}

	var count int64
	err = s.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*)
		FROM records
		WHERE %s
	`, where), params...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("postgres: count: %w", err)
	}

	return count, nil
}

// DeleteWhere soft‑deletes all live records matching the predicate.
func (s *Store) DeleteWhere(ctx context.Context, p *store.Predicate) (int64, error) {
	if p == nil {
		// Optimize common case: delete all live records
		sql := `
			UPDATE records
			SET deleted_at = NOW()
			WHERE deleted_at IS NULL
		`
		tag, err := s.db.Exec(ctx, sql)
		if err != nil {
			return 0, fmt.Errorf("postgres: delete where all: %w", err)
		}
		return tag.RowsAffected(), nil
	}

	// Get predicate SQL without the deleted_at IS NULL part
	where, params, err := translatePredicateForDelete(p, 1)
	if err != nil {
		return 0, err
	}

	sql := fmt.Sprintf(`
		UPDATE records
		SET deleted_at = NOW()
		WHERE %s AND deleted_at IS NULL
	`, where)

	tag, err := s.db.Exec(ctx, sql, params...)
	if err != nil {
		return 0, fmt.Errorf("postgres: delete where: %w", err)
	}

	return tag.RowsAffected(), nil
}

// translatePredicateForDelete translates predicate for DELETE WHERE (no auto-added deleted_at IS NULL).
func translatePredicateForDelete(p *store.Predicate, startParam int) (where string, params []any, err error) {
	if p == nil {
		return "TRUE", nil, nil // Match all live records
	}

	where, params, err = translatePredicateRec(p, startParam)
	if err != nil {
		return "", nil, err
	}

	if where == "" {
		return "TRUE", nil, nil
	}
	return where, params, nil
}

// scanRecords converts query rows to store.Record slices.
func scanRecords(ctx context.Context, s *Store, rows pgx.Rows) ([]store.Record, error) {
	var records []store.Record
	var recordIDs []string

	for rows.Next() {
		var r store.Record
		var metadata map[string]any

		if err := rows.Scan(&r.ID, &r.Content, &metadata, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}

		r.Metadata = metadata
		records = append(records, r)
		recordIDs = append(recordIDs, r.ID)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Batch load all vectors
	if len(recordIDs) > 0 {
		vectorsByRecord, err := s.loadVectorsBatch(ctx, recordIDs)
		if err != nil {
			return nil, err
		}
		for i := range records {
			records[i].Vectors = vectorsByRecord[records[i].ID]
		}
	}

	return records, nil
}

// safeCursorName generates a safe SQL identifier for cursor names.
func safeCursorName(prefix string) string {
	// Use UUID for uniqueness - no collision possible
	id := uuid.New().String()
	// Replace hyphens with underscores and prefix
	return prefix + strings.ReplaceAll(id, "-", "_")
}
