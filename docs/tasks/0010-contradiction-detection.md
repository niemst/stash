# Task: Contradiction Detection with Temporal Facts

**Status:** Completed  
**Date:** 2026-04-23
**Completed:** 2026-04-23

---

## 1. Context

**Goal:** Detect conflicting beliefs in memory with temporal awareness. Facts that contradict should be surfaced for review, but evolution over time (was true, now false) should be tracked, not flagged as error.

**Why:** Memory without conflict detection silently accumulates contradictions. "Mohamed speaks French" + "Mohamed doesn't speak French" (both active) = corrupted knowledge. But "Mohamed speaks French (2026-01)" + "Mohamed speaks Spanish (2026-04)" = normal evolution, needs tracking, not alarm.

**What this is:** New `ValidFrom`/`ValidUntil` fields on Facts. `Memory.FindContradictions()` method. Temporal overlap detection. Production-ready contradiction reporting.

**What this is NOT:**
- Auto-resolution or merging of contradictions
- Semantic checking (will implement in Phase 3 with Reasoner)
- Automatic deletion of conflicting facts
- Consensus mechanisms or voting
- Confidence scoring or probabilistic logic

---

## 2. Boundaries

**In scope:**
- Enhance `Fact` type with `ValidFrom`, `ValidUntil`, `Source` fields
- Store temporal metadata in `_memory.valid_from`, `_memory.valid_until`, `_memory.source`
- `Contradiction` type with both facts' IDs, entity, property, values, temporal info
- `Memory.FindContradictions(ctx, namespace) ([]Contradiction, error)`
- Overlap detection: two facts with same entity+property, overlapping time ranges, different values
- Status categorization: "conflict" (overlapping, incompatible) vs "evolution" (sequential, allowed)
- Update `Memory.ConsolidateRecent()` to set temporal metadata on created facts
- Tests: 10+ cases covering overlaps, sequential facts, edge cases, multiple contradictions

**Not in scope:**
- Auto-merging or resolution
- Semantic contradiction detection (Phase 3)
- Confidence scoring
- Temporal reasoning beyond overlap
- Deprecation or soft-deletion of old facts
- User-facing conflict resolution UI

---

## 3. Design

### 3.1 Fact Type Enhancement

**Update `internal/memory/types.go`:**

```go
// Fact represents a durable, synthesized belief derived from events.
// Stored as a store.Record with _memory.type = "fact".
// Temporal fields allow tracking evolution and detecting contradictions.
type Fact struct {
	ID              string         // UUID
	Namespace       string         // same as source events
	Content         string         // the synthesized fact text
	SynthesizedFrom []string       // event IDs used to create this fact
	ValidFrom       time.Time      // when this fact becomes true (required)
	ValidUntil      *time.Time     // when fact stops being true; nil = ongoing
	Source          string         // where fact came from: "consolidation", "user", "import"
	ConflictWith    []string       // fact IDs with detected overlapping contradictions
	CreatedAt       time.Time      // when this record was created in store
	Metadata        map[string]any // optional caller metadata (entity, property, value, etc.)
	Score           float32        // similarity score for retrieval
}

// FactFromRecord extracts a Fact from a store.Record (updated to parse temporal fields)
func FactFromRecord(r *store.Record) (*Fact, error) { ... }

// Helper to check if two facts' temporal ranges overlap
func timeRangesOverlap(from1, until1, from2, until2 time.Time) bool {
	return from1.Before(until2 || time.Now()) && from2.Before(until1 || time.Now())
}
```

### 3.2 Contradiction Type

**New type in `internal/memory/types.go`:**

```go
// Contradiction represents two facts with incompatible values for the same entity+property.
// Stored transiently (not persisted); computed on-demand via FindContradictions().
type Contradiction struct {
	ID            string         // UUID for this contradiction report
	FactID1       string         // first fact ID
	FactID2       string         // second fact ID
	Entity        string         // the entity in question (from metadata)
	Property      string         // the property (from metadata)
	Value1        string         // first fact's value
	Value2        string         // second fact's value
	ValidFrom1    time.Time      // first fact's temporal scope
	ValidUntil1   *time.Time
	ValidFrom2    time.Time      // second fact's temporal scope
	ValidUntil2   *time.Time
	Status        string         // "conflict" (overlapping) or "evolution" (sequential)
	DiscoveredAt  time.Time      // when this contradiction was detected
	Metadata      map[string]any // optional additional context
}

// Supported contradiction statuses
const (
	ContradictionStatusConflict   = "conflict"    // overlapping, incompatible
	ContradictionStatusEvolution  = "evolution"   // sequential, normal change
)
```

