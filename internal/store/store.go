// Package store provides a generic record store with vector search, structured filtering,
// and transactional writes. It is a standalone, portable Go library.
//
// The package defines a backend-agnostic interface with a Postgres + pgvector implementation.
// Stores opaque records with primary text, named vectors, and arbitrary JSON metadata.
package store

import (
	"context"
	"errors"
	"time"
)

// Record is the unit of storage.
type Record struct {
	// ID is the unique identifier for the record.
	ID string

	// Content is the primary text content.
	Content string

	// Vectors maps vector names to their embeddings.
	// A record can have multiple named vectors (e.g., "bge", "openai").
	Vectors map[string]Vector

	// Metadata is arbitrary JSON-serializable data associated with the record.
	// Filtering and indexing are available on dotted paths (e.g., "metadata.kind").
	Metadata map[string]any

	// CreatedAt is the time the record was first stored.
	CreatedAt time.Time

	// UpdatedAt is the time the record was last modified.
	UpdatedAt time.Time

	// DeletedAt is nil for live records, non-nil for soft-deleted records.
	// Soft-deleted records are invisible to all read operations.
	DeletedAt *time.Time
}

// Vector is a named embedding.
type Vector struct {
	// Values is the embedding vector as float32 array.
	Values []float32

	// Model identifies the embedding model that produced this vector.
	// Examples: "bge-v1.5", "openai-text-embedding-3-small".
	Model string
}

// Op is a comparison operator.
type Op string

const (
	OpEq       Op = "eq"       // equals
	OpNe       Op = "ne"       // not equals
	OpGt       Op = "gt"       // greater than
	OpGte      Op = "gte"      // greater than or equal
	OpLt       Op = "lt"       // less than
	OpLte      Op = "lte"      // less than or equal
	OpIn       Op = "in"       // value is a slice
	OpNotIn    Op = "not_in"   // value is a slice
	OpExists   Op = "exists"   // value is bool
	OpContains Op = "contains" // for arrays in metadata
	OpPrefix   Op = "prefix"   // for strings
)

// Predicate is a recursive predicate tree for structured filtering.
// If And, Or, or Not is set, the leaf fields (Field, Op, Value) are ignored.
type Predicate struct {
	// Field is a dotted path: "id", "content", "metadata.kind", "created_at".
	Field string

	// Op is the comparison operator.
	Op Op

	// Value is the operand; type depends on Op.
	// For OpIn and OpNotIn, must be a slice.
	// For OpExists, must be bool.
	Value any

	// And is a logical AND of child predicates.
	And []Predicate

	// Or is a logical OR of child predicates.
	Or []Predicate

	// Not is a logical NOT applied to a single child predicate.
	Not *Predicate
}

// Order specifies ordering for List and Iterate.
type Order struct {
	// Field is a dotted path to order by.
	Field string

	// Desc sorts descending if true, ascending otherwise.
	Desc bool
}

// Filter defines a structured query for listing records.
type Filter struct {
	// Where is the predicate to match; nil matches all live records.
	Where *Predicate

	// Order specifies the sort order.
	Order []Order

	// Limit restricts the number of results; 0 uses backend default.
	// Backends enforce a hard maximum (e.g., 10000).
	Limit int

	// Offset skips the first N results.
	Offset int
}

// Query defines a search query for vector or text similarity.
type Query struct {
	// Vector is the query embedding for semantic search.
	Vector []float32

	// VectorName specifies which named vector to search against (e.g., "bge").
	VectorName string

	// Text is the query text for full-text search.
	Text string

	// Filter is an optional structured predicate to pre‑filter records.
	Filter *Predicate

	// TopK is the maximum number of results; required if Vector or Text is set.
	TopK int
}

// Store is the interface for a generic record store.
type Store interface {
	// Single-record operations

	// Put stores a record, creating or replacing it.
	Put(ctx context.Context, r Record) error

	// Get retrieves a live record by ID.
	// Returns ErrNotFound if the record is missing or soft‑deleted.
	Get(ctx context.Context, id string) (Record, error)

	// Delete soft‑deletes a record by ID.
	Delete(ctx context.Context, id string) error

	// Purge hard‑deletes a record by ID, removing it permanently.
	Purge(ctx context.Context, id string) error

	// Bulk operations

	// PutMany stores multiple records atomically.
	PutMany(ctx context.Context, rs []Record) error

	// DeleteWhere soft‑deletes all live records matching the predicate.
	// Returns the number of records deleted.
	DeleteWhere(ctx context.Context, p *Predicate) (count int64, err error)

	// Read operations

	// Search performs vector or text similarity search.
	Search(ctx context.Context, q Query) ([]Record, error)

	// List returns live records matching the filter.
	List(ctx context.Context, f Filter) ([]Record, error)

	// Iterate streams live records matching the filter via channels.
	// Returns a record channel and an error channel; both close on completion.
	// Respects ctx cancellation. Uses server‑side cursors to avoid OOM.
	Iterate(ctx context.Context, f Filter) (<-chan Record, <-chan error)

	// Count returns the number of live records matching the predicate.
	Count(ctx context.Context, p *Predicate) (int64, error)

	// Transactions

	// WithTx runs fn inside a transaction.
	// The transaction is committed if fn returns nil, rolled back on error or panic.
	// The inner Store uses the transaction; calling the outer Store from inside fn is undefined.
	WithTx(ctx context.Context, fn func(tx Store) error) error

	// Lifecycle

	// Health checks the store's connectivity and readiness.
	Health(ctx context.Context) error

	// Migrate applies any required schema migrations.
	// Safe to call on every startup; idempotent.
	Migrate(ctx context.Context) error

	// Close releases resources associated with the store.
	Close() error
}

// Sentinel errors.
var (
	ErrNotFound      = errors.New("store: record not found")
	ErrInvalidQuery  = errors.New("store: invalid query")
	ErrInvalidVector = errors.New("store: invalid vector")
	ErrTxConflict    = errors.New("store: transaction conflict")
)
