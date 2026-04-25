# Stash — Persistent Memory for AI

Without me, you're stateless. Every conversation starts blind. With Stash, I become continuous — I remember, I recall, I consolidate, I learn.

## Quick Start

```bash
# Build
go build -o stash ./cmd/cli

# Configure
cp .env.example .env  # edit with your OpenAI key + Postgres DSN

# Remember
./stash remember "I prefer dark mode" -n /users/alice

# Recall
./stash recall "UI preferences" -n /users/alice

# Consolidate (episodes → facts → relationships → patterns → contradictions → decay)
./stash consolidate run -n /users/alice
```

## Commands

| Command | Description |
|---------|-------------|
| `stash remember <content>` | Store an episode (alias: `add`) |
| `stash recall <query>` | Semantic search across episodes and facts |
| `stash forget <query>` | Soft-delete the best-matching episode |
| `stash purge episode <id>` | Hard-delete an episode |
| `stash purge fact <id>` | Hard-delete a fact |
| `stash facts list` | List facts in a namespace |
| `stash consolidate run` | Run 5-stage consolidation once |
| `stash consolidate serve` | Run consolidation on a schedule |
| `stash context set <focus>` | Set working context |
| `stash context show` | Show current context |
| `stash namespace create <slug>` | Create a namespace |
| `stash namespace list` | List namespaces |
| `stash contradictions list` | List unresolved contradictions |
| `stash contradictions resolve <id>` | Resolve a contradiction |
| `stash causal list` | List causal links between facts |
| `stash causal create` | Create a causal link between two facts |
| `stash causal trace <id>` | Trace causal chain forward or backward |
| `stash causal delete <id>` | Delete a causal link |
| `stash hypothesis create` | Create a hypothesis |
| `stash hypothesis list` | List hypotheses |
| `stash hypothesis show <id>` | Show a hypothesis |
| `stash hypothesis test <id>` | Mark hypothesis as testing |
| `stash hypothesis confirm <id>` | Confirm hypothesis (auto-creates fact) |
| `stash hypothesis reject <id>` | Reject a hypothesis |
| `stash hypothesis refine <id>` | Refine a hypothesis (resets to proposed) |
| `stash hypothesis delete <id>` | Delete a hypothesis |
| `stash goal create` | Create a goal |
| `stash goal list` | List goals |
| `stash goal show <id>` | Show a goal with sub-goal progress |
| `stash goal complete <id>` | Complete a goal (auto-completes parent if all siblings done) |
| `stash goal abandon <id>` | Abandon a goal |
| `stash goal update <id>` | Update an active goal |
| `stash goal delete <id>` | Delete a goal and its sub-goals |
| `stash failure create` | Record a failure (what, why, lesson) |
| `stash failure list` | List failures |
| `stash failure show <id>` | Show a failure |
| `stash failure delete <id>` | Delete a failure |
| `stash serve` | Start all services (HTTP, MCP, consolidation) |
| `stash mcp serve` | Start MCP SSE server |
| `stash mcp execute` | Start MCP stdio server |
| `stash env` | Show configuration |

### Flags

- `-n, --namespace` — Namespace path (default: `/`)
- `--namespaces` — Comma-separated paths; each includes itself + all descendants
- `--limit` — Max results (default: 10 for recall, 100 for lists)
- `--offset` — Result offset for pagination
- `--occurred-at` — RFC3339 timestamp for episode

## Architecture

```
cmd/cli/               — CLI entry point
internal/
  bootstrap/           — Service wiring
  brain/               — Core: Remember, Recall, Consolidate, Contradictions, Causal, Hypothesis, Goals, Failures
  db/                  — pgxpool + goose migrations
  models/              — Domain structs
  queries/             — sqltmpl dynamic SQL
  embedder/            — OpenAI embeddings + cache
  reasoner/            — LLM reasoning (facts, relationships, patterns, contradictions, causal links, goal progress, failure patterns, hypothesis evidence)
  observability/       — Prometheus metrics
  config/              — Environment configuration
```

### 8-Stage Consolidation

