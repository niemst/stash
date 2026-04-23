# Stash CLI Reference

Complete command reference for the Stash memory system.

---

## Global

### `stash --help`
Show help for all commands.

### `stash env`
Display environment variables and configuration status.

```bash
stash env
```

**Output:**
- Store type and configuration
- Embedder driver and model
- Reasoner driver and model (if configured)

---

## Events Management

### `stash events create` (alias: `remember`)
Store an event in memory.

```bash
stash events create "Event content" --namespace=<ns> --metadata='{"key":"value"}'
```

**Flags:**
- `--namespace` — Namespace for the event (optional)
- `--metadata` — JSON metadata for the event (optional)

**Output:**
- `event_id` — UUID of created event
- `timestamp` — Creation timestamp
- JSON with full event details

---

### `stash events list`
List recent events.

```bash
stash events list --namespace=<ns> --limit=20
```

**Flags:**
- `--namespace` — Namespaces to list (optional, repeatable)
- `--limit` — Maximum results (default: 20)

**Output:**
- Array of events with ID, content, timestamp, metadata

---

### `stash events search` (alias: `recall`)
Search for events by semantic similarity.

```bash
stash events search "query text" --namespace=<ns> --limit=10 --where='field=value'
```

**Flags:**
- `--namespace` — Namespaces to search (optional, repeatable)
- `--limit` — Maximum results (default: 10)
- `--where` — Metadata filter (format: `field=value,field>=value,...)` (optional)

**Operators:** `=`, `!=`, `<`, `>`, `<=`, `>=`

**Output:**
- Array of events sorted by relevance score
- Score field shows semantic similarity (0-1)

---

### `stash events delete`
Soft-delete an event by ID.

```bash
stash events delete <event-id>
```

**Output:**
- Confirmation of soft delete

---

### `stash events purge`
Hard-delete an event by ID (permanent).

```bash
stash events purge <event-id>
```

**Output:**
- Confirmation of permanent deletion

---

## Context Management

### `stash context show`
View current working memory context.

```bash
stash context show --namespace=<ns>
```

**Flags:**
- `--namespace` — Namespace for context (optional)

**Output:**
- Current focus area
- Associated event IDs
- Creation/update timestamps

---

### `stash context update`
Update the focus of working memory.

```bash
stash context update "New focus" --namespace=<ns>
```

**Flags:**
- `--namespace` — Namespace for context (optional)

**Arguments:**
- Focus text (required)

**Output:**
- Updated context with new focus
- Timestamps

---

## Facts Management

### `stash facts consolidate`
Synthesize recent events into facts using LLM.

**Requires:** `STASH_REASONER_DRIVER` and `STASH_REASONER_MODEL` environment variables

```bash
stash facts consolidate --namespace=<ns> --window=1h --limit=100
```

**Flags:**
- `--namespace` — Namespace to consolidate (optional)
- `--window` — Time window for recent events (default: `1h`). Format: e.g., `1h`, `30m`, `2h`
- `--limit` — Maximum events to process (default: 100)

**Output:**
- `synthesized_count` — Number of facts created
- `facts` — Array of synthesized facts with IDs, content, confidence

---

### `stash facts contradictions`
Find contradictions in facts.

```bash
stash facts contradictions --namespace=<ns>
```

**Flags:**
- `--namespace` — Namespace to check (optional)

**Output:**
- Array of contradicting fact pairs
- Contradiction type (e.g., temporal overlap)
- Confidence scores

---

### `stash facts reflect`
Introspect memory state and generate report.

```bash
stash facts reflect --namespace=<ns>
```

**Flags:**
- `--namespace` — Namespace to reflect on (optional)

**Output:**
- Memory summary: total facts, entities, relationships
- Confidence distribution
- Identified gaps
- Source breakdown (user vs consolidation)

---

### `stash facts reinforce`
Strengthen a fact by increasing observation count and confidence.

```bash
stash facts reinforce --entity=<entity> --property=<prop> --value=<val> --count=1
```

**Flags:**
- `--entity` — Entity identifier (required)
- `--property` — Property name (required)
- `--value` — Property value (required)
- `--count` — Number of reinforcements (default: 1)

**Output:**
- Updated fact with new confidence and observation count

---

### `stash facts query`
Query facts by type (temporal semantics).

```bash
stash facts query --namespace=<ns> --type=state
```

**Flags:**
- `--namespace` — Namespace to query (optional)
- `--type` — Fact type: `atemporal`, `state`, or `point-in-time` (default: `state`)

**Fact Types:**
- `atemporal` — Always true, never expires (e.g., "Mohamed was born in Egypt")
- `state` — Current state, true until changed (e.g., "Mohamed is working on Stash")
- `point-in-time` — Snapshot at specific moment (e.g., "Mohamed deployed v0.1 on April 18")

**Output:**
- Array of facts of the specified type
- Content, confidence, validity period

---

### `stash facts recall` (alias: `search`)
Search facts by semantic similarity with optional confidence ranking.

```bash
stash facts recall "query text" --namespace=<ns> --limit=10 --ranked
```

**Flags:**
- `--namespace` — Namespace to search (optional)
- `--limit` — Maximum results (default: 10)
- `--ranked` — Use confidence-ranked retrieval (60% relevance + 40% confidence)

**Without `--ranked`:**
- Returns facts by semantic similarity only

**With `--ranked`:**
- Combined score = (relevance × 0.6) + (confidence × 0.4)
- High-confidence facts rank higher when relevance is similar
- Useful for preferring well-established beliefs

**Output:**
- Array of facts sorted by score
- Includes confidence, observation count, source

---

### `stash facts extract-relationships`
Extract relationships from facts using LLM.

**Requires:** `STASH_REASONER_DRIVER` and `STASH_REASONER_MODEL` environment variables

```bash
stash facts extract-relationships --namespace=<ns> --limit=100
```

**Flags:**
- `--namespace` — Namespace to extract from (optional)
- `--limit` — Maximum facts to process (default: 100)

**What it does:**
1. Queries recent facts (from last 7 days)
2. For each fact, asks LLM to identify entities and relationships
3. Stores relationships in knowledge graph with confidence scores
4. Returns count of extracted relationships

**Output:**
- `extracted_count` — Number of relationships extracted
- `namespace` — Namespace processed
- `limit` — Processing limit used

**Example:**
If you have fact: "Alice works at TechCorp in Paris"
Extracts relationships:
- Alice -- works_at --> TechCorp (confidence: 0.9)
- TechCorp -- located_in --> Paris (confidence: 0.85)

---

### `stash facts relationships`
Show incoming and outgoing relationships for an entity.

```bash
stash facts relationships --entity=<entity> --namespace=<ns>
```

**Flags:**
- `--entity` — Entity name (required)
- `--namespace` — Namespace to query (optional)

**Output:**
- `outgoing` — Relationships where entity is the source
  - Type, target entity, confidence, source
- `incoming` — Relationships where entity is the target
  - Type, source entity, confidence, source

**Example Outgoing:**
```json
"outgoing": [
  {"type": "works_at", "to": "TechCorp", "confidence": 0.9, "source": "consolidation"},
  {"type": "manages", "to": "Bob", "confidence": 0.8, "source": "user"}
]
```

---

### `stash facts graph`
Traverse knowledge graph from an entity.

```bash
stash facts graph --entity=<entity> --namespace=<ns> --depth=2
```

**Flags:**
- `--entity` — Starting entity (required)
- `--namespace` — Namespace to query (optional)
- `--depth` — Maximum traversal depth (default: 1)

**What it does:**
1. Starts from entity
2. BFS traversal up to depth hops
3. Returns all reachable entities and their connections

**Output:**
- `root_entity` — Starting point
- `depth` — Traversal depth used
- `nodes` — Number of entities in subgraph
- `graph` — Entity map with connections:
  ```json
  "graph": {
    "Alice": [
      {"type": "works_at", "to": "TechCorp", "confidence": 0.9}
    ],
    "TechCorp": [
      {"type": "located_in", "to": "Paris", "confidence": 0.85}
    ]
  }
  ```

**Example Use Cases:**
- Depth 1: Direct connections (who works with Alice?)
- Depth 2: Second-order connections (where do people Alice works with live?)
- Depth 3+: Multi-hop reasoning (who has skills related to Alice's projects?)

---

## Environment Configuration

### Required Environment Variables

**Store:**
- `STASH_STORE_DRIVER` — Storage backend: `mapdb` (in-memory) or `postgres`
- `STASH_STORE_POSTGRES_DSN` — PostgreSQL connection string (if driver=postgres)

**Embedder:**
- `STASH_EMBEDDER_DRIVER` — Embedding service: `openai` (default is fake for testing)
- `STASH_EMBEDDER_MODEL` — Model name: e.g., `text-embedding-3-small`
- `STASH_OPENAI_API_KEY` — API key for embedder

**Reasoner (LLM):**
- `STASH_REASONER_DRIVER` — LLM provider: `openai`
- `STASH_REASONER_MODEL` — Model name: e.g., `gpt-4o-mini`
- `STASH_OPENAI_API_KEY` — API key (shared with embedder if same provider)

### Optional Environment Variables

- `STASH_LOG_LEVEL` — Logging level: `debug`, `info`, `warn`, `error`

### Example Configuration

```bash
# In-memory store with OpenAI embeddings and LLM
export STASH_STORE_DRIVER=mapdb
export STASH_EMBEDDER_DRIVER=openai
export STASH_EMBEDDER_MODEL=text-embedding-3-small
export STASH_REASONER_DRIVER=openai
export STASH_REASONER_MODEL=gpt-4o-mini
export STASH_OPENAI_API_KEY=sk-...
```

```bash
# Production: PostgreSQL with OpenAI
export STASH_STORE_DRIVER=postgres
export STASH_STORE_POSTGRES_DSN="postgresql://user:pass@localhost/stash"
export STASH_EMBEDDER_DRIVER=openai
export STASH_EMBEDDER_MODEL=text-embedding-3-small
export STASH_REASONER_DRIVER=openai
export STASH_REASONER_MODEL=gpt-4o-mini
export STASH_OPENAI_API_KEY=sk-...
```

---

## Output Format

All commands output JSON for easy parsing and integration.

### Standard Response Structure

```json
{
  "command": "command_name",
  "namespace": "optional_ns",
  "data": [/* command-specific data */],
  "error": null,
  "timestamp": "2026-04-24T10:30:00Z"
}
```

### Parsing with jq

Extract event ID:
```bash
stash events create "Test" | jq -r '.event_id'
```

Filter facts by confidence:
```bash
stash facts query --namespace=test | jq '.facts[] | select(.confidence > 0.8)'
```

Check recall score:
```bash
stash facts recall "query" --ranked | jq '.facts[] | {content, score}'
```

---

## Common Workflows

### Store and Consolidate

```bash
# Store events
stash events create "Alice joined TechCorp" --namespace=alice
stash events create "Alice works on Go projects" --namespace=alice
stash events create "Alice lives in Paris" --namespace=alice

# Consolidate into facts
stash facts consolidate --namespace=alice

# View facts
stash facts query --namespace=alice --type=state
```

### Extract and Traverse Relationships

```bash
# Extract relationships from facts
stash facts extract-relationships --namespace=alice

# View direct connections
stash facts relationships --entity=Alice --namespace=alice

# Traverse 2 hops
stash facts graph --entity=Alice --namespace=alice --depth=2
```

### Search with Confidence

```bash
# Basic search (relevance only)
stash facts recall "Alice work" --namespace=alice

# Confidence-ranked search (relevance + confidence)
stash facts recall "Alice work" --namespace=alice --ranked
```

### Reinforce Knowledge

```bash
# Reinforce that Alice works at TechCorp
stash facts reinforce --entity=Alice --property=works_at --value=TechCorp --count=3

# Check updated confidence
stash facts query --namespace=alice --type=state
```

---

## Troubleshooting

### "reasoner not configured"
Error when running `consolidate` or `extract-relationships` without LLM setup.

**Fix:** Set `STASH_REASONER_DRIVER` and `STASH_REASONER_MODEL` environment variables.

### "bootstrap context not available"
Rare internal error. Try again or report if persists.

### "query cannot be empty"
When using search/recall commands without query text.

**Fix:** Provide query: `stash facts recall "your query" --namespace=test`

### "invalid --where flag"
Filter syntax error in metadata predicates.

**Fix:** Use correct format: `field=value,field>10,field!=exclude`

---

## Command Hierarchy

```
stash/
├── env                    — Configuration status
├── events/
│   ├── create (remember)  — Store event
│   ├── list               — List events
│   ├── search (recall)    — Search events
│   ├── delete             — Soft delete
│   └── purge              — Hard delete
├── context/
│   ├── show               — View context
│   └── update             — Update focus
└── facts/
    ├── consolidate        — Synthesize events → facts
    ├── contradictions     — Find conflicts
    ├── reflect            — Memory report
    ├── reinforce          — Strengthen fact
    ├── query              — Query by type
    ├── recall (search)    — Search facts (ranked)
    ├── extract-relationships — Extract graph
    ├── relationships      — Show entity connections
    └── graph              — Traverse graph
```

---

## Version

Stash CLI — Phase 3 Complete
- Events & Context: Phase 1
- Facts & Consolidation: Phase 2
- Temporal Types, Relationships, Extraction, Ranking: Phase 3
