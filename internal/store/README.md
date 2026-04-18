# internal/store

A generic record store with vector search, structured filtering, and transactional writes — backed by Postgres, embeddable as a Go library.

## Purpose

`internal/store` is the Stash persistence layer for storing and retrieving records with:

1. **Content**: Primary text
2. **Named vectors**: Multiple embeddings per record (e.g., "bge", "openai")  
3. **Arbitrary metadata**: JSON-serializable data with indexed filtering
4. **Vector search**: Semantic similarity search via pgvector
5. **Structured filtering**: Type-safe predicate API, not raw SQL

It's designed as a **persistence primitive** — generic, unaware of its callers, usable by any Go project.

## Non-goals

This package is NOT:

- ❌ Multi-tenant or namespaced
- ❌ A graph database (no traversal, edges, joins)
- ❌ A domain model store (no `Fact`, `Entity`, `Episode` types)
- ❌ A server (no HTTP, CLI, gRPC)
- ❌ Multi-backend pluggable in v0.1 (Postgres + pgvector only)
- ❌ Aware of embedding models (vectors are opaque)
- ❌ A query engine (no aggregates, no `GROUP BY`)
- ❌ A framework (no plugins, hooks, middleware)

The store's job is to disappear into the background and just work.

## Data model

```go
type Record struct {
    ID        string
    Content   string                // primary text
    Vectors   map[string]Vector     // named vectors, e.g. "bge" -> Vector{...}
    Metadata  map[string]any        // arbitrary JSON-serializable data
    CreatedAt time.Time
    UpdatedAt time.Time
    DeletedAt *time.Time            // nil = live, non-nil = soft-deleted
}

type Vector struct {
    Values []float32
    Model  string                   // identifier, e.g. "bge-v1.5"
}
```

## Filter AST

No SQL strings. Use the predicate API:

```go
type Filter struct {
    Where  *Predicate     // nil = match all live records
    Order  []Order
    Limit  int            // 0 = backend default; max enforced
    Offset int
}

// Predicate is a recursive AST
type Predicate struct {
    Field string         // dotted path: "id", "metadata.kind", "created_at"
    Op    Op             // eq, ne, gt, lt, in, not_in, exists, contains, prefix
    Value any

    And []Predicate      // logical AND
    Or  []Predicate      // logical OR  
    Not *Predicate       // logical NOT
}

type Query struct {
    Vector     []float32      // optional: semantic similarity
    VectorName string         // which named vector to search against
    Text       string         // optional: keyword search  
    Filter     *Predicate     // optional: structured pre-filter
    TopK       int            // required if Vector or Text is set
}
```

Eleven operators, all implementable by any modern datastore.

## Store interface

```go
type Store interface {
    // Single-record operations
    Put(ctx context.Context, r Record) error
    Get(ctx context.Context, id string) (Record, error)        // ErrNotFound if missing or soft-deleted
    Delete(ctx context.Context, id string) error               // soft delete
    Purge(ctx context.Context, id string) error                // hard delete

    // Bulk operations
    PutMany(ctx context.Context, rs []Record) error
    DeleteWhere(ctx context.Context, p *Predicate) (count int64, err error)

    // Read operations
    Search(ctx context.Context, q Query) ([]Record, error)     // vector/text search
    List(ctx context.Context, f Filter) ([]Record, error)      // filtered listing
    Iterate(ctx context.Context, f Filter) (<-chan Record, <-chan error) // streaming
    Count(ctx context.Context, p *Predicate) (int64, error)

    // Transactions
    WithTx(ctx context.Context, fn func(tx Store) error) error

    // Lifecycle
    Health(ctx context.Context) error
    Migrate(ctx context.Context) error
    Close() error
}
```

**Eleven user-facing methods plus three lifecycle methods.**

### Sentinel errors

```go
var (
    ErrNotFound      = errors.New("store: record not found")
    ErrInvalidQuery  = errors.New("store: invalid query")
    ErrInvalidVector = errors.New("store: invalid vector")
    ErrTxConflict    = errors.New("store: transaction conflict")
)
```

