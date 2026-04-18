package postgres

import (
	"context"
	"fmt"

	"github.com/alash3al/stash/internal/store"
)

// WithTx runs fn inside a transaction.
// The transaction is committed if fn returns nil, rolled back on error or panic.
// The inner Store uses the transaction for all operations.
// Calling Health, Migrate, or Close on the transaction-backed Store returns an error.
func (s *Store) WithTx(ctx context.Context, fn func(tx store.Store) error) error {
	if s.pool == nil {
		return fmt.Errorf("postgres: WithTx not available in transaction")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin tx: %w", err)
	}

	// Create a transaction-backed Store sharing the same config
	txStore := &Store{db: tx, cfg: s.cfg} // pool is nil

	// Handle panic: rollback on panic, then re-panic
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p) // re-panic after rollback
		}
	}()

	err = fn(txStore)
	if err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("postgres: rollback after error (%v): %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit: %w", err)
	}

	return nil
}
