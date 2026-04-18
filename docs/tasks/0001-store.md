# Build Prompt: `pkg/store`

> a very simple, generic record store with vector search, structured filtering, and transactional writes — backed by Postgres, embeddable as a Go library.

You are an autonomous coding agent. Your job is to build the `pkg/store` package described below — nothing more, nothing less.

**Read this entire document before writing any code. Then write the README first.**

---

## Project context (read carefully)

This package lives inside a larger Go project called **Stash** at the import path:

```
github.com/alash3al/stash/pkg/store
```

The `pkg/` directory convention is deliberate: **packages under `pkg/` are reusable, standalone, and importable by any Go project — not just Stash.** `pkg/store` is *used* by Stash, but it is not *part of* Stash's identity.

**Hard rule: `pkg/store` imports nothing from anywhere else in the Stash repo.**

- It does **not** know about Stash.
- It does **not** know about other `pkg/*` packages.
- It does **not** know about the `memory`, `kernel`, or any other higher layer that will exist later.
- Its name, error messages, schema, and logs must not mention "Stash" or any domain concept from Stash.

The dependency flow is **one-way, outward-in**:

```
stash  →  pkg/memory  →  pkg/store
                              ↑
                     (nothing flows back)
```

If at any point you feel the urge to import something from outside `pkg/store/` — stop. That's the signal that your design is wrong. `pkg/store` is a leaf package. It depends only on stdlib + pgx/database-sql + pgvector bindings.

This is the same discipline as Go's own `net/http` or `database/sql`: generic primitives, ignorant of their callers, usable by anyone.

---

## 0. Spirit of the project

This package follows a specific philosophy. Internalize it before you start.

- **Tiny scope, sharp edges.** One concept, done well. If you find yourself building "while I'm here, I should also add X" — stop and put X in `TODO.md`.
- **Simple is a feature, not a side effect.** Every line of code, every type, every method must justify its existence. The smallest thing that solves the problem wins.
- **Use what already works.** Postgres + pgvector + Go's standard library. No inventing protocols. No new query languages. No frameworks.
- **Standalone.** One binary's worth of code. No CGO. No external services beyond Postgres. No background daemons.
- **Pluggable where it matters, opinionated where it doesn't.** The Store is an *interface* with one *implementation*. Future implementations are possible — but not built. Defaults are good; configuration exists for the 5%.
- **Honest names.** Call things what they are. `Store` stores. `Record` is a record. No marketing words in the codebase.
- **"What I cannot create I do not understand."** Build it from primitives you understand end-to-end. No magic.

If your instinct is to add abstraction, configuration, or features — resist. The discipline of *saying no* is the work.

---

## 1. What this is

A generic record store. Holds opaque records with arbitrary metadata and named vectors. Supports CRUD, structured filtering, semantic search, and transactions. Backed by PostgreSQL + pgvector. Embeddable as a Go library.

It is the **persistence primitive** of a larger system. A higher layer (Memory) will be built on top of it later. The Store has zero knowledge of that layer.

## 2. What this is NOT

- ❌ Not multi-tenant. No `tenant_id`. No namespaces. No isolation logic.
- ❌ Not a graph database. No traversal, no joins, no edge concepts.
- ❌ Not a domain model. No `Fact`, `Entity`, `Episode`, `Relation` types.
- ❌ Not a server. No HTTP, no CLI, no gRPC inside this package.
- ❌ Not pluggable across backends in v0.1. One backend: Postgres + pgvector.
- ❌ Not aware of embedding models. Vectors are opaque float arrays.
- ❌ Not a query engine. No aggregations beyond `Count`. No `GROUP BY`. No joins.
- ❌ Not a framework. No plugins, no hooks, no event bus, no middleware.

If you find yourself building any of the above — stop. It belongs elsewhere or in another version.

---

## 3. Core principles (non-negotiable)

These are the constitutional rules. Every decision gets checked against them.

1. **The interface is backend-agnostic.** Even though Postgres is the only implementation, the public interface must be describable without mentioning SQL, indexes, transactions-as-`*sql.Tx`, or any Postgres concept. Read every method signature and ask: *"could a non-SQL backend implement this?"* If no, redesign.

2. **Records are opaque.** A `Record` has an ID, content, named vectors, and arbitrary JSON metadata. The Store does not interpret metadata semantically — it filters and indexes paths into it.

3. **Filters are data, not strings.** No SQL fragments accepted as input. Filters are a small predicate AST. The Postgres backend translates internally.

4. **Multiple named vectors per record.** Critical. A record can have a `bge` vector and an `openai` vector simultaneously. Re-embedding is just adding a new named vector and switching reads.

