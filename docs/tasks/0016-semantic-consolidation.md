# Task: Semantic Consolidation (Relationship Extraction)

**Status:** In Execution  
**Date:** 2026-04-24

---

## 1. Context

**Goal:** Automatically extract relationships between entities from stored facts. Use LLM reasoning to identify `entity → relationship_type → entity` triples and store them in the knowledge graph.

**Why:** Facts alone describe isolated beliefs. "Alice works at TechCorp" is valuable. But the graph enables reasoning: "Where do people Alice works with live?" or "What skills do people at TechCorp have?" Relationships transform facts from isolated statements into a connected knowledge structure.

**What this is:**
- Method `Memory.ConsolidateRelationships(ctx, namespace)` to extract relationships from recent facts
- Uses LLM (ReasonStructured) to identify entity pairs and relationship types
- Stores extracted relationships with source tracking and confidence
- CLI command to trigger extraction and preview results
- Conflict handling: tolerate duplicate relationships (idempotent)
- Unit tests + integration test

**What this is NOT:**
- Automatic background extraction (manual CLI trigger only)
- Schema changes to facts or relationships
- Relationship ranking or confidence propagation (Phase 3 Task 0017)
- Query optimization or indexing
- Visualization or UI

---

## 2. Boundaries

**In scope:**
- `Memory.ConsolidateRelationships(ctx, namespace, limit)` method
  - Retrieves recent facts (limit=100 default)
  - For each fact, asks LLM: "What entities and relationships does this fact describe?"
  - Parses LLM response to extract `(from_entity, relationship_type, to_entity)` triples
  - Stores each triple as a Relationship record
  - Returns count of relationships extracted
- Extend reasoner with `ReasonStructured(ctx, texts)` for entity/relationship extraction
  - Add prompt requesting relationship extraction format
  - Parse output for `From: X | RelationType: Y | To: Z | Confidence: N` format
- CLI command: `stash facts consolidate --namespace=<ns>` 
  - Shows extracted relationship count
  - Optional preview of relationships (first N)
- Source tracking: all extracted relationships have `source="consolidation"`
- Confidence scoring for extracted relationships (0.7 baseline for LLM extraction)
- Idempotency: same relationship extracted twice = upsert, not duplicate
- Unit tests (5–6 tests)
- Integration test with real PostgreSQL + LLM

**Not in scope:**
- Background consolidation or scheduled triggers
- Updating existing relationships (only insert/store)
- Conflict resolution between relationships
- Querying consolidated relationships (they're in the graph already)
- Relationship ranking or sorting
- Batch extraction from historical facts

---

## 3. Approach & Review

**Extraction Strategy:**

For each recent fact:
1. Fact content: "Alice works at TechCorp"
2. Ask LLM: "Identify all entities (people, places, organizations) and relationships between them in this fact."
3. LLM response (structured): "From: Alice | RelationType: works_at | To: TechCorp | Confidence: 0.9"
4. Parse response, store Relationship record
5. Return summary: "Extracted 3 relationships from 8 facts"

**LLM Prompt:**

```
You are extracting semantic relationships from a fact for a knowledge graph.

Fact: {fact_content}

Identify all entities and relationships in this fact. For each relationship:
- From: the subject entity
- RelationType: a simple, lowercase relationship type (e.g., works_at, located_in, manages, knows, owns)
- To: the object entity
- Confidence: 0.7-1.0 (how confident you are this relationship is valid)

Format each relationship on a new line:
From: Subject | RelationType: type_name | To: Object | Confidence: 0.8

Example fact: "Alice is an engineer at TechCorp in Paris"
From: Alice | RelationType: role_at | To: engineer | Confidence: 0.85
From: Alice | RelationType: works_at | To: TechCorp | Confidence: 0.9
From: TechCorp | RelationType: located_in | To: Paris | Confidence: 0.85
From: engineer | RelationType: at_organization | To: TechCorp | Confidence: 0.75
```

**Design Decisions:**

- **LLM method:** Extend `ReasonStructured` to handle multiple outputs (multiple relationships per fact)
- **Confidence:** Extract from LLM response; fallback to 0.7 if parsing fails
- **Deduplication:** Use `StoreRelationship` which updates if same triple exists
- **Source tracking:** All consolidated relationships have `source="consolidation"`
- **Error handling:** If extraction fails for one fact, log and continue (don't block whole batch)
- **Recent facts:** Query facts created in last 7 days (configurable via parameter)

---

## 4. Implementation Notes

**Files to Create/Modify:**

1. `internal/reasoner/reasoner.go`
   - Add `ReasonRelationships(ctx, factContent) ([]StructuredRelationship, error)` method
   - `StructuredRelationship` struct: FromEntity, RelationType, ToEntity, Confidence

2. `internal/reasoner/openai.go`
   - Implement `ReasonRelationships` with multi-line parsing

3. `internal/reasoner/fake.go`
   - Implement `ReasonRelationships` deterministically for tests

4. `internal/memory/memory.go`
   - `ConsolidateRelationships(ctx, namespace, limit) (int, error)` method
     - Query recent facts
     - For each fact, call `reasoner.ReasonRelationships()`
     - Store extracted relationships
     - Return count

5. `cmd/cli/facts_consolidate.go` (new)
   - Command: `stash facts consolidate --namespace=<ns> [--preview]`

6. `cmd/cli/main.go`
   - Register consolidate command

7. `internal/memory/memory_test.go`
   - Test extraction parsing
   - Test relationship storage
   - Test idempotency
   - Test error handling

---

## 5. Acceptance Criteria

- [ ] `ReasonRelationships` method added to Reasoner interface
- [ ] OpenAI implementation parses multi-line relationship output
- [ ] Fake implementation returns deterministic relationships for testing
- [ ] `Memory.ConsolidateRelationships(ctx, namespace, limit)` exists and works
- [ ] Extracted relationships stored with source="consolidation" and proper confidence
- [ ] CLI command `stash facts consolidate --namespace=<ns>` executes
- [ ] Duplicate relationships are upserted (no duplication)
- [ ] 5+ unit tests covering: extraction, parsing, storage, idempotency, error cases
- [ ] Integration test creates facts, consolidates, verifies relationships exist
- [ ] All existing tests still pass
- [ ] No schema changes
- [ ] Full backward compatibility

---

## 6. Verification Plan

**Unit Tests:**
1. `ReasonRelationships` parses multi-line output correctly
2. Extracting relationships from a single fact works
3. Consolidating multiple facts returns correct count
4. Duplicate relationships are not created (upsert)
5. Missing confidence defaults to 0.7
6. Integration: real facts → consolidation → graph query works

**Integration Test:**
1. Create 5 facts with entity/relationship content
2. Call `ConsolidateRelationships()`
3. Verify relationships were extracted and stored
4. Query graph to confirm edges exist
5. Verify CLI command outputs correctly

**Compatibility:**
- No impact on existing facts, events, or relationships
- Phase 2 facts work unchanged
- Phase 3 graph queries unaffected

---

## 7. Execution Steps

- [ ] Add `ReasonRelationships` to interface
- [ ] Implement in OpenAI + Fake
- [ ] Add `ConsolidateRelationships` method
- [ ] Create CLI command
- [ ] Write unit tests (5+)
- [ ] Run integration test
- [ ] Verify all tests pass
- [ ] Commit with conventional message

---

## 8. Progress Notes

- [2026-04-24 starting] Reading context and existing code

---

## 9. Outcome

(To be filled after completion)
