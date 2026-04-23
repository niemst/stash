package memory

import (
	"fmt"
	"time"

	"github.com/alash3al/stash/internal/store"
)

// Event represents something that happened at a specific point in time.
// Stored as a store.Record with _memory.type = "event".
type Event struct {
	ID        string
	Namespace string
	Content   string
	Timestamp time.Time
	ExpiresAt *time.Time     // nil = forever, non-nil = expiration
	Metadata  map[string]any
	Score     float32
}

// WorkingMemory represents working memory — what is actively being thought about.
// Single global working memory for MVP, stored with fixed ID "_memory.context".
// Stored as a store.Record with _memory.type = "context".
type WorkingMemory struct {
	ID        string
	Focus     string
	EventIDs  []string
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiresAt time.Time
}

// Relation represents a directed semantic link between two events.
// Stored as a store.Record with _memory.type = "relationship".
type Relation struct {
	ID           string         // generated UUID
	Namespace    string         // same namespace as linked events
	FromEventID  string         // source event
	ToEventID    string         // target event
	RelationType string         // e.g., "contradicts", "caused_by"
	Metadata     map[string]any // optional caller metadata
	CreatedAt    time.Time
}

// Supported relation types (extensible)
const (
	RelationTypeContradicts = "contradicts" // A contradicts B
	RelationTypeCausedBy    = "caused_by"   // A caused B
	RelationTypeSimilarTo   = "similar_to"  // A is similar to B
	RelationTypeReferences  = "references"  // A references B
)

// FactType represents the temporal semantics category of a fact.
// Three types with different retrieval and update strategies.
const (
	// FactTypeAtemporal: Fact that is always true. Never expires.
	// Example: "Mohamed was born in Egypt"
	// ValidUntil is always nil, ValidFrom is creation time.
	FactTypeAtemporal = "atemporal"

	// FactTypeState: Current state fact. True until it changes.
	// Example: "Mohamed is working on Stash"
	// ValidUntil = nil if current, set when fact is superseded or corrected.
	FactTypeState = "state"

	// FactTypePointInTime: Snapshot at a specific moment.
	// Example: "Mohamed deployed v0.1 on April 18, 2026"
	// ValidFrom = ValidUntil = snapshot moment.
	FactTypePointInTime = "point-in-time"
)

// BulkRemember represents a single event for batch import.
// Minimal structure: just content, optional metadata and TTL.
type BulkRemember struct {
	Content  string         // required, non-empty
	Metadata map[string]any // optional caller metadata
	TTL      *time.Duration // optional; nil = no expiry
}

// Fact represents a durable, synthesized belief derived from events.
// Stored as a store.Record with _memory.type = "fact".
// Facts are synthesized from clusters of similar events via LLM reasoning.
// Temporal fields (ValidFrom, ValidUntil) track when facts are/were true.
// Confidence tracks how many times a fact has been reinforced.
type Fact struct {
	ID               string         // UUID
	Namespace        string         // same as source events
	Content          string         // the synthesized fact text
	SynthesizedFrom  []string       // event IDs used to create this fact
	Type             string         // FactType: "atemporal", "state", or "point-in-time"
	ValidFrom        time.Time      // when this fact becomes true (required)
	ValidUntil       *time.Time     // when fact stops being true; nil = ongoing
	Source           string         // where fact came from: "consolidation", "user", "import"
	ConflictWith     []string       // fact IDs with overlapping contradictions (if any)
	Confidence       float32        // 0.0-1.0; how confident we are in this fact
	ObservationCount int            // how many times this fact has been observed/reinforced
	CreatedAt        time.Time      // when fact was created in store
	Metadata         map[string]any // optional caller metadata (entity, property, value, etc.)
	Score            float32        // similarity score for retrieval
}