### 3.3 Metadata Storage Format

**When storing facts in ConsolidateRecent(), set:**

```go
memMeta := map[string]any{
	"type":              "fact",
	"content":           factText,
	"created_at":        now.Format(time.RFC3339),
	"valid_from":        now.Format(time.RFC3339),  // now
	"valid_until":       nil,                        // ongoing
	"source":            "consolidation",
	"synthesized_from":  eventIDs,
}

// Caller metadata (assumed to be set for contradiction detection):
recordMeta := map[string]any{
	"_memory": memMeta,
	"entity":   "Mohamed",      // from event text, set by caller
	"property": "programming_language",  // caller-defined
	"value":    "Go",           // caller-defined
}
```

### 3.4 FindContradictions Method

**New method in `internal/memory/memory.go`:**

```go
// FindContradictions returns all facts in a namespace that contradict each other.
// Two facts contradict if:
// - They have the same entity and property (from metadata)
// - Their valid time ranges overlap
// - Their values differ
//
// Returns contradictions sorted by discovery time.
// Status is "conflict" if ranges overlap (both active), "evolution" if sequential.
// Returns empty slice if no contradictions found.
func (m *Memory) FindContradictions(ctx context.Context, namespace string) ([]Contradiction, error)
```

**Implementation:**

1. Query all facts in namespace: `store.List()` with `_memory.type = "fact"`
2. For each fact pair (i, j where i < j):
   - Extract entity, property, value from metadata
   - If entity and property match AND values differ:
     - Parse ValidFrom, ValidUntil
     - Check timeRangesOverlap()
     - If overlapping: Status = "conflict"
     - If sequential: Status = "evolution"
     - Create Contradiction record
3. Return sorted by DiscoveredAt

