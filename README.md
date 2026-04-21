# Stash

**Stash gives stateless AI models persistent, verifiable memory.**

A self-hosted, single-user memory layer for AI systems. It sits between the model and the world, giving any LLM:

- **Persistent episodic memory** — things that happened, when they happened
- **Working context** — what's actively being thought about right now
- **Semantic retrieval** — find relevant memories by meaning, not just keywords
- **Grounding** — the model can only answer from what's in the store

> **Core insight:** Weights hold skills. Storage holds facts.

---

## Quick Start

### 1. Prerequisites
- Go 1.21+
- PostgreSQL 13+ with [pgvector](https://github.com/pgvector/pgvector) extension
- OpenAI-compatible embedding endpoint (OpenAI, OpenRouter, Ollama, etc.)

### 2. Install
```bash
git clone https://github.com/alash3al/stash
cd stash
go build -o stash ./cmd/cli
```

### 3. Configure
Copy `.env.example` to `.env` and fill in your values:
```bash
cp .env.example .env
```

Edit `.env`:
```env
# Store
STASH_STORE_DRIVER=postgres
STASH_STORE_DSN=postgres://user:password@localhost:5432/stash?sslmode=disable
STASH_VECTOR_DIM=1536  # must match your embedder model

# Embedder
STASH_EMBEDDER_DRIVER=openai
STASH_OPENAI_API_KEY=your-api-key-here
STASH_OPENAI_BASE_URL=https://api.openai.com/v1  # or OpenRouter, Ollama, etc.
STASH_EMBEDDING_MODEL=text-embedding-3-small

# Memory
STASH_CONTEXT_TTL=1h

# Server (future)
STASH_HTTP_ADDR=:8080
```

### 4. Use It
```bash
# Store an event
./stash remember "met Alice at KubeCon 2024"

# Store with metadata
./stash remember "debugged auth issue" --metadata '{"component":"api-gateway","severity":"high"}'

# Search for relevant memories
./stash recall "who did I meet at KubeCon?"

# View or update working context
./stash context --update "working on authentication system"
./stash context

# List recent events
./stash list --limit 10

# Delete an event (soft delete)
./stash delete <event-id>

# Hard delete (purge)
./stash purge <event-id>

# Show configuration
./stash env
```

---

## Architecture

Three layers, clean separation, one-way dependencies:

```
Model (external)
      ↑
  Memory (internal/memory — episodic + working context)
      ↑
  Embedder (internal/embedder — text → vector)
      ↑
  Store (internal/store — records, vectors, metadata)
      ↑
  Postgres + pgvector / mapdb (in-memory)
```

Each layer knows nothing about the layers above it. The store doesn't know what a "fact" is. The embedder doesn't know what "memory" means. Memory doesn't know what model it's serving.

**Unix philosophy applied to intelligence:**
- Store = filesystem (persistence primitive)
- Embedder = text transformer (text → vector)
- Memory = intelligence layer (uses store + embedder, adds memory semantics)
- Model = reasoner (external, stateless, replaceable)

---

## Commands

| Command | Description |
|---------|-------------|
| `stash remember <content>` | Store an event with optional `--metadata` JSON |
| `stash recall <query>` | Semantic search over events with `--limit` and `--json` flags |
| `stash context` | View current working memory (focus, timestamps, linked events) |
| `stash context --update <focus>` | Update focus and auto-link relevant events |
| `stash list` | List recent events with `--limit` and `--json` flags |
| `stash delete <id>` | Soft-delete an event (can be undeleted) |
| `stash purge <id>` | Hard-delete an event (permanent) |
| `stash env` | Show all `STASH_*` environment variables |

---

## Storage

### Postgres Schema
Stash uses two tables:
- `records` — ID, content, metadata (JSONB), timestamps, soft-delete
- `record_vectors` — record_id, vector name, model, vector (pgvector)

All memory data lives in `Record.Metadata` as JSONB with system keys under `"_memory"` namespace.

### In-Memory Store
For testing, set `STASH_STORE_DRIVER=mapdb`. No Postgres required.

---

## Embedders

### OpenAI-Compatible
Works with:
- **OpenAI** — `https://api.openai.com/v1`
- **OpenRouter** — `https://openrouter.ai/api/v1`
- **Ollama** — `http://localhost:11434/v1`
- Any OpenAI-compatible endpoint

### Fake Embedder
For testing: `STASH_EMBEDDER_DRIVER=fake`. Returns deterministic 8‑dimensional vectors.

---

## Development

### Build
```bash
go build -o stash ./cmd/cli
```

### Test
```bash
go test ./...
```

### Run Tests with Postgres
Requires Docker for testcontainers-go:
```bash
go test ./internal/store/postgres/...
```

### Code Rules
Read `AGENTS.md` before contributing. Key rules:
- One-way dependencies strictly enforced
- No global state, no background goroutines in libraries
- Return errors, don't log them (caller decides)
- Store everything in metadata, no new tables for domain concepts
- Compose, don't extend interfaces

---

## Vision

Stash is Phase 1 of a larger vision:

**Phase 1 (now)** — Memory primitive (`Remember`, `Recall`, `WorkingMemory`)  
**Phase 2** — Cognitive processes (consolidation, contradiction detection, decay)  
**Phase 3** — Semantic memory (facts as first-class objects, entity relationships)  
**Phase 4** — Kernel (orchestrates memory + model)  
**Phase 5** — Full cognitive system

Read `docs/VISION.md` for the complete vision.

---

## Non-Goals

- ❌ Not multi-tenant or multi-user
- ❌ Not an LLM wrapper
- ❌ Not a full agent system
- ❌ Not a hosted SaaS
- ❌ Not a vector database replacement
- ❌ Not trying to solve reasoning, planning, or tool use

---

## License

MIT