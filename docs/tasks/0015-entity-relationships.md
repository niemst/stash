# Task: Entity Relationships (Knowledge Graph)

**Status:** Completed  
**Date:** 2026-04-24

---

## 1. Context

**Goal:** Add entity relationships to semantic memory. Create a knowledge graph layer that enables multi-hop reasoning and graph-based queries.

**Why:** Facts alone don't enable relationship reasoning. "Alice works at TechCorp" is a fact. "Where does TechCorp get located?" requires traversing Alice→TechCorp→located_in→Paris. A knowledge graph enables the model to reason **across** entities, not just within them.

**What this is:**
- Directed edges between entities (Alice → works_at → TechCorp)
- Typed relationships (works_at, located_in, manages, etc.)
- Graph traversal (BFS, path finding, reachability)
- Confidence tracking per relationship
- CLI commands to explore graph

**What this is NOT:**
- Automatic relationship extraction (Phase 3 Task 0016)
- Semantic consolidation or fact merging
- Ranked retrieval
- Knowledge base federation

---

## 2. Boundaries

**In scope:**
- Relationship type: `from_entity → relationship_type → to_entity`
- Stored as Records with `_memory.type = "relationship"`
- Methods:
  - `StoreRelationship(ctx, namespace, from, type, to) error`
  - `GetRelationshipsFrom(ctx, namespace, entity) []Relationship`
  - `GetRelationshipsTo(ctx, namespace, entity) []Relationship`
  - `TraverseGraph(ctx, namespace, entity, depth) map[entity][]Relationship`
  - `FindPath(ctx, namespace, from, to, depth) []Relationship`
  - `GetAllRelationships(ctx, namespace) []Relationship`
- CLI commands:
  - `stash facts relationships --entity=X`
  - `stash facts graph --entity=X --depth=N`
- Unit tests (7 test cases)
- No schema changes

**Not in scope:**
- Automatic extraction from facts
- Semantic merging
- Ranked retrieval
- Visualization
- Query DSL beyond simple traversal

---

## 3. Approach & Review

**Graph Model:**

Directed typed edges:
```
Alice --[works_at]--> TechCorp
TechCorp --[located_in]--> Paris
Alice --[manages]--> Bob
```

**Query Operations:**

1. **GetRelationshipsFrom(entity)** — All outgoing edges
   - Query: "Who/what does Alice relate to?"
   - Answer: {works_at→TechCorp, manages→Bob}

2. **GetRelationshipsTo(entity)** — All incoming edges
   - Query: "Who/what relates to TechCorp?"
   - Answer: {Alice-[works_at]→, Bob-[works_at]→}

3. **TraverseGraph(entity, depth)** — BFS to depth limit
   - Query: "What's reachable from Alice in ≤2 hops?"
   - Answer: Map of {Alice→[dests], TechCorp→[dests], ...}

4. **FindPath(from, to, depth)** — BFS shortest path
   - Query: "How is Alice connected to Paris?"
   - Answer: [Alice-[works_at]→TechCorp, TechCorp-[located_in]→Paris]

**Design Decisions:**

- Typed relationships (not generic edges) — clarity and semantics
- Confidence per edge (0.0-1.0) — uncertainty propagation
- BFS for traversal (not DFS) — shortest paths first
- Depth limits — prevent infinite loops, control scope
- Store as Records — no schema change, metadata-first

---

## 4. Implementation Notes

**Files Modified:**
- `internal/memory/types.go`: Relationship struct, RelationshipFromRecord
- `internal/memory/memory.go`: 6 query/traversal methods
- `cmd/cli/facts_relationships.go`: CLI commands (new)
- `cmd/cli/main.go`: Register commands
- `internal/memory/memory_test.go`: 7 unit tests

**Relationship Storage:**
```go
type Relationship struct {
  ID           string    // UUID
  Namespace    string
  FromEntity   string
  RelationType string   // "works_at", "located_in", etc.
  ToEntity     string
  Source       string   // "consolidation", "user"
  Confidence   float32  // 0.0-1.0
  CreatedAt    time.Time
  FactID       string   // optional link back
}
```

**BFS Implementation:**
- Queue-based traversal with visited set
- Prevents infinite loops
- Respects depth limit
- Returns map of reachable entities

---

## 5. Acceptance Criteria

- [x] Relationship type defined and stored as Records
- [x] StoreRelationship creates edges correctly
- [x] GetRelationshipsFrom queries outgoing edges
- [x] GetRelationshipsTo queries incoming edges
- [x] TraverseGraph returns reachable entities with depth limit
- [x] FindPath returns shortest path or error
- [x] GetAllRelationships returns all relationships in namespace
- [x] CLI relationships command shows incoming/outgoing
- [x] CLI graph command shows subgraph
- [x] 7 unit tests all passing
- [x] No schema changes
- [x] Full backward compatibility
- [x] Confidence tracking on all relationships

---

## 6. Verification Plan

**Unit Tests:**
1. StoreRelationship creates and persists
2. GetRelationshipsFrom filters correctly
3. GetRelationshipsTo filters correctly
4. TraverseGraph does BFS with depth limit
5. FindPath returns shortest path
6. FindPath errors on disconnected nodes
7. GetAllRelationships returns all

**Integration Test:**
1. Create a graph of relationships
2. Query outgoing relationships
3. Query incoming relationships
4. Traverse graph at various depths
5. Find paths between entities
6. Verify CLI output

**Backward Compatibility:**
- Phase 2 facts unaffected
- No store schema changes
- Existing queries work unchanged

---

## 7. Execution Log

- [2026-04-24 22:35] Added Relationship type to types.go
- [2026-04-24 22:35] Implemented RelationshipFromRecord extractor
- [2026-04-24 22:35] Added 6 query methods to Memory
- [2026-04-24 22:35] Created CLI commands (relationships, graph)
- [2026-04-24 22:35] Added 7 unit tests, all pass
- [2026-04-24 22:35] Created integration test script
- [2026-04-24 22:35] All 140+ tests pass

---

## 8. Outcome

**Final Result:**

Task 0015 (Entity Relationships / Knowledge Graph) is complete. The system now supports multi-hop reasoning via directed edges between entities.

**What Changed:**
- types.go: +60 lines (Relationship struct, extractor)
- memory.go: +200 lines (6 query methods)
- CLI: +80 lines (2 commands)
- Tests: +120 lines (7 test cases)
- Total: ~460 lines

**What Was Verified:**
- All relationship methods work (store, query, traverse, find path)
- BFS traversal respects depth limits
- Path finding returns shortest path
- CLI commands execute cleanly
- JSON output valid and queryable
- No schema changes
- All 140+ tests pass

**What Remains Open:**
- Task 0016: Semantic Consolidation (extract relationships from facts)
- Task 0017: Confidence-Ranked Retrieval (rank by relevance + confidence)
- Integration with model reasoning
- Query DSL extension
- Visualization

---

## 9. Next

Proceed to Task 0016 (Semantic Consolidation) to automatically extract relationships from facts.