// FactFromRecord extracts a Fact from a store.Record.
// Returns error if record type is not "fact" or required fields are missing.
func FactFromRecord(r *store.Record) (*Fact, error) {
	// Check type
	memMeta, ok := r.Metadata["_memory"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("record metadata missing _memory field")
	}

	recType, ok := memMeta["type"].(string)
	if !ok || recType != "fact" {
		return nil, fmt.Errorf("record is not a fact (type=%q)", recType)
	}

	// Extract synthesized_from
	synthesizedFrom := []string{}
	if sf, ok := memMeta["synthesized_from"].([]any); ok {
		for _, id := range sf {
			if idStr, ok := id.(string); ok {
				synthesizedFrom = append(synthesizedFrom, idStr)
			}
		}
	}

	// Extract conflict_with
	conflictWith := []string{}
	if cw, ok := memMeta["conflict_with"].([]any); ok {
		for _, id := range cw {
			if idStr, ok := id.(string); ok {
				conflictWith = append(conflictWith, idStr)
			}
		}
	}

	// Extract timestamps
	createdAt := time.Now()
	if ts, ok := memMeta["created_at"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			createdAt = parsed
		}
	}

	validFrom := time.Now() // Default to now if not set
	if ts, ok := memMeta["valid_from"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			validFrom = parsed
		}
	}

	var validUntil *time.Time
	if ts, ok := memMeta["valid_until"].(string); ok && ts != "" {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			validUntil = &parsed
		}
	}

	source, _ := memMeta["source"].(string)

	// Extract fact type (default to state for backward compatibility)
	factType := FactTypeState
	if ft, ok := memMeta["fact_type"].(string); ok && ft != "" {
		factType = ft
	}

	// Extract confidence and observation count
	confidence := float32(0.5) // Default for new facts
	if conf, ok := memMeta["confidence"].(float64); ok {
		confidence = float32(conf)
	}

	observationCount := 1 // Default for new facts
	if count, ok := memMeta["observation_count"].(float64); ok {
		observationCount = int(count)
	}

	return &Fact{
		ID:               r.ID,
		Namespace:        r.Namespace,
		Content:          r.Content,
		SynthesizedFrom:  synthesizedFrom,
		Type:             factType,
		ValidFrom:        validFrom,
		ValidUntil:       validUntil,
		Source:           source,
		ConflictWith:     conflictWith,
		Confidence:       confidence,
		ObservationCount: observationCount,
		CreatedAt:        createdAt,
		Metadata:         r.Metadata,
		Score:            0, // No score for fact record (not from search)
	}, nil
}

// Contradiction represents two facts with incompatible values for the same entity+property.
// Stored transiently (not persisted); computed on-demand via FindContradictions().
// Status categorizes the contradiction: "conflict" (overlapping time ranges) or
// "evolution" (sequential time ranges, normal change).
type Contradiction struct {
	ID           string         // UUID for this contradiction report
	FactID1      string         // first fact ID
	FactID2      string         // second fact ID
	Entity       string         // the entity in question (from metadata)
	Property     string         // the property (from metadata)
	Value1       string         // first fact's value
	Value2       string         // second fact's value
	ValidFrom1   time.Time      // first fact's temporal scope
	ValidUntil1  *time.Time
	ValidFrom2   time.Time      // second fact's temporal scope
	ValidUntil2  *time.Time
	Status       string         // "conflict" (overlapping) or "evolution" (sequential)
	DiscoveredAt time.Time      // when this contradiction was detected
	Metadata     map[string]any // optional additional context
}

// Supported contradiction statuses
const (
	ContradictionStatusConflict  = "conflict"   // overlapping, incompatible
	ContradictionStatusEvolution = "evolution"  // sequential, normal change
)

// timeRangesOverlap checks if two temporal ranges overlap.
// Uses the provided `now` as reference for nil until values (ongoing facts).
// Two ranges [from1, until1] and [from2, until2] overlap iff from1 < until2 and from2 < until1.
func timeRangesOverlap(from1 time.Time, until1 *time.Time, from2 time.Time, until2 *time.Time, now time.Time) bool {
	effectiveUntil1 := until1
	if effectiveUntil1 == nil {
		effectiveUntil1 = &now
	}

	effectiveUntil2 := until2
	if effectiveUntil2 == nil {
		effectiveUntil2 = &now
	}

	return from1.Before(*effectiveUntil2) && from2.Before(*effectiveUntil1)
}

// ReflectionReport summarizes the state of memory in a namespace.
// Produced by Memory.Reflect() for human review and decision-making.
type ReflectionReport struct {
	Namespace           string                   `json:"namespace"`
	TotalFacts          int                      `json:"total_facts"`
	TotalContradictions int                      `json:"total_contradictions"`
	TotalEntities       int                      `json:"total_entities"`
	EntitiesByName      map[string]*EntitySummary `json:"entities_by_name"`
	Contradictions      []Contradiction          `json:"contradictions"`
	Gaps                []EntityGap              `json:"gaps"`
	DateRange           *DateRange               `json:"date_range"`
	GeneratedAt         time.Time                `json:"generated_at"`
}

// EntitySummary aggregates facts about a single entity.
type EntitySummary struct {
	Entity             string                  `json:"entity"`
	FactCount          int                     `json:"fact_count"`
	Properties         map[string][]FactValue  `json:"properties"`
	ContradictionCount int                     `json:"contradiction_count"`
	FirstFact          time.Time               `json:"first_fact"`
	LastFact           time.Time               `json:"last_fact"`
	Sources            map[string]int          `json:"sources"`
}

// FactValue represents a fact about entity/property.
type FactValue struct {
	Value      string     `json:"value"`
	FactID     string     `json:"fact_id"`
	ValidFrom  time.Time  `json:"valid_from"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`
	Source     string     `json:"source"`
}

// EntityGap represents an entity with few facts (potential gap in knowledge).
type EntityGap struct {
	Entity     string `json:"entity"`
	FactCount  int    `json:"fact_count"`
	Properties int    `json:"properties"`
}

// DateRange spans a time period.
type DateRange struct {
	From time.Time  `json:"from"`
	To   *time.Time `json:"to,omitempty"`
}