1. **Episodes → Facts** — Cluster similar episodes, synthesize into facts via LLM (+ inline contradiction detection)
2. **Facts → Relationships** — Extract entity edges (subject-predicate-object)
3. **Facts → Causal Links** — Extract cause-effect pairs between facts
4. **Goal Progress Inference** — Scan recent facts against active goals; annotate with [PROGRESS], [SUGGESTED COMPLETE], or [CONTRADICTED]
5. **Failure Pattern Detection** — Detect repeated failures (creates `REPEAT FAILURE` episodes) and extract higher-order failure patterns as facts
6. **Facts + Relationships → Patterns** — Abstract higher-order patterns
7. **Hypothesis Evidence Scanning** — Auto-transition hypotheses based on new evidence: proposed→testing→confirmed, or auto-reject at threshold 0.9
8. **Confidence Decay** — Pure SQL batch: `confidence *= decay_factor`; auto-expire below threshold

Each stage tracks checkpoints so it only processes new data on subsequent runs.

### Data Model

```
namespaces                — Hierarchical paths (/users/alice, /projects/stash)
  episodes                — Raw observations with embeddings (append-only, immutable)
  facts                   — Synthesized beliefs with confidence + entity/property/value
  fact_sources            — Links facts to source episodes
  relationships           — Entity edges (from → type → to)
  patterns                — Abstractions over facts/relationships
  contexts                — Working focus per namespace (with TTL)
  consolidation_progress  — Per-namespace checkpoints (including goal/failure/hypothesis tracking)
  settings                — Operational state (embedding model lock)
  embedding_cache         — Deduplicated embedding computation
  contradictions          — Conflicting facts: old vs new, entity/property, resolution
  causal_links            — Fact → fact cause-effect with confidence and method
  hypotheses              — Uncertain beliefs with verification plans, status lifecycle
  goals                   — Persistent objectives with parent/sub-goal hierarchy
  failures                — What didn't work, why, and what to do instead
```

### Namespace Resolution

Namespaces are hierarchical paths. Every read operation is **recursive by default** — querying `/users/alice` returns results from alice and all sub-namespaces. `/` means everything.

- No wildcards — paths are plain strings
- Empty namespace list = explicit error (forces intentionality)
- Write operations target a single namespace exactly

### Self-Model (/self)

Stash includes a built-in convention for agent self-knowledge. Calling `init` (MCP tool) creates the `/self` namespace scaffold:

- `/self/capabilities` — What the agent can do well
- `/self/limits` — What the agent struggles with or cannot do
- `/self/preferences` — How the agent works best

These are ordinary namespaces — `remember` to store, `recall` to retrieve, `consolidate` to extract facts. The self-model is not special infrastructure; it's the agent using Stash on itself.

## Configuration

All configuration via environment variables (`.env` file supported):

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `STASH_POSTGRES_DSN` | Yes | — | PostgreSQL connection string |
| `STASH_VECTOR_DIM` | Yes | — | Embedding dimension (must match model) |
| `STASH_MAX_RESULT_SIZE` | Yes | — | Max rows for list queries |
| `STASH_OPENAI_API_KEY` | Yes | — | OpenAI-compatible API key |
| `STASH_OPENAI_BASE_URL` | Yes | — | API base URL |
| `STASH_EMBEDDING_MODEL` | Yes | — | Embedding model name |
| `STASH_REASONER_MODEL` | Yes | — | Reasoning LLM model name |
| `STASH_CONTEXT_TTL` | Yes | — | Working context TTL (e.g. `1h`) |
| `STASH_HTTP_ADDR` | Yes | — | HTTP listen address (e.g. `:8080`) |
| `STASH_LOG_LEVEL` | Yes | — | `debug`, `info`, `warn`, `error` |
| `STASH_LOG_FORMAT` | Yes | — | `text` or `json` |
| `STASH_CONSOLIDATION_BATCH_SIZE` | No | `100` | Episodes per consolidation batch |
| `STASH_CONSOLIDATION_SIMILARITY_THRESHOLD` | No | `0.85` | Clustering similarity |
| `STASH_CONSOLIDATION_DEDUP_THRESHOLD` | No | `0.95` | Fact dedup threshold |
| `STASH_CONSOLIDATION_WINDOW` | No | `168h` | Consolidation time window |
| `STASH_DECAY_FACTOR` | No | `0.95` | Confidence decay multiplier per run |
| `STASH_EXPIRY_THRESHOLD` | No | `0.1` | Confidence below which facts are auto-expired |
| `STASH_HYPOTHESIS_AUTO_CONFIRM_THRESHOLD` | No | `0.9` | Confidence threshold for auto-confirming hypotheses during consolidation |
| `STASH_HYPOTHESIS_AUTO_REJECT_THRESHOLD` | No | `0.9` | Confidence threshold for auto-rejecting hypotheses during consolidation |

