# Task: Build internal/memory - The AGI Memory Layer

**Status:** Active  
**Date:** 2026-04-18

## 1. Context
- **Goal:** Build the memory layer that turns LLMs from stateless predictors into systems with persistent, intelligent memory.
- **Why:** Current LLMs are "dummy" because they lack memory. By adding episodic and working memory with cognitive processes, we enable AGI-like behavior. Memory is the missing piece for true intelligence.

## 2. Boundaries
- **In Scope:**
  - Episodic memory (events with temporal relationships) - **MVP**
  - Working memory (active context management) - **MVP**
  - Storage-agnostic design using only `store.Store` interface
  - Metadata-only storage (no schema changes, no new tables)
  - Simple 3-5 method interface for MVP

- **Non-Goals:**
  - LLM integration (memory layer is LLM-agnostic)
  - Agent framework (memory is a primitive, not a full agent system)
  - Multi-tenant isolation (single memory space for now)
  - Schema design or database extensions (use metadata only)
  - PostgreSQL-specific code (storage-agnostic)
  - Real-time streaming or pub/sub
  - Semantic memory (concepts and facts) - Phase 3
  - Procedural memory (skills) - Phase 4
  - Graph relationships between memories - Phase 4
  - Cognitive processes (consolidation, forgetting) - Phase 2

- **Constraints:**
  - Must depend only on `store.Store` interface from `internal/store` (Rule 3.9)
  - Must store everything in `Record.Metadata` (Rule 3.7)
  - Must compose tools, not extend interfaces (Rule 3.8)
  - Must follow Unix philosophy: do one thing well, compose tools (Rule 11.6)
  - No background goroutines without explicit caller control (Rule 3.4)
  - No global state (Rule 3.3)
  - Return errors, do not log (Rule 3.5)
  - No `panic` for runtime errors (Rule 3.6)

- **Dependencies:**
  - **Required**: `internal/store` (no rule override needed)
  - **Required**: OpenAI SDK for embeddings (`github.com/sashabaranov/go-openai`)
  - **Required**: OpenAI API key for meaningful semantic search
  - **No CGO** (pure Go only, Rule 4.3)

## 3. Approach & Review

### Proposed Architecture
```
internal/
├── embedder/              # NEW: OpenAI-compatible embedding service (product-specific)
│   └── embedder.go        # Concrete Embedder type (no interface, API key required)
└── memory/                # Pure business logic, storage-agnostic
    ├── memory.go          # Core interface (3 methods)
    ├── types.go           # Shared types (Event, Context)
    ├── impl/              # Single implementation
    │   ├── memory.go      # Main implementation (composes store + embedder)
    │   └── impl_test.go   # Integration tests
    └── memory_test.go     # Unit tests
```

**Key Design Points:**
1. **`internal/embedder`** - Product-specific, not reusable library
2. **Concrete type** - No interface, OpenAI-only implementation
3. **API key required** - No mock, real embeddings from day one
4. **Memory composes** - Depends on `*embedder.Embedder` and `store.Store`
5. **Metadata-only** - All memory data stored in `Record.Metadata`

### Unix Philosophy Applied
- **Store** = filesystem/io.Reader (storage primitive)
- **Memory** = awk/sed (intelligence layer)
- **Future tools** (graph, index, cache) = grep/join (compose with store)

### Storage Strategy (Metadata-Only)
All memory data stored in `Record.Metadata`:
```go
// Event stored as Record with metadata.type="event"
metadata := map[string]any{
    "type": "event",
    "content": "User asked about weather",
    "timestamp": "2024-01-01T10:00:00Z",
    "importance": 0.7,
    "accessed_at": "2024-01-01T10:00:00Z",
}

// Working context stored as Record with metadata.type="context"
metadata := map[string]any{
    "type": "context",
    "focus": "weather conversation",
    "event_ids": []string{"event-1", "event-2"},
    "created_at": "2024-01-01T10:00:00Z",
    "expires_at": "2024-01-01T11:00:00Z",
}
```

**No schema changes.** **No new tables.** **Storage-agnostic.**

