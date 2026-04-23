package memory

import (
	"time"
)

// Event represents something that happened at a specific point in time.
// Stored as a store.Record with _memory.type = "event".
type Event struct {
	ID        string
	Namespace string
	Content   string
	Timestamp time.Time
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
