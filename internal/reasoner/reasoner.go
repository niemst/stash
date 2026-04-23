// Package reasoner synthesizes structured reasoning over text.
// Implementations: OpenAI (production), Fake (tests).
package reasoner

import (
	"context"
)

// StructuredFact represents an extracted fact with entity, property, and value.
type StructuredFact struct {
	// Entity is the subject (e.g., "Alice", "Bob").
	Entity string
	// Property is the attribute or predicate (e.g., "role", "location").
	Property string
	// Value is the fact value (e.g., "engineer", "Paris").
	Value string
	// Summary is the full natural language fact statement.
	Summary string
}

// StructuredRelationship represents an extracted relationship between two entities.
type StructuredRelationship struct {
	// FromEntity is the source entity (e.g., "Alice").
	FromEntity string
	// RelationType is the relationship type (e.g., "works_at", "located_in").
	RelationType string
	// ToEntity is the target entity (e.g., "TechCorp").
	ToEntity string
	// Confidence is how confident the LLM is in this relationship (0.0-1.0).
	Confidence float32
}

// Reasoner synthesizes structured reasoning over text input.
// Implementations: OpenAI (production), Fake (tests).
type Reasoner interface {
	// Reason takes a list of text inputs and returns synthesized reasoning output.
	// Implementation determines how to combine inputs, query the LLM, and format the result.
	Reason(ctx context.Context, texts []string) (string, error)

	// ReasonStructured takes a list of text inputs and returns a structured fact.
	// Attempts to extract entity, property, and value from the LLM response.
	// Falls back to StructuredFact with empty entity/property/value if extraction fails.
	ReasonStructured(ctx context.Context, texts []string) (*StructuredFact, error)

	// ReasonRelationships takes a fact and extracts relationships between entities.
	// Returns a slice of relationships found in the fact.
	// Returns empty slice if no relationships found.
	ReasonRelationships(ctx context.Context, factContent string) ([]*StructuredRelationship, error)

	// Model returns the model identifier as passed at construction.
	// Examples: "gpt-4o-mini", "gpt-4".
	// Used for logging and debugging.
	Model() string

	// Driver returns the driver name as passed at construction.
	// Examples: "openai".
	Driver() string
}