## Postgres backend

The v0.1 implementation uses PostgreSQL + pgvector with:

**Tables:**
- `records`: ID, content, metadata (JSONB), timestamps, deleted_at
- `record_vectors`: record_id, name, model, vector (vector(N))

**Indexes:**
- HNSW on vectors with cosine distance
- GIN on metadata for JSONB queries  
- B-tree on timestamps and deleted_at
- Expression index for full-text search on content

**Features:**
- Soft delete by default (`deleted_at IS NULL` filter on all reads)
- Named vectors: multiple embeddings per record
- Filter pushdown: vector/text search with structured WHERE clauses
- Server-side cursors for iteration (no OOM on large datasets)

### Construction

```go
type Config struct {
    DSN             string         // Postgres connection string
    VectorDim       int            // dimension of all vectors stored
    IndexedMetadata []string       // metadata paths to index (e.g. "kind", "entity_id")
    MaxResultSize   int            // hard cap on Limit (default 10000)
    HNSWParams      HNSWParams     // optional HNSW tuning
}

func New(cfg Config) (Store, error)
```

`New()` opens a pool, verifies pgvector, runs migrations, returns a ready Store.

## Usage example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "github.com/alash3al/stash/internal/store"
)

func main() {
    ctx := context.Background()

    cfg := store.Config{
        DSN:       "postgres://localhost/stash?sslmode=disable",
        VectorDim: 384,
        IndexedMetadata: []string{"category", "source"},
        MaxResultSize: 1000,
    }

    s, err := store.New(cfg)
    if err != nil {
        log.Fatal(err)
    }
    defer s.Close()

    // Create a record
    r := store.Record{
        ID:      "123",
        Content: "The quick brown fox jumps over the lazy dog",
        Vectors: map[string]store.Vector{
            "bge": {Values: []float32{0.1, 0.2, /* ... */}, Model: "bge-v1.5"},
        },
        Metadata: map[string]any{"category": "example", "lang": "en"},
    }

    // Store it
    if err := s.Put(ctx, r); err != nil {
        log.Fatal(err)
    }

    // Retrieve
    got, err := s.Get(ctx, "123")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Got: %q\n", got.Content)

    // Search semantically
    queryVec := []float32{0.2, 0.3, /* ... */ 384 values}
    results, err := s.Search(ctx, store.Query{
        Vector:     queryVec,
        VectorName: "bge",
        TopK:       10,
    })
    if err != nil {
        log.Fatal(err)
    }

    // Filter
    filtered, err := s.List(ctx, store.Filter{
        Where: &store.Predicate{
            Field: "metadata.category",
            Op:    store.OpEq,
            Value: "example",
        },
        Limit: 100,
    })

    // Transaction
    err = s.WithTx(ctx, func(tx store.Store) error {
        // Use tx for atomic operations
        return tx.Put(ctx, store.Record{ID: "456", Content: "Another"})
    })
}
```

## Portability contract

The interface is backend-agnostic. While v0.1 is Postgres-only, the API is designed so alternative backends could implement it without changes:

- No SQL fragments accepted anywhere
- No Postgres-specific types in the public API (`*sql.Tx`, `*pgx.Conn`, etc.)
- No pgvector distance functions named in method signatures
- Filter AST uses only operators implementable by any modern datastore

If it can't be described without mentioning SQL, it doesn't belong in the interface.

## Installation

```bash
go get github.com/alash3al/stash/internal/store
```

Requires:
- Go 1.21+
- PostgreSQL 13+ with pgvector extension
- `database/sql` driver (compatible with pgx or lib/pq)

## Testing

Run the shared test suite:

```bash
cd internal/store
go test ./...
```

Integration tests use testcontainers-go to spin up Postgres + pgvector.

## License

BSD-3-Clause

## Contributing

See CONTRIBUTING.md (if present). Follow the package philosophy: **do less.**