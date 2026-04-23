# Task: Reinforcement (Fact Confidence Tracking)

**Status:** Completed  
**Date:** 2026-04-23
**Completed:** 2026-04-23

---

## 1. Context

**Goal:** Track fact confidence based on observation count. Facts observed multiple times become stronger; noisy facts remain weak.

**Why:** Memory that learns from repetition is smarter than memory that treats all facts equally. A fact observed 5 times is more likely true than a fact observed once. Reinforcement enables confidence-ranked retrieval later (Phase 3).

**What this is:** Add confidence score to Facts. Implement reinforcement logic that increases confidence when facts are re-observed. Update facts during consolidation when the same entity+property+value appears again.

**What this is NOT:**
- Probabilistic reasoning (that's Phase 3)
- Auto-deletion of low-confidence facts
- Scheduled confidence decay (Phase 3+)
- Semantic similarity thresholds (facts must match exactly)

---

## 2. Boundaries

**In scope:**
- Add `Confidence` (0.0-1.0) and `ObservationCount` to Fact type
- Stored in `_memory.confidence` and `_memory.observation_count`
- `Memory.Reinforce(ctx, namespace, entity, property, value) error` method
- Update existing fact: increment count, recalculate confidence
- Confidence formula: `min(1.0, observationCount / (observationCount + 2))`
- Tests: 6+ cases covering reinforcement logic, edge cases

**Not in scope:**
- Confidence decay over time
- Probabilistic combination of conflicting facts
- Temporal confidence (what was true then vs now)
- Auto-deletion based on confidence
- Retrieval ranking by confidence (Phase 3)

---

## 3. Design

### 3.1 Fact Type Enhancement

**Update `internal/memory/types.go`:**

```go
type Fact struct {
	ID                 string         // UUID
	Namespace          string
	Content            string
	SynthesizedFrom    []string
	ValidFrom          time.Time
	ValidUntil         *time.Time
	Source             string
	ConflictWith       []string
	Confidence         float32        // 0.0-1.0; default 0.5 for new facts
	ObservationCount   int            // how many times observed/reinforced
	CreatedAt          time.Time
	Metadata           map[string]any
	Score              float32
}

// FactFromRecord updated to parse confidence and observation_count
```

### 3.2 Confidence Formula

```go
// Calculate confidence based on observation count
// More observations → higher confidence, asymptotically approaching 1.0
// Formula: count / (count + 2)
// Examples:
//   count=1: 1/3 ≈ 0.33 (weak)
//   count=2: 2/4 = 0.50 (medium)
//   count=5: 5/7 ≈ 0.71 (strong)
//   count=10: 10/12 ≈ 0.83 (very strong)
func calculateConfidence(observationCount int) float32 {
	if observationCount <= 0 {
		return 0.0
	}
	confidence := float32(observationCount) / float32(observationCount+2)
	if confidence > 1.0 {
		confidence = 1.0
	}
	return confidence
}
```

### 3.3 Reinforce Method

**New method in `internal/memory/memory.go`:**

```go
// Reinforce increments the observation count for a fact matching entity+property+value.
// If a matching fact exists, updates its observation count and confidence.
// If no match exists, returns error (caller should consolidate instead).
//
// Used when the same fact is observed again, reinforcing its truthfulness.
// Typically called during consolidation when duplicate facts are detected.
func (m *Memory) Reinforce(ctx context.Context, namespace, entity, property, value string) error
```

**Implementation:**

1. Query facts with entity+property
2. Find exact value match
3. If found: increment count, recalculate confidence, update fact
4. If not found: return error
5. No new facts created (only reinforcement of existing)

### 3.4 ConsolidateRecent Integration

When synthesizing facts, check if a fact already exists:
- Same entity+property+value → reinforce it
- Different value → create new fact (handled by existing contradiction logic)

**Optional for 0012:** Can be a future enhancement. For now, Reinforce is manual.

---

## 4. Implementation Notes

**File changes:**
- `internal/memory/types.go` — enhance Fact, update FactFromRecord
- `internal/memory/memory.go` — add Reinforce, calculateConfidence helpers
- `internal/memory/memory_test.go` — 6+ tests

**Reinforce workflow:**

```go
func (m *Memory) Reinforce(ctx context.Context, namespace, entity, property, value string) error {
	// 1. Query facts with entity+property
	facts := queryFactsByEntityProperty(ctx, namespace, entity, property)
	
	// 2. Find exact value match
	var targetFact *Fact
	for _, fact := range facts {
		if fact.Metadata["value"] == value {
			targetFact = fact
			break
		}
	}
	
	if targetFact == nil {
		return fmt.Errorf("no fact found for entity=%q property=%q value=%q", entity, property, value)
	}
	
	// 3. Increment count, recalculate confidence
	targetFact.ObservationCount++
	targetFact.Confidence = calculateConfidence(targetFact.ObservationCount)
	
	// 4. Update in store
	record := factToRecord(targetFact)
	return m.store.Put(ctx, record)
}
```

---

## 5. Acceptance Criteria

### Fact Type
- [ ] `Confidence` (float32) field added
- [ ] `ObservationCount` (int) field added
- [ ] Stored in `_memory.confidence` and `_memory.observation_count`
- [ ] FactFromRecord parses both fields
- [ ] New facts default to confidence=0.5, count=1

### Confidence Formula
- [ ] `calculateConfidence()` implemented
- [ ] count=1 → ~0.33, count=5 → ~0.71, count=10 → ~0.83
- [ ] Returns 0.0-1.0 range
- [ ] Asymptotic (approaches 1.0 but never exceeds)

### Reinforce Method
- [ ] Method exists: `Reinforce(ctx, namespace, entity, property, value) error`
- [ ] Finds matching fact by entity+property+value
- [ ] Increments observation_count
- [ ] Recalculates confidence
- [ ] Updates fact in store
- [ ] Returns error if no matching fact

### Testing
- [ ] TestReinforce_ExistingFact: increments count, updates confidence
- [ ] TestReinforce_MultipleReinforcements: count increases each time
- [ ] TestReinforce_ConfidenceProgression: confidence follows formula
- [ ] TestReinforce_NotFound: error when fact doesn't exist
- [ ] TestReinforce_DifferentValue: doesn't match different values
- [ ] TestReinforce_PersistsToStore: updated fact retrievable from store
- [ ] `go vet` and `staticcheck` pass

---

## 6. Explicit Assumptions

- Confidence formula: count / (count + 2)
- New facts start with confidence=0.5, count=1
- Exact value matching required (no fuzzy matching)
- Reinforcement doesn't create new facts
- No confidence decay over time (Phase 3+)
- No temporal tracking (same confidence for recent and old reinforcements)

---

## 7. Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Confidence formula too aggressive | Conservative denominator (+2); can tune later |
| Reinforcing wrong fact | Exact entity+property+value match required |
| No decay for old facts | Document as Phase 3 enhancement |

---

## 8. Definition of Done

- Code compiles without warnings
- All 6+ tests pass
- Confidence calculated correctly
- Reinforcement updates facts in store
- `go vet` and `staticcheck` pass
- Ready for review

---

## 9. After 0012: Phase 2 Complete

With Reinforcement done:
- ✅ Consolidation (synthesize events → facts)
- ✅ Contradiction Detection (temporal overlap detection)
- ✅ Reflection (memory introspection)
- ✅ Reinforcement (fact confidence tracking)

**Phase 2 is complete.** Next: CLI commands for Phase 2 features, then Phase 3 (semantic facts, entity graphs, confidence-ranked retrieval).
