# Stash

**Persistent memory for AI agents.**

Every LLM starts every conversation from zero. Stash fixes that. It's a memory layer that remembers, recalls, consolidates, and learns — so your agent doesn't have to repeat itself across sessions.

Built on PostgreSQL + pgvector. Exposed via MCP. No ORM, no abstractions, just direct pgx SQL.

## Quick Start

```bash
go build -o stash ./cmd/cli
cp .env.example .env   # add your OpenAI key + Postgres DSN

./stash remember "I prefer dark mode" -n /users/alice
./stash recall "UI preferences" -n /users/alice
./stash consolidate run -n /users/alice
```

Or with Docker:

```bash
docker compose up
```

## How It Works

Stash has three memory types:

| Type | What | How |
|------|------|-----|
| **Episodes** | Raw observations | `remember` stores them |
| **Facts** | Distilled truths | `consolidate` extracts them |
| **Context** | Current working focus | `set_context` / `get_context` |

Consolidation runs an 8-stage pipeline that turns raw observations into structured knowledge:

```
Episodes → Facts → Relationships → Causal Links
  → Goal Progress → Failure Patterns → Patterns → Hypothesis Evidence → Decay
```

Each stage only processes new data since the last run. No wasted work.

## Namespaces

Everything lives in a namespace. They're hierarchical paths like `/users/alice` or `/projects/stash`.

**Reads are recursive by default** — querying `/users/alice` returns results from alice *and* all sub-namespaces. `/` means everything.

```bash
./stash namespace create /projects/myapp --name "MyApp" --description "My application project"
./stash remember "Using PostgreSQL 16 with pgvector" -n /projects/myapp
./stash recall "database setup" -n /projects/myapp
```

## Memory Features

### Contradiction Detection
When new information conflicts with existing facts, Stash detects it. If the new fact clearly replaces the old one (confidence ≥ 0.9), it auto-supersedes. Otherwise, it records the contradiction for human review.

### Causal Links
Facts can be linked cause → effect. Chain traversal uses a recursive CTE — trace forward ("what did this lead to?") or backward ("why did this happen?").

### Hypotheses
Uncertain beliefs with verification plans. Lifecycle: `proposed → testing → confirmed` (auto-creates a fact) or `rejected`. During consolidation, new evidence can auto-transition hypotheses at threshold 0.9.

### Goals
Persistent objectives across sessions. Sub-goals with auto-complete — when all children complete, the parent completes too. Consolidation annotates active goals with progress notes.

### Failures
What didn't work, why, and what to do instead. The `lesson` field is required — it's the anti-repeat mechanism. Consolidation detects when failures are being repeated and flags them.

### Self-Model (/self)
The `init` MCP tool creates `/self/capabilities`, `/self/limits`, `/self/preferences`. These are ordinary namespaces — the agent uses Stash on itself.

## MCP Integration

Stash is designed to be used by AI agents via MCP:

```bash
# SSE (remote agents)
./stash mcp serve --host 0.0.0.0 --port 8080 --with-consolidation

# Stdio (Claude Desktop, etc.)
./stash mcp execute --with-consolidation
```

The `--with-consolidation` flag runs background consolidation alongside the MCP server — one process does both.

**27 MCP tools:**

| Tool | Does |
|------|------|
| `init` | Create /self namespace scaffold |
| `remember` | Store an episode |
| `recall` | Semantic search |
| `forget` | Remove an episode |
| `consolidate` | Run consolidation |
| `set_context` | Set working focus |
| `get_context` | Read working focus |
| `clear_context` | Clear working focus |
| `list_namespaces` | List namespaces |
| `create_namespace` | Create a namespace |
| `query_facts` | List consolidated facts |
| `query_relationships` | List entity relationships |
| `list_contradictions` | List contradictions |
| `resolve_contradiction` | Resolve a contradiction |
| `list_causal_links` | List causal links |
| `create_causal_link` | Assert a causal link |
| `trace_causal_chain` | Follow causation chain |
| `list_hypotheses` | List hypotheses |
| `create_hypothesis` | Create a hypothesis |
| `confirm_hypothesis` | Confirm → auto-create fact |
| `reject_hypothesis` | Reject with reason |
| `list_goals` | List goals |
| `create_goal` | Create a goal |
| `complete_goal` | Complete (auto-completes parent) |
| `abandon_goal` | Abandon a goal |
| `list_failures` | List failures |
| `create_failure` | Record a failure |
| `delete_failure` | Delete a failure |

## Configuration

All via environment variables (`.env` file supported):

| Variable | Default | Description |
|----------|---------|-------------|
| **Required** | | |
| `STASH_POSTGRES_DSN` | — | PostgreSQL connection string |
| `STASH_VECTOR_DIM` | — | Embedding dimension (1536 for text-embedding-3-small) |
| `STASH_MAX_RESULT_SIZE` | — | Max rows for list queries |
| `STASH_OPENAI_API_KEY` | — | OpenAI-compatible API key |
| `STASH_OPENAI_BASE_URL` | — | API base URL |
| `STASH_EMBEDDING_MODEL` | — | Embedding model name |
| `STASH_REASONER_MODEL` | — | Reasoning LLM model name |
| `STASH_CONTEXT_TTL` | — | Working context TTL (e.g. `1h`) |
| `STASH_HTTP_ADDR` | — | HTTP listen address |
| `STASH_LOG_LEVEL` | — | `debug`, `info`, `warn`, `error` |
| `STASH_LOG_FORMAT` | — | `text` or `json` |
| **Optional** | | |
| `STASH_CONSOLIDATION_BATCH_SIZE` | `100` | Episodes per batch |
| `STASH_CONSOLIDATION_SIMILARITY_THRESHOLD` | `0.85` | Clustering similarity |
| `STASH_CONSOLIDATION_DEDUP_THRESHOLD` | `0.95` | Fact dedup threshold |
| `STASH_CONSOLIDATION_WINDOW` | `168h` | Time window |
| `STASH_DECAY_FACTOR` | `0.95` | Confidence decay per run |
| `STASH_EXPIRY_THRESHOLD` | `0.1` | Auto-expire below this |
| `STASH_HYPOTHESIS_AUTO_CONFIRM_THRESHOLD` | `0.9` | Auto-confirm threshold |
| `STASH_HYPOTHESIS_AUTO_REJECT_THRESHOLD` | `0.9` | Auto-reject threshold |

## Architecture

```
internal/
  brain/       — Core logic: Remember, Recall, Consolidate, all memory features
  db/          — pgxpool + goose migrations (20 forward-only migrations)
  models/      — Domain structs
  queries/     — sqltmpl dynamic SQL
  embedder/    — OpenAI embeddings + pgx-backed cache
  reasoner/    — LLM reasoning for all 8 consolidation stages
  config/      — Environment configuration
  bootstrap/   — Service wiring
  observability/ — Prometheus metrics
```

No ORM. No generic store interface. Direct pgx throughout.

## License

Apache 2.0