**Edge cases:**
- ValidUntil = nil → treat as "now" for overlap check
- Both ValidUntil = nil → overlapping (both ongoing)
- One ValidUntil = nil, other before it → no overlap
- Entity or property missing from metadata → skip (can't compare)
- Value missing → treat as distinct (safe default)

### 3.5 Update ConsolidateRecent

**In `internal/memory/memory.go`, when storing synthesized fact:**

```go
factID := uuid.New().String()
now := time.Now().UTC()

memMeta := map[string]any{
	"type":              typeFact,
	"content":           factText,
	"created_at":        now.Format(time.RFC3339),
	"valid_from":        now.Format(time.RFC3339),    // NEW
	"valid_until":       nil,                          // NEW (nil = ongoing)
	"source":            "consolidation",              // NEW
	"synthesized_from":  eventIDs,
}
```

Note: Caller is responsible for setting entity, property, value in recordMeta if they want contradiction detection to work.

---

## 4. Implementation Notes

**File changes:**
- `internal/memory/types.go` — enhance Fact, add Contradiction, add timeRangesOverlap helper
- `internal/memory/memory.go` — add FindContradictions method, update ConsolidateRecent
- `internal/memory/memory_test.go` — 12+ tests
- `internal/memory/errors.go` — add error sentinel if needed

**Overlap detection pseudocode:**

```go
func timeRangesOverlap(from1, until1, from2, until2 time.Time) bool {
	// Treat nil until as "now"
	effectiveUntil1 := until1
	if until1.IsZero() {
		effectiveUntil1 = time.Now()
	}
	effectiveUntil2 := until2
	if until2.IsZero() {
		effectiveUntil2 = time.Now()
	}

	// Two ranges [A, B) and [C, D) overlap iff A < D and C < B
	return from1.Before(effectiveUntil2) && from2.Before(effectiveUntil1)
}
```

**Sorting contradictions:**

```go
// Sort by: namespace (already filtered), then by (entity, property), then by discovery time
// Makes it easy for caller to batch review
sort.Slice(contradictions, func(i, j int) bool {
	if contradictions[i].Entity != contradictions[j].Entity {
		return contradictions[i].Entity < contradictions[j].Entity
	}
	if contradictions[i].Property != contradictions[j].Property {
		return contradictions[i].Property < contradictions[j].Property
	}
	return contradictions[i].DiscoveredAt.Before(contradictions[j].DiscoveredAt)
})
```

**Backward compatibility:**
- Existing Facts (from 0009) will have ValidFrom = now, ValidUntil = nil, Source = "consolidation"
- When facts are updated (in future), caller must set temporal fields
- FindContradictions works on all facts regardless of age

---

## 5. Acceptance Criteria

### Fact Type
- [ ] `Fact` type has ValidFrom, ValidUntil, Source fields
- [ ] Fields stored in `_memory.valid_from`, `_memory.valid_until`, `_memory.source` metadata
- [ ] FactFromRecord() parses temporal fields correctly
- [ ] nil ValidUntil handled as "ongoing"

### Contradiction Type
- [ ] `Contradiction` type defined with all required fields
- [ ] Status values: "conflict" and "evolution"
- [ ] Metadata field for extensibility

### FindContradictions Method
- [ ] Method exists: `FindContradictions(ctx, namespace) ([]Contradiction, error)`
- [ ] Returns all contradictions in namespace
- [ ] Empty slice if no contradictions
- [ ] Returns []Contradiction sorted by entity, property, discovery time
- [ ] Handles missing entity/property in metadata (skips)
- [ ] Handles nil ValidUntil as "ongoing"

### Overlap Detection
- [ ] Two overlapping facts → contradiction
- [ ] Two sequential facts (no overlap) → evolution, no error
- [ ] One ongoing, one ended before start → no overlap
- [ ] Same entity, different property → no contradiction
- [ ] Same entity+property, same value → no contradiction

### ConsolidateRecent Update
- [ ] Synthesized facts get valid_from = now
- [ ] Synthesized facts get valid_until = nil
- [ ] Synthesized facts get source = "consolidation"
- [ ] Existing consolidation tests still pass

### Testing
- [ ] TestFindContradictions_OverlappingConflict: two overlapping incompatible facts
- [ ] TestFindContradictions_SequentialEvolution: two sequential facts, no error
- [ ] TestFindContradictions_OneOngoing: one fact ongoing, one ended before start
- [ ] TestFindContradictions_SamePropertySameValue: same entity+property but same value
- [ ] TestFindContradictions_DifferentProperty: same entity, different properties
- [ ] TestFindContradictions_MissingMetadata: facts without entity/property
- [ ] TestFindContradictions_MultipleContradictions: 3+ facts with multiple conflicts
- [ ] TestFindContradictions_EmptyNamespace: no facts in namespace
- [ ] TestTimeRangesOverlap: unit tests for overlap logic (6 cases)
- [ ] TestConsolidateRecent_SetsTemporalMetadata: verify synthesized facts have temporal fields
- [ ] `go vet` and `staticcheck` pass
- [ ] No new dependencies

---

## 6. Explicit Assumptions

- Caller sets entity, property, value in fact metadata if contradiction detection is desired
- ValidFrom is always set (required); ValidUntil is optional (nil = ongoing)
- Overlap is inclusive on start, exclusive on end: [from, until)
- "Now" for nil ValidUntil is evaluated at query time, not storage time
- Contradictions are reported but not auto-resolved
- Facts are never auto-deleted; caller reviews and decides
- Source field is informational (not used for conflict resolution)

---

## 7. Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| O(n²) comparison on many facts | n is small (facts << events). Optimize to indexing later if needed. |
| Caller forgets entity/property | Contradiction detection gracefully skips (logged as debug). Document in README. |
| Clock skew between systems | Each fact's temporal scope is set at creation; comparison is local time. Not ideal for distributed systems; acceptable for single-instance MVP. |
| ValidUntil parsing edge cases | Treat any parse error as "ongoing" (safe default). Add tests for malformed timestamps. |
| Contradictions pile up | Caller reviews via FindContradictions() output; human decision to update/delete. No auto-cleanup. |

---

## 8. Definition of Done

- Code compiles without warnings
- All 11+ tests pass
- Overlap detection verified with unit tests
- ConsolidateRecent integration tested
- Backward compatible (existing facts still work)
- `go vet` and `staticcheck` pass
- README updated with example (if one exists)
- Ready for review

---

## 9. Next Steps After 0010

- **Task 0011:** Reflection (periodic passes that ask: what do we know? what's inconsistent?)
- **Task 0012:** Reinforcement (patterns observed many times → high confidence; noise filtered out)
- **Phase 3:** Semantic facts, entity graphs, temporal type distinctions