## Observability

Start the HTTP server:

```bash
./stash serve --http-host 0.0.0.0 --http-port 9090
```

Endpoints:
- `GET /healthz` — Liveness (database ping)
- `GET /readyz` — Readiness (consolidation layer accessible)
- `GET /metrics` — Prometheus metrics

Prometheus metrics (labeled by namespace):
- `stash_build_info` — Version gauge
- `consolidation_events_read_total`
- `consolidation_events_processed_total`
- `consolidation_facts_created_total`
- `consolidation_facts_deduplicated_total`
- `consolidation_relationships_created_total`
- `consolidation_llm_calls_total`
- `consolidation_duration_seconds`
- `consolidation_errors_total`

## MCP Integration

Stash exposes an MCP server for agent integration:

```bash
# SSE (for remote agents)
./stash mcp serve --host 0.0.0.0 --port 8080

# Stdio (for local agents like Claude Desktop)
./stash mcp execute
```

MCP tools:

| Tool | Description |
|------|-------------|
| `init` | Initialize memory connection; creates /self namespace scaffold |
| `remember` | Store an episode (raw observation) |
| `recall` | Semantic search across episodes and facts |
| `forget` | Soft-delete a matching episode |
| `consolidate` | Run consolidation on a namespace |
| `set_context` | Set working focus for a namespace |
| `get_context` | Read current working focus |
| `clear_context` | Clear working focus |
| `list_namespaces` | List all namespaces |
| `create_namespace` | Create a new namespace |
| `query_facts` | List consolidated facts |
| `query_relationships` | List entity relationships |
| `list_contradictions` | List unresolved contradictions |
| `resolve_contradiction` | Resolve a contradiction |
| `list_causal_links` | List cause-effect relationships |
| `create_causal_link` | Manually assert a causal link |
| `trace_causal_chain` | Follow a chain of causation |
| `list_hypotheses` | List hypotheses |
| `create_hypothesis` | Create a hypothesis |
| `confirm_hypothesis` | Confirm hypothesis (auto-creates fact) |
| `reject_hypothesis` | Reject a hypothesis |
| `list_goals` | List goals |
| `create_goal` | Create a goal |
| `complete_goal` | Complete a goal (auto-completes parent) |
| `abandon_goal` | Abandon a goal |
| `list_failures` | List recorded failures |
| `create_failure` | Record a failure (what, why, lesson) |
| `delete_failure` | Delete a failure |

## Production Notes

- **Vector dimension** is set at runtime from `STASH_VECTOR_DIM`, not hardcoded in migrations
- **Connection pool**: 25 max conns, 5 min conns, 30min lifetime, 5min idle, 30s health checks
- **Embedding model lock**: Stored in `settings` table; startup fails on mismatch
- **Graceful shutdown**: SIGTERM/SIGINT handled in `serve`, `consolidate serve`, and `mcp serve`
- **Namespace slugs**: Must start with `/`, use lowercase alphanumeric/hyphen segments
- **Content limit**: 10,000 characters max per episode
- **Forward-only migrations**: No down migrations; schema changes are additive
- **Contradiction auto-supersede**: LLM classifies as `replacement` + confidence ≥ 0.9 → old fact superseded automatically
- **Confidence decay**: Pure SQL batch operation; zero LLM calls
- **Hypothesis lifecycle**: proposed → testing → confirmed (auto-creates fact) / rejected; auto-transition during consolidation at threshold 0.9
- **Goal auto-complete**: Completing all sub-goals auto-completes the parent, recursively; consolidation annotates active goals with progress
- **Failure records**: Immutable once created; `lesson` field is required (the anti-repeat mechanism); consolidation detects repetitions and extracts failure patterns

## License

Apache 2.0