5. **Soft delete by default.** Hard delete is a separate explicit operation (`Purge`).

6. **Zero domain imports. Zero parent-repo imports.** The package depends only on stdlib + pgx (or `database/sql` + driver) + pgvector's Go bindings. It does not import anything from the Stash repo — not even constants, logger setup, or helper utilities. If Stash has something useful, either copy the 5 lines you need or leave it behind.

7. **No silent magic.** No background goroutines started without the caller's knowledge. No global state. No package-level loggers. Errors are returned, not logged.

---

## 4. The data model

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
    Model  string                   // identifier of the producing model, e.g. "bge-v1.5"
}
```

That's the entire data model. Two types. If you feel the need for a third — ask first.

---

## 5. The filter AST

The filter is a small, recursive predicate tree. Backends translate it; callers never write queries.

```go
type Op string

const (
    OpEq       Op = "eq"
    OpNe       Op = "ne"
    OpGt       Op = "gt"
    OpGte      Op = "gte"
    OpLt       Op = "lt"
    OpLte      Op = "lte"
    OpIn       Op = "in"          // value is a slice
    OpNotIn    Op = "not_in"      // value is a slice
    OpExists   Op = "exists"      // value is bool
    OpContains Op = "contains"    // for arrays in metadata
    OpPrefix   Op = "prefix"      // for strings
)

// A Predicate is either a leaf (Field/Op/Value) or a composite (And/Or/Not).
// If And, Or, or Not is set, the leaf fields are ignored.
type Predicate struct {
    Field string         // dotted path: "id", "content", "metadata.kind", "created_at"
    Op    Op
    Value any

    And []Predicate
    Or  []Predicate
    Not *Predicate
}

type Order struct {
    Field string
    Desc  bool
}

type Filter struct {
    Where  *Predicate     // nil = match all live records
    Order  []Order
    Limit  int            // 0 = backend default; enforce a hard max (e.g. 10000)
    Offset int
}

type Query struct {
    Vector     []float32      // optional: semantic similarity
    VectorName string         // which named vector to search against (e.g. "bge")
    Text       string         // optional: keyword search
    Filter     *Predicate     // optional: structured pre-filter
    TopK       int            // required if Vector or Text is set
}
```

**Field path rules:**

- `id`, `content`, `created_at`, `updated_at`, `deleted_at` map to top-level columns.
- `metadata.foo.bar` walks into JSONB.
- Unknown paths return `ErrInvalidQuery`.

**Operator rules:**

- Eleven operators total. No `Regex`, `Like`, `Fuzzy`, `Geo`. Those are backend-specific traps.
- Every operator must be implementable by *any* halfway-modern store. That is the portability test.

---

## 6. The Store interface

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
    Search(ctx context.Context, q Query) ([]Record, error)
    List(ctx context.Context, f Filter) ([]Record, error)
    Iterate(ctx context.Context, f Filter) (<-chan Record, <-chan error)   // streaming
    Count(ctx context.Context, p *Predicate) (int64, error)

    // Transactions
    WithTx(ctx context.Context, fn func(tx Store) error) error

    // Lifecycle
    Health(ctx context.Context) error
    Migrate(ctx context.Context) error
    Close() error
}
```

**Eleven user-facing methods plus three lifecycle methods. That is the entire surface. Do not add to it without asking.**

### Sentinel errors

```go
var (
    ErrNotFound      = errors.New("store: record not found")
    ErrInvalidQuery  = errors.New("store: invalid query")
    ErrInvalidVector = errors.New("store: invalid vector")
    ErrTxConflict    = errors.New("store: transaction conflict")
)
```

---

## 7. Postgres backend requirements

### Schema

Two tables. No more.

**`records`:**
- `id TEXT PRIMARY KEY`
- `content TEXT`
- `metadata JSONB NOT NULL DEFAULT '{}'`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `deleted_at TIMESTAMPTZ NULL`

**`record_vectors`:**
- `record_id TEXT NOT NULL REFERENCES records(id) ON DELETE CASCADE`
- `name TEXT NOT NULL`
- `model TEXT NOT NULL`
- `vector vector(N) NOT NULL` — dimension `N` is set at construction time
- `PRIMARY KEY (record_id, name)`

If different vector dimensions per name are required, document that as a v1.0 concern. v0.1: one dimension per Store instance.

### Indexes

- HNSW on `record_vectors.vector` with cosine distance: `vector_cosine_ops`.
- GIN on `records.metadata` for general JSONB queries.
- B-tree on `records(deleted_at)`, `records(created_at)`.
- B-tree on common metadata paths configured at construction (see `Config.IndexedMetadata`).
- Generated column or expression index for full-text search on `content` (tsvector).