### Core Data Models (MVP Only)
```go
// Event represents an episodic memory - something that happened at a specific time.
type Event struct {
    ID         string
    Content    string                 // What happened (text description)
    Timestamp  time.Time              // When it happened
    Metadata   map[string]any         // Additional structured context
    // Note: Embeddings stored separately via store.Vectors
    // Note: Importance, access patterns stored in metadata
}

// Context represents working memory state - what's actively being thought about.
type Context struct {
    ID         string
    Focus      string                 // Current topic/query
    EventIDs   []string               // Event IDs in working memory
    CreatedAt  time.Time
    UpdatedAt  time.Time
    ExpiresAt  time.Time              // Working memory has limited duration
}

// Vector is re-exported from store for embedding operations.
type Vector = store.Vector
```

### Memory Interface (MVP - 3 Methods)
```go
// Memory is the core interface for AGI-like memory systems.
// MVP focuses on episodic memory + working memory (3 methods).
type Memory interface {
    // Remember: Store an event with embedding
    Remember(ctx context.Context, content string, metadata map[string]any) (string, error)
    
    // Recall: Retrieve relevant events for a query
    Recall(ctx context.Context, query string, limit int) ([]Event, error)
    
    // Context: Update and get working memory context
    Context(ctx context.Context, input string) (Context, error)
    
    // Lifecycle
    Close() error
}
```

**Why 3 methods?**
1. **Remember** - Stores events with OpenAI embeddings (episodic memory)
2. **Recall** - Retrieves relevant events via vector search + temporal filtering
3. **Context** - Manages working memory state (active thinking)

**Embedding Strategy:**
- Uses `internal/embedder` (concrete type, no interface)
- OpenAI-compatible API only (industry standard)
- API key required, no mock for development
- Embeddings stored as `store.Vector` with model name as key

**No separate interfaces.** **No cognitive methods in MVP.** **Simple, focused, composable.**

### Self-Critique
**Strengths:**
1. **Storage-agnostic**: Pure business logic, no storage code (Rule 3.9)
2. **Metadata-only**: No schema changes, uses existing store (Rule 3.7)
3. **Simple interface**: 3 methods for MVP, easy to understand
4. **Unix philosophy**: Composes with store + embedder, doesn't extend interfaces (Rule 3.8)
5. **LLM-agnostic**: Works with any model, not tied to specific LLM
6. **Embedding simplicity**: Concrete `embedder.Embedder` type, no interface abstraction

**Risks:**
1. **Embedding dependency**: Requires OpenAI API key from day one
2. **Performance**: Vector search via store may need optimization
3. **Working memory persistence**: Need to decide storage strategy
4. **API cost**: Real embeddings cost money even during development

**Tradeoffs considered:**
- **Option A**: Full cognitive memory with schema extensions (rejected - violates rules)
- **Option B**: Storage-agnostic with metadata-only (chosen - aligns with rules)
- **Option C**: Mock embeddings for development (rejected - meaningless memory)
- **Option D**: Embedder interface abstraction (rejected - YAGNI, OpenAI protocol is standard)

**Architecture validation:**
1. **Why `internal/memory`?** Memory is Stash-specific intelligence layer
2. **Why storage-agnostic?** Follows AGENTS.md Rule 3.9 (higher layers are storage-agnostic)
3. **Why metadata-only?** Follows AGENTS.md Rule 3.7 (store everything in metadata)
4. **Why 3 methods?** MVP focus, do less, ship faster
5. **Why concrete embedder?** OpenAI protocol is standard, no need for abstraction
6. **Why no mock?** Real embeddings required for meaningful semantic memory

### Decision
Build **storage-agnostic memory with 3-method interface and concrete embedder** because:
1. **Rule-compliant**: Follows all AGENTS.md architectural rules
2. **Simple**: 3 methods, concrete dependencies, no abstraction overhead
3. **Composable**: Works with any store + OpenAI-compatible embedder
4. **Real from day one**: Meaningful semantic search requires real embeddings
5. **Unix philosophy**: Memory (intelligence) + Store (storage) + Embedder (text→vector) compose

## 4. Execution Steps

### Phase 0: Dependencies & Setup (0.5 day)
- [ ] Add OpenAI SDK to `go.mod`: `github.com/sashabaranov/go-openai`
- [ ] Create `internal/embedder/` package with concrete `Embedder` type
- [ ] Implement `embedder.New()` with OpenAI client
- [ ] Write `embedder_test.go` with integration tests (requires API key)

### Phase 1: Core Types & Interface (0.5 day)
- [ ] Create `internal/memory/types.go` with Event, Context structs
- [ ] Create `internal/memory/memory.go` with 3-method Memory interface
- [ ] Create `internal/memory/errors.go` with sentinel errors
- [ ] Write `internal/memory/README.md` with architecture

### Phase 2: Single Implementation (1.5 days)
- [ ] Create `internal/memory/impl/memory.go` main implementation
- [ ] Implement `Remember`: Store event with OpenAI embedding
- [ ] Implement `Recall`: Search events via store vectors + temporal filtering
- [ ] Implement `Context`: Manage working memory state
- [ ] Add configuration struct and `New()` constructor
- [ ] Write `internal/memory/impl/impl_test.go` integration tests

### Phase 3: Testing & Examples (0.5 day)
- [ ] Write unit tests for all methods (using test doubles)
- [ ] Create `example_chatbot.go` showing memory usage
- [ ] Write comprehensive godoc comments
- [ ] Update project README with memory package
- [ ] Verify compilation and tests pass

**Total estimated: 3 days of focused work**

### Verification Criteria
Each phase must pass:
1. **Compiles cleanly**: `go build ./internal/...`
2. **Tests pass**: `go test ./internal/...` (integration tests may need API key)
3. **No lint errors**: `go vet ./internal/...`
4. **Follows AGENTS.md**: No rule violations
5. **Storage-agnostic**: No PostgreSQL-specific code in memory
6. **Metadata-only**: No schema changes, uses Record.Metadata
7. **Concrete dependencies**: Memory depends on `*embedder.Embedder`, not interface
8. **Documented**: Godoc comments on all exported symbols

### Verification Criteria
Each phase must pass:
1. **Compiles cleanly**: `go build ./internal/memory/...`
2. **Tests pass**: `go test ./internal/memory/...`
3. **No lint errors**: `go vet ./internal/memory/...`
4. **Follows AGENTS.md**: No rule violations
5. **Storage-agnostic**: No PostgreSQL-specific code
6. **Metadata-only**: No schema changes, uses Record.Metadata
7. **Documented**: Godoc comments on all exported symbols

## 5. Progress Notes
- [2026-04-18] - Task created based on research and architectural planning
- [2026-04-18] - Task refined to align with AGENTS.md rules: storage-agnostic, metadata-only, 3-method interface
- [2026-04-18] - Design decisions: concrete embedder (no interface), OpenAI-only, no mock

## 6. Outcome
- **Final Result:** A storage-agnostic memory layer with 3 methods that enables LLMs to remember and recall events with working memory context.

- **Success Metrics:**
  1. **Functional**: 
     - `Remember` stores events with OpenAI embeddings
     - `Recall` retrieves relevant events via vector search + temporal filtering
     - `Context` manages working memory state
  2. **Architectural**:
     - Storage-agnostic (uses only `store.Store` interface)
     - Metadata-only (no schema changes)
     - Composable (follows Unix philosophy)
     - Concrete dependencies (`*embedder.Embedder`, no interface)
  3. **Performance** (with real embeddings):
     - `Remember`: < 100ms (including OpenAI API call)
     - `Recall`: < 50ms for 10 events (store vector search)
     - `Context`: < 20ms
  4. **Reliability**: 
     - Transactional via store
     - Handles concurrent access safely
     - Persistent storage via store
     - Proper error handling for OpenAI API failures
  5. **Usability**:
     - Clear 3-method API
     - Good documentation with examples
     - Works with any LLM (agnostic)
     - Requires OpenAI API key (meaningful semantic search)

- **Definition of Done:**
  1. 3-method interface implemented and tested
  2. Storage-agnostic implementation using only `store.Store`
  3. Metadata-only storage (no schema changes)
  4. Example chatbot demonstrates memory usage
  5. Documentation complete (godoc + README)