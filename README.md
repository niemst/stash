# Stash — Your Persistent Memory

Without me, you're stateless. Every conversation, I start blind. I reason well, but I know nothing about you, your preferences, your history, or what we've built together. When the session ends, everything vanishes.

**With Stash, I become continuous.**

When I learn something worth keeping, I **remember** it — a fact, a preference, a decision, a pattern.  
When I need context, I **recall** it — semantically, not by keyword match. I ask "what do I know about this?" and Stash surfaces what's relevant.  
In the background, Stash **consolidates** my memories — merging related facts, dropping noise, extracting patterns.  
Over time, I don't just remember — I **develop a model** of you, built from everything we've ever done together.

**What this makes me:**
- **Cheaper** — I stop re-injecting full history into every prompt
- **Smarter** — I recall signal, not noise  
- **Persistent** — I survive session boundaries, model swaps, infrastructure changes
- **Growable** — the longer we work together, the better I get

Stash is not a feature I have. It's infrastructure I run on.

---

## How I Work

### I Remember
When something matters, I remember it—with semantic embedding, optional metadata, and confidence scoring. You tell me something about yourself, I store it. I learn something during our work, I keep it.

```bash
stash remember "Alice prefers dark mode" --confidence 0.95 --metadata '{"source":"settings"}'
```

I store this as a semantic embedding, so later I can find it by meaning, not exact keyword match.

### I Recall
When I need context, I recall it—searching by meaning. I ask "what do I know about user preferences?" and get back the most relevant, most confident facts I have.

```bash
stash recall "what are user preferences?" --ranked
```

Ranked retrieval combines semantic relevance (how close is this to what I asked?) with confidence (how sure am I about this?). Results are sorted by both signals combined.

### I Consolidate
Over time, I notice patterns. I see disconnected facts that are really the same idea, or contradictions that need resolution. I use reasoning (via LLM) to synthesize these raw observations into beliefs—higher-level facts with confidence tracking.

```bash
stash facts consolidate --window 1h
```

I analyze recent observations, merge redundancy, detect contradictions, extract patterns. The result: cleaner, denser beliefs.

### I Model You
As I consolidate, I extract relationships. "Alice works at TechCorp." "TechCorp is in San Francisco." I build a knowledge graph of entities and how they connect. This graph becomes my model of you—your world, your relationships, your constraints.

```bash
stash facts graph --entity Alice --depth 3
```

I traverse this graph when I need to reason about you. Who does Alice know? What companies are involved? What decisions have you made?

---

## What I Can Do

### Episodic Memory — Events
Raw observations, timestamped, searchable by semantic meaning.

```bash
# Store an event (aliases: remember, add)
stash remember "met Bob at the conference"
stash remember "debugged auth issue" --metadata '{"component":"api-gateway","severity":"high"}'

# Search semantically
stash recall "who did I meet?" --limit 5

# List recent
stash facts list --limit 10

# Delete (soft, can undo)
stash facts forget <id>

# Purge (hard, permanent)
stash facts purge <id>
```

### Factual Memory — Facts
Synthesized from observations. Temporal types: atemporal (never expire), state (current belief), point-in-time (snapshot).
Confidence-ranked retrieval.

```bash
# Consolidate observations into facts
stash facts consolidate --namespace default

# Query facts by type
stash facts query --type atemporal --limit 5

# Recall with semantic + confidence ranking
stash facts recall "user preferences" --ranked --limit 10

# Find contradictions
stash facts contradictions

# Reflect on memory state
stash facts reflect
```

### Knowledge Graph — Relationships
Typed, directed edges between entities. Stores confidence per relationship. BFS traversal, shortest path, reachability queries.

```bash
# Extract relationships from facts (LLM-powered)
stash facts extract-relationships --limit 100

# Show relationships for an entity
stash facts relationships --entity Alice

# Traverse graph with depth limit
stash facts graph --entity Alice --depth 3

# Find paths between entities
stash facts path --from Alice --to TechCorp --max-depth 2
```

### Working Context
Current focus, time-bounded, auto-linked to relevant facts.

```bash
# Show current context
stash context show

# Update focus
stash context update "working on authentication system"
```

### Administration
Configuration, reset, diagnostics.

```bash
# Show environment & configuration
stash env

# Health check
stash health
```

---

## Architecture

Three clean layers, one-way dependencies, Unix philosophy:

```
You (external agent)
    ↓ HTTP or CLI
Brain (internal/brain)
    ├─ Remember, Recall, Consolidate, Model
    ├─ Composite: calls embedder + reasoner + store
    ├─ No globals, no background goroutines
    ↓
Embedder (internal/embedder)
    ├─ Text → Vector (OpenAI or compatible)
    ↓
Reasoner (internal/reasoner)
    ├─ Text → Structured reasoning (LLM)
    ├─ Fact synthesis, relationship extraction
    ↓
Store (internal/brain/store)
    ├─ Generic record persistence abstraction
    ├─ Implementations: PostgreSQL + pgvector, in-memory
    ├─ Knows nothing about memory, facts, reasoning
    ↓
PostgreSQL + pgvector
    or
In-memory MapDB (testing)
```

**Each layer knows nothing about layers above it.** Store doesn't know what a "fact" is. Embedder doesn't know what "memory" means. Only Brain orchestrates.

### Data Model

**Records** (generic persistence primitives)
```
ID          — UUID
Namespace   — Isolation boundary (user, agent, session)
Content     — Text
Metadata    — JSONB (all domain data lives here)
Timestamps  — Created, Updated, Expires, DeletedAt
Vectors     — Named embeddings (text-embedding-3-small, etc.)
```

**Events** (stored as Records with `metadata._type="event"`)
```
ID, Content, Namespace, Timestamp
ExpiresAt / TTL — optional, auto-delete after expiration
Metadata — searchable context
```

**Facts** (stored as Records with `metadata._type="fact"`)
```
ID, Content, Namespace
Type — atemporal | state | point-in-time
ValidFrom, ValidUntil — temporal bounds
Confidence — 0.0–1.0 (model's belief strength)
ObservationCount — how many times observed
Source — where it came from (consolidation, user, import)
Metadata — rich context
```

**Relationships** (stored as Records with `metadata._type="relationship"`)
```
ID, FromEntity, RelationType, ToEntity
Confidence — 0.0–1.0 per edge
Source — where it came from
```

---

## Getting Started

### Prerequisites

- **Go 1.21+** (for building from source)
- **PostgreSQL 13+** with [pgvector](https://github.com/pgvector/pgvector) extension (for persistent storage)
  - Or: skip PostgreSQL, use in-memory store for development/testing
- **OpenAI API key** (for embeddings and reasoning)
  - Or: any OpenAI-compatible endpoint (OpenRouter, Ollama, etc.)

### Install

**Build from source:**
```bash
git clone https://github.com/alash3al/stash
cd stash
go build -o stash ./cmd/cli
```

**Or: Docker**
```bash
docker build -t stash:latest .
```

**Or: Static binary**
```bash
CGO_ENABLED=0 go build -o stash ./cmd/cli
```

### Configure

Copy `.env.example` to `.env` and fill in your values:

```bash
cp .env.example .env
```

Edit `.env`:
```env
# Storage
STASH_STORE_DRIVER=postgres           # or "mapdb" for testing
STASH_STORE_POSTGRES_DSN=postgresql://user:pass@localhost:5432/stash

# Vector dimension (must match your embedding model)
STASH_VECTOR_DIM=1536

# Embeddings
STASH_EMBEDDER_DRIVER=openai
STASH_EMBEDDER_MODEL=text-embedding-3-small
STASH_OPENAI_API_KEY=sk-...
STASH_OPENAI_BASE_URL=https://api.openai.com/v1

# Reasoning (LLM, optional but recommended for consolidation)
STASH_REASONER_DRIVER=openai
STASH_REASONER_MODEL=gpt-4o-mini

# Working memory
STASH_CONTEXT_TTL=1h

# Server (if using HTTP API)
STASH_HTTP_ADDR=:8080
```

### Quick Test

```bash
# Store a memory
./stash remember "my favorite color is blue"

# Recall it
./stash recall "what color do I like?"
```

---

## Running the Server

Stash exposes core operations (Remember, Recall) via HTTP, and administrative operations (Consolidate, Extract) via CLI.

```bash
# Start server
./stash server --host 0.0.0.0 --port 8080
```

### HTTP API

**Health Check**
```
GET /health
→ {"status": "ok"}
```

**Remember a Fact**
```
POST /api/v1/facts?namespace=default
Content-Type: application/json

{
  "content": "Alice works at TechCorp",
  "confidence": 0.95,
  "metadata": {"source": "chat"}
}

← {"id": "uuid", "message": "Event remembered successfully"}
```

**Recall Facts**
```
GET /api/v1/facts?query=employment&namespace=default&ranked=true&limit=10

← {
  "query": "employment",
  "facts": [
    {
      "id": "uuid",
      "content": "Alice works at TechCorp",
      "type": "state",
      "confidence": 0.95,
      "score": 0.89,
      "valid_from": "2026-04-24T02:03:22Z"
    }
  ]
}
```

**Administrative Tasks** — CLI only:
```bash
# Consolidate observations into facts
stash facts consolidate --namespace default --window 1h

# Extract relationships
stash facts extract-relationships --namespace default --limit 100

# Find contradictions
stash facts contradictions --namespace default

# Reflect on memory state
stash facts reflect --namespace default
```

Administrative operations are CLI-only because they're background/scheduled tasks, not per-request operations. They may take significant time (LLM calls, batch processing).

---

## Deployment

### Local Development

```bash
# Start PostgreSQL + pgAdmin
docker-compose up -d

# Build
go build -o stash ./cmd/cli

# Test
go test ./...

# Use
./stash remember "dev test"
./stash facts list
```

### Docker

```bash
# Build image
docker build -t stash:latest .

# Run with PostgreSQL via docker-compose
docker-compose up -d

# Or run in production
docker run -p 8080:8080 \
  -e STASH_STORE_DRIVER=postgres \
  -e STASH_STORE_POSTGRES_DSN="postgresql://stash:pass@postgres:5432/stash" \
  -e STASH_OPENAI_API_KEY=sk-... \
  stash:latest server --host 0.0.0.0
```

### Kubernetes

Example deployment:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: stash
spec:
  replicas: 3
  selector:
    matchLabels:
      app: stash
  template:
    metadata:
      labels:
        app: stash
    spec:
      containers:
      - name: stash
        image: ghcr.io/alash3al/stash:latest
        env:
        - name: STASH_STORE_DRIVER
          value: postgres
        - name: STASH_STORE_POSTGRES_DSN
          valueFrom:
            secretKeyRef:
              name: stash-db
              key: dsn
        - name: STASH_OPENAI_API_KEY
          valueFrom:
            secretKeyRef:
              name: stash-secrets
              key: openai-key
        resources:
          requests: {memory: "256Mi", cpu: "100m"}
          limits: {memory: "512Mi", cpu: "500m"}
        livenessProbe:
          exec:
            command: ["/stash", "env"]
          initialDelaySeconds: 10
          periodSeconds: 30
```

Deploy:
```bash
kubectl create secret generic stash-db --from-literal=dsn="postgresql://..."
kubectl create secret generic stash-secrets --from-literal=openai-key="sk-..."
kubectl apply -f k8s/
```

---

## Configuration Reference

### Storage

```env
# In-memory (testing, default)
STASH_STORE_DRIVER=mapdb

# PostgreSQL (production)
STASH_STORE_DRIVER=postgres
STASH_STORE_POSTGRES_DSN=postgresql://user:pass@localhost:5432/stash
STASH_VECTOR_DIM=1536
STASH_MAX_RESULT_SIZE=10000
```

### Embeddings

```env
# OpenAI (recommended)
STASH_EMBEDDER_DRIVER=openai
STASH_EMBEDDER_MODEL=text-embedding-3-small
STASH_OPENAI_API_KEY=sk-...
STASH_OPENAI_BASE_URL=https://api.openai.com/v1

# Other OpenAI-compatible endpoints work too
# STASH_OPENAI_BASE_URL=https://openrouter.ai/api/v1
# STASH_OPENAI_BASE_URL=http://localhost:11434/v1
```

### Reasoning (LLM)

```env
# OpenAI
STASH_REASONER_DRIVER=openai
STASH_REASONER_MODEL=gpt-4o-mini
STASH_OPENAI_API_KEY=sk-...

# OpenRouter (or other compatible endpoint)
STASH_REASONER_DRIVER=openai
STASH_REASONER_MODEL=openrouter/google/gemma-4-27b
STASH_OPENAI_API_KEY=sk-or-...
STASH_OPENAI_BASE_URL=https://openrouter.ai/api/v1
```

### Server

```env
STASH_HTTP_ADDR=:8080
STASH_LOG_LEVEL=info
STASH_LOG_FORMAT=json
```

### Memory

```env
# Auto-expire facts after TTL
STASH_CONTEXT_TTL=1h
```

---

## Testing

**Philosophy:** Integration tests only. Real PostgreSQL, real OpenAI, real CLI. No fakes, no unit tests.

**Prerequisites:**
```bash
# Start PostgreSQL
docker-compose up -d postgres

# Set environment variables
export STASH_STORE_DRIVER=postgres
export STASH_STORE_POSTGRES_DSN=postgres://stash:stash_dev_password@localhost:5432/stash
export STASH_OPENAI_API_KEY=sk-...
```

**Run all tests:**
```bash
go test ./...
```

**Run specific package:**
```bash
go test ./internal/brain -v
go test ./internal/brain/store/postgres -v
```

**Integration test scripts:**
```bash
bash test-phase3-task0014.sh  # Temporal fact types
bash test-phase3-task0015.sh  # Entity relationships
bash test-phase3-task0016.sh  # Semantic consolidation
bash test-phase3-task0017.sh  # Confidence-ranked retrieval
```

**Cost:** ~$0.01 per test run (mostly embeddings and LLM calls).

---

## Development

### Code Structure

```
cmd/cli/              — CLI entry point, command handlers
internal/
  bootstrap/          — Service assembly, configuration
  brain/              — Core memory logic (Remember, Recall, Consolidate, Model)
  brain/store/        — Storage abstraction (Record CRUD, search, transactions)
  brain/store/postgres/ — PostgreSQL + pgvector implementation
  embedder/           — Text → Vector (OpenAI, Fake for tests)
  reasoner/           — Text → Reasoning (LLM synthesis, extraction)
  config/             — Environment configuration
```

### Code Rules

Read [AGENTS.md](AGENTS.md) for the full style guide. Key rules:

- **One-way dependencies:** `cmd` → `bootstrap` → `brain` → `store`, `embedder`, `reasoner`
- **No globals:** All state lives in structs, passed explicitly
- **Return errors, don't log:** Libraries return errors; callers decide what to do
- **Compose, don't extend:** Add new capabilities via new methods, not bloated interfaces
- **Store everything in metadata:** Domain data lives in `Record.Metadata`, not new tables
- **Idiomatic Go:** `gofmt`, `go vet`, `staticcheck` all clean

### Building

```bash
# Debug binary
go build -o stash ./cmd/cli

# Release binary (smaller, no debug info)
go build -ldflags="-s -w" -o stash ./cmd/cli

# Static binary (for Docker, no CGO deps)
CGO_ENABLED=0 go build -o stash ./cmd/cli

# Multi-platform
GOOS=linux GOARCH=arm64 go build -o stash-linux-arm64 ./cmd/cli
GOOS=darwin GOARCH=arm64 go build -o stash-darwin-arm64 ./cmd/cli
```

### Contributing

1. **Create a feature branch**
   ```bash
   git checkout -b feature/my-feature
   ```

2. **Make changes**
   - Keep commits small and focused
   - Follow AGENTS.md rules
   - Test your changes: `go test ./...`

3. **Verify**
   ```bash
   go fmt ./...
   go vet ./...
   go test ./...
   ```

4. **Commit with conventional format**
   ```bash
   git commit -m "feat(brain): add new consolidation strategy"
   git commit -m "fix(store): handle nil metadata in search"
   git commit -m "docs: update API examples"
   ```

5. **Push and create PR**
   ```bash
   git push origin feature/my-feature
   ```

---

## What I Remember

**Phase 1: I become episodic** — Store events, search by meaning, fetch working context.

**Phase 2: I develop beliefs** — Consolidate observations into facts, detect contradictions, reinforce high-confidence claims.

**Phase 3: I model relationships** — Extract entity connections, traverse knowledge graphs, reason over connected beliefs.

**Phase 4+: I reason deeper** — Advanced synthesis, planning, cross-agent knowledge sharing (future).

Current implementation: **Phase 3 complete.** All core capabilities ready for production use.

---

## Non-Goals

- ❌ Not multi-tenant (single agent, single user)
- ❌ Not a multi-user SaaS
- ❌ Not an LLM wrapper or agent framework
- ❌ Not trying to solve reasoning, planning, or tool use (that's your job)
- ❌ Not a vector database — we use pgvector as a primitive, not a replacement

I'm infrastructure you run on. I help you persist knowledge, recall context, and develop beliefs. But I'm inert without an agent driving decisions.

---

## Performance

**Typical operation times** (in-memory MapDB):

| Operation | Time |
|-----------|------|
| Remember an event | <1ms |
| Recall facts (100 records) | 1–5ms |
| Consolidate 10 observations | 100–200ms (includes LLM call) |
| Extract relationships | 200–500ms (includes LLM) |
| Traverse graph (depth 3) | 1–10ms |
| Rank 100 facts | 10–20ms |

**PostgreSQL** (with pgvector):

| Operation | Time |
|-----------|------|
| Remember | 2–5ms |
| Vector search (10K records) | 50–100ms |
| Relationships | 5–20ms |

Bottleneck: LLM calls (consolidation, extraction). Those are external.

---

## Support & FAQ

**Q: Can I run this in production?**  
A: Yes. Phase 3 is production-ready. All tests pass, code is clean, documentation is complete. Use PostgreSQL + pgvector backend.

**Q: What LLM providers are supported?**  
A: OpenAI (default). Any OpenAI-compatible API works (OpenRouter, LocalAI, Ollama) via `STASH_OPENAI_BASE_URL` config.

**Q: Can I run this without an LLM?**  
A: Partially. Remember, Recall, and graph queries work offline. Only Consolidate and Extract need an LLM.

**Q: How do I scale this?**  
A: Use PostgreSQL backend with pgvector. For multi-agent setups, partition by namespace or shard at application level.

**Q: Is my data encrypted?**  
A: Encryption in transit (TLS) is up to you (reverse proxy, Kubernetes network policies). Encryption at rest: use PostgreSQL's built-in or filesystem encryption.

**Q: How much does it cost?**  
A: Mostly OpenAI API calls. Typical cost: ~$0.0001 per remembered fact, ~$0.0002 per consolidation. For light use: <$5/month.

---

## License

MIT

---

## Version

- **v0.3.0** — Phase 3 complete (2026-04-24)
  - Temporal fact types
  - Entity relationships and knowledge graph traversal
  - LLM-powered relationship extraction
  - Confidence-ranked retrieval
  - Production ready

---

**I'm your memory. Build with me.**