### Visibility rule

**Every read method silently filters `deleted_at IS NULL`.** No exceptions in v0.1. Soft-deleted records are invisible to all reads. Recovery is a future feature.

### Filter translation

- A dedicated file (e.g. `predicate.go`) translates `Predicate` → parameterized SQL `WHERE` clause.
- Field path mapping:
    - `id` → `id`
    - `content` → `content`
    - `created_at`, `updated_at`, `deleted_at` → corresponding columns
    - `metadata.foo.bar` → `metadata->'foo'->>'bar'` (use `->>` for the leaf; cast as needed by operator + value type)
- **Always parameterized.** No string interpolation of values. Ever. No exceptions.
- Unknown operators or invalid paths → `ErrInvalidQuery`.

### Search behavior

- **Vector only:** ANN search via `<=>` (cosine distance) against `record_vectors` for the named vector. `Filter` is applied as a WHERE for filter pushdown. Order by distance ascending. Limit by `TopK`.
- **Text only:** Postgres full-text search on `content` via `tsvector`. Order by `ts_rank` descending.
- **Vector + Text:** vector search with text as an additional filter on `content`. Hybrid ranking is out of scope for v0.1 — document this clearly.
- **Neither set:** `ErrInvalidQuery`. Use `List` for filter-only queries.

### Transactions

- `WithTx` opens a transaction, constructs an inner `Store` backed by the tx, passes it to `fn`. Commit on nil return, rollback on error or panic.
- All operations on the inner Store use the tx. Calls to the outer Store from inside `fn` are undefined behavior — document this.

### Iterate

- Server-side cursor or keyset pagination. Do **not** load all records into memory.
- Returns two channels: records and errors. Closes both on completion. Respects `ctx` cancellation.

### Migrations

- Embedded SQL files via `embed.FS`. Versioned in a `schema_version` table.
- `Migrate(ctx)` is **idempotent**. Safe to call on every startup.
- Forward-only in v0.1. Document this.

### Construction

```go
type Config struct {
    DSN             string         // Postgres connection string
    VectorDim       int            // dimension of vectors stored
    IndexedMetadata []string       // metadata paths to create B-tree indexes on (e.g. "kind", "entity_id")
    MaxResultSize   int            // hard cap on Limit (default 10000)
    HNSWParams      HNSWParams     // optional tuning; sane defaults if zero-value
}

func New(cfg Config) (Store, error)
```

The constructor:
1. Opens a connection pool.
2. Verifies pgvector is installed (`CREATE EXTENSION IF NOT EXISTS vector`).
3. Runs migrations.
4. Returns a ready Store.

---

## 8. Tests

- Use `testcontainers-go` to spin up Postgres + pgvector.
- A subpackage `storetest` exposes `RunSuite(t *testing.T, s Store)`. The Postgres test calls into this suite.
- Coverage at minimum:
    - `Put` / `Get` / `Delete` / `Purge` round-trips.
    - `PutMany` with 1000 records.
    - `DeleteWhere` with various predicates.
    - Every operator in isolation.
    - `And` / `Or` / `Not` composition, including nested.
    - Filter on metadata paths at depth 1, 2, 3.
    - `Search` with vector only.
    - `Search` with text only.
    - `Search` with vector + filter (verify filter pushdown).
    - `Iterate` over 10000 records — no OOM, cancellation works, error propagation works.
    - `WithTx` commit and rollback semantics.
    - Soft-delete invisibility on all read methods.
    - Concurrent `Put` on the same ID (last write wins).
    - `Health` on healthy and unhealthy connection.
- A small benchmark file: `Put`, `Get`, `Search` latency at 10k and 100k records.

---

## 9. Order of work

Build in this order. Do not skip steps. Do not parallelize.

1. **Write the package README.** Two pages. Purpose, non-goals, the interface, a usage example, the portability contract. No code yet.
2. **Define types and the interface in code.** Make it compile. Re-read it as a *user* of the package. Refine until the API feels obvious.
3. **Write the test suite skeleton.** `storetest.RunSuite` signature + scaffolding. No implementation behind it yet — just the shape.
4. **Implement the Postgres backend method by method.** After each method, the corresponding tests pass. Commit per method.
5. **Add benchmarks.** Capture numbers in `BENCHMARKS.md`.
6. **Polish.** Godoc on every exported symbol. README example must run as-is. Error messages clear.

**Step 1 is not optional.** The README is the design document. If you cannot write it crisply, the design is not ready.

---

## 10. Repository layout

The package lives at `pkg/store/` inside the Stash repo:

```
stash/
├── docs/
│   └── tasks/
│       └── 0001-store.md       ← this task
├── pkg/
│   └── store/                  ← YOU BUILD THIS
│       ├── README.md           ← write this FIRST
│       ├── BENCHMARKS.md
│       ├── TODO.md             ← park every "while I'm here" idea
│       ├── store.go            ← interface, types, sentinel errors
│       ├── predicate.go        ← Predicate AST + validation
│       ├── postgres/
│       │   ├── postgres.go     ← New, lifecycle, Health
│       │   ├── crud.go         ← Put, Get, Delete, Purge, PutMany, DeleteWhere
│       │   ├── search.go       ← Search, List, Iterate, Count
│       │   ├── tx.go           ← WithTx
│       │   ├── translate.go    ← Predicate → SQL
│       │   ├── migrations/     ← embedded .sql files
│       │   └── postgres_test.go
│       └── storetest/
│           └── suite.go        ← RunSuite(t, s)
└── go.mod
```

**Import rules (strict):**

- `pkg/store/` (root) defines the interface, types, predicate AST, sentinel errors. Depends only on stdlib.
- `pkg/store/postgres/` implements the interface. Imports `pkg/store` + pgx/database-sql + pgvector. Nothing else.
- `pkg/store/storetest/` is the shared test suite. Imports `pkg/store` + `testing`.
- **Nothing in `pkg/store/` imports anything from `pkg/` or from the Stash root.** Ever.
- The public API is everything exported from `pkg/store/`. Everything under `pkg/store/postgres/` is an implementation detail — callers use `store.New(...)` which returns a `store.Store`, not a `*postgres.Store`.

One-way dependency. Always. If you need to violate this, ask first.

---

## 11. Definition of done

- All exported symbols have godoc comments.
- `README.md` is complete and the example code in it actually runs.
- All tests in `storetest.RunSuite` pass against the Postgres backend.
- Benchmarks exist; numbers committed in `BENCHMARKS.md`.
- No `TODO` comments in shipped code (all TODOs go in `TODO.md`).
- No SQL fragments accepted from callers anywhere in the public API.
- `go vet ./...` clean. `staticcheck ./...` clean.
- A new user can `go get` the package, copy the README example, and have a working Store running in under 5 minutes.

---

## 12. Things to ask before starting

If anything is ambiguous, **ask before writing code**. Specifically you may want to clarify:

- pgx directly vs `database/sql` with a pgx driver.
- One vector dimension per Store vs per record (v0.1 is per Store, but confirm).
- `Iterate` implementation: server-side cursor vs keyset pagination.
- Anything in the filter AST translation that seems underspecified.

**Do not invent answers silently. Ask.**

---

## 13. Things to never do

- ❌ Never add a method, type, or feature not specified above without asking.
- ❌ Never import anything from the Stash repo outside `pkg/store/`. Not even "just one helper."
- ❌ Never mention "Stash", "Memory", "Kernel", "Fact", "Entity", "Episode", or any higher-layer concept in code, comments, errors, logs, schema, or docs inside this package.
- ❌ Never accept raw SQL from callers.
- ❌ Never leak `*sql.Tx`, `*pgx.Conn`, or any Postgres type through the public API.
- ❌ Never add domain types (`Fact`, `Entity`, `Episode`, `Relation`) — those belong in another package.
- ❌ Never silently swallow errors. Wrap with context: `fmt.Errorf("store: ...: %w", err)`.
- ❌ Never `panic` for runtime errors. Return errors.
- ❌ Never log from inside the package. Return errors; let the caller decide.
- ❌ Never start background goroutines the caller didn't explicitly start.
- ❌ Never use `interface{}`/`any` where a concrete type works. Reserve `any` for genuinely untyped data (metadata values).
- ❌ Never add a configuration knob without a real use case driving it.

---

## 14. The mantra

> **A store should be the most boring component in the system.**

Boring means: predictable, well-understood, low surprise, easy to reason about. The exciting work happens in the layers above. The store's job is to disappear into the background and just work.

Every "cool feature" you add is a future bug, a future migration, a future thing that confuses the next reader (which might be you in six months).

**One more principle to hold close:** this package is a *library*, not a *module of Stash*. Build it as if a stranger on the other side of the world will `go get` it tomorrow and use it for something you've never imagined. That discipline — writing for an unknown caller — is what keeps the abstraction honest.

When in doubt: **do less.**

---

**Begin by writing `README.md`. Show it to me before writing any Go code.**