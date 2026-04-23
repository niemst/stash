package memory

import (
	"context"
	"errors"
	"time"

	"github.com/alash3al/stash/internal/embedder"
	"github.com/alash3al/stash/internal/store"
	"github.com/google/uuid"
)

const (
	contextID       = "_memory.context"
	contextDuration = time.Hour
	typeEvent       = "event"
	typeContext     = "context"
)

var errMissingStore = errors.New("memory: store is required")
var errMissingEmbedder = errors.New("memory: embedder is required")

// Memory is the core memory system.
// Concrete type — not an interface.
// Extend it with new methods; do not abstract it.
type Memory struct {
	store    store.Store
	embedder embedder.Embedder
}

// New creates a Memory using the provided store and embedder.
// Both are required. Returns error if either is nil.
func New(s store.Store, e embedder.Embedder) (*Memory, error) {
	if s == nil {
		return nil, errMissingStore
	}
	if e == nil {
		return nil, errMissingEmbedder
	}
	return &Memory{
		store:    s,
		embedder: e,
	}, nil
}

// Remember stores an event with its embedding.
// Generates a UUID v4 event ID before calling store.Put.
// Returns the generated event ID on success.
// content must not be empty.
// metadata keys must not start with "_memory" (returns ErrInvalidMetadata).
func (m *Memory) Remember(ctx context.Context, namespace, content string, metadata map[string]any) (string, error) {
	if content == "" {
		return "", ErrEmptyContent
	}
	if err := validateMetadata(metadata); err != nil {
		return "", err
	}

	vec, err := m.embedder.Embed(ctx, content)
	if err != nil {
		return "", err
	}

	eventID := uuid.New().String()
	now := time.Now().UTC()

	memMeta := map[string]any{
		"type":      typeEvent,
		"content":   content,
		"timestamp": now.Format(time.RFC3339),
	}

	recordMeta := map[string]any{
		"_memory": memMeta,
	}
	for k, v := range metadata {
		recordMeta[k] = v
	}

	record := store.Record{
		ID:        eventID,
		Namespace: namespace,
		Content:   content,
		Vectors: map[string]store.Vector{
			m.embedder.Model(): {
				Values: vec,
				Model:  m.embedder.Model(),
			},
		},
		Metadata: recordMeta,
	}

	if err := m.store.Put(ctx, record); err != nil {
		return "", err
	}

	return eventID, nil
}

// Recall retrieves events relevant to a query.
// Embeds the query, searches the store by vector similarity.
// Returns at most limit events ordered by relevance.
// Returns empty slice (not error) when nothing matches.
// limit must be > 0.
func (m *Memory) Recall(ctx context.Context, namespaces []string, query string, limit int) ([]Event, error) {
	if limit <= 0 {
		return nil, ErrInvalidLimit
	}

	vec, err := m.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	results, err := m.store.Search(ctx, store.Query{
		Namespaces: namespaces,
		Vector:     vec,
		VectorName: m.embedder.Model(),
		TopK:       limit,
		Filter: &store.Predicate{
			Field: "metadata._memory.type",
			Op:    store.OpEq,
			Value: typeEvent,
		},
	})
	if err != nil {
		return nil, err
	}

	events := make([]Event, 0, len(results))
	for _, result := range results {
		e, err := recordToEvent(result.Record, result.Score)
		if err != nil {
			continue
		}
		events = append(events, e)
	}

	return events, nil
}

// RecallWhere retrieves events matching both semantic similarity and structured metadata.
// Combines vector search with optional predicate filtering.
// If filter is nil, behaves identically to Recall().
// Returns at most limit events ordered by relevance (score descending).
// limit must be > 0.
func (m *Memory) RecallWhere(ctx context.Context, namespaces []string, query string, filter *store.Predicate, limit int) ([]Event, error) {
	if limit <= 0 {
		return nil, ErrInvalidLimit
	}

	vec, err := m.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	// Build a compound predicate: event type AND user filter
	typeFilter := &store.Predicate{
		Field: "metadata._memory.type",
		Op:    store.OpEq,
		Value: typeEvent,
	}

	var combinedFilter *store.Predicate
	if filter == nil {
		combinedFilter = typeFilter
	} else {
		// AND together: type=event AND user_filter
		combinedFilter = &store.Predicate{
			And: []store.Predicate{*typeFilter, *filter},
		}
	}

	results, err := m.store.Search(ctx, store.Query{
		Namespaces: namespaces,
		Vector:     vec,
		VectorName: m.embedder.Model(),
		TopK:       limit,
		Filter:     combinedFilter,
	})
	if err != nil {
		return nil, err
	}

	events := make([]Event, 0, len(results))
	for _, result := range results {
		e, err := recordToEvent(result.Record, result.Score)
		if err != nil {
			continue
		}
		events = append(events, e)
	}

	return events, nil
}

// WorkingMemory returns the current working memory state.
// Creates a new working memory if none exists.
// Replaces the working memory (lazy) if the existing one has expired.
// When input is non-empty and a valid context exists, updates the focus,
// re-searches for relevant events, and resets the expiry.
// Empty input returns existing context unchanged.
// Does not start background goroutines.
func (m *Memory) WorkingMemory(ctx context.Context, namespace, input string) (WorkingMemory, error) {
	// Context ID is namespaced
	contextIDWithNamespace := namespace + ":" + contextID
	
	record, err := m.store.Get(ctx, contextIDWithNamespace)
	if errors.Is(err, store.ErrNotFound) {
		return m.createWorkingMemory(ctx, namespace, input)
	}
	if err != nil {
		return WorkingMemory{}, err
	}

	wm, err := recordToWorkingMemory(record)
	if err != nil {
		return WorkingMemory{}, err
	}

	if time.Now().UTC().After(wm.ExpiresAt) {
		return m.createWorkingMemory(ctx, namespace, input)
	}

	if input != "" {
		return m.updateWorkingMemory(ctx, namespace, wm, input)
	}

	// input == "" → return existing context unchanged
	return wm, nil
}

// LinkEvents creates a directed relationship from fromID to toID.
// Returns the relation ID.
// Both events must exist in the namespace (validated).
// relationType must be one of the known types.
// metadata must not contain "_memory" keys.
func (m *Memory) LinkEvents(
	ctx context.Context,
	namespace string,
	fromID string,
	toID string,
	relationType string,
	metadata map[string]any,
) (string, error) {
	// Validate inputs
	if fromID == "" || toID == "" {
		return "", ErrEmptyContent // Reuse for missing ID
	}

	if fromID == toID {
		return "", errors.New("memory: cannot link event to itself")
	}

	if err := validateMetadata(metadata); err != nil {
		return "", err
	}

	// Verify both events exist
	if _, err := m.store.Get(ctx, fromID); err != nil {
		return "", errors.New("memory: from_event not found")
	}
	if _, err := m.store.Get(ctx, toID); err != nil {
		return "", errors.New("memory: to_event not found")
	}

	// Validate relation type (allow any string for extensibility, but document the standard types)
	if relationType == "" {
		return "", errors.New("memory: relation_type cannot be empty")
	}

	now := time.Now().UTC()
	relationID := uuid.New().String()

	memMeta := map[string]any{
		"type":           "relationship",
		"from_event_id":  fromID,
		"to_event_id":    toID,
		"relation_type":  relationType,
		"created_at":     now.Format(time.RFC3339),
	}

	recordMeta := map[string]any{
		"_memory": memMeta,
	}
	for k, v := range metadata {
		recordMeta[k] = v
	}

	record := store.Record{
		ID:        relationID,
		Namespace: namespace,
		Content:   "", // relationships are metadata-only
		Metadata:  recordMeta,
	}

	if err := m.store.Put(ctx, record); err != nil {
		return "", err
	}

	return relationID, nil
}

// FindRelated retrieves all events that are related to eventID by relationType.
// Returns events that satisfy: exists relation where from_event=eventID AND type=relationType.
// Returns empty slice if no relations found.
func (m *Memory) FindRelated(
	ctx context.Context,
	namespace string,
	eventID string,
	relationType string,
) ([]Event, error) {
	if eventID == "" {
		return nil, ErrEmptyContent
	}
	if relationType == "" {
		return nil, errors.New("memory: relation_type cannot be empty")
	}

	// Query for relationship records
	filter := &store.Predicate{
		And: []store.Predicate{
			{
				Field: "metadata._memory.type",
				Op:    store.OpEq,
				Value: "relationship",
			},
			{
				Field: "metadata._memory.from_event_id",
				Op:    store.OpEq,
				Value: eventID,
			},
			{
				Field: "metadata._memory.relation_type",
				Op:    store.OpEq,
				Value: relationType,
			},
		},
	}

	// List all matching relationship records
	relationships, err := m.store.List(ctx, store.Filter{
		Namespaces: []string{namespace},
		Where:      filter,
		Limit:      10000, // reasonable upper bound
	})
	if err != nil {
		return nil, err
	}

	if len(relationships) == 0 {
		return []Event{}, nil
	}

	// Extract toEventIDs from relationships
	toEventIDs := make([]string, 0, len(relationships))
	for _, rel := range relationships {
		memMeta, ok := rel.Metadata["_memory"].(map[string]any)
		if !ok {
			continue
		}
		toID, ok := memMeta["to_event_id"].(string)
		if ok && toID != "" {
			toEventIDs = append(toEventIDs, toID)
		}
	}

	if len(toEventIDs) == 0 {
		return []Event{}, nil
	}

	// Fetch the actual events
	events := make([]Event, 0, len(toEventIDs))
	for _, toID := range toEventIDs {
		record, err := m.store.Get(ctx, toID)
		if err != nil {
			// Skip events that don't exist (orphaned relationships)
			continue
		}
		e, err := recordToEvent(record, 0) // score is 0 for related events (not from search)
		if err != nil {
			continue
		}
		events = append(events, e)
	}

	return events, nil
}

// Close releases any resources held by Memory.
func (m *Memory) Close() error {
	return nil
}

func (m *Memory) createWorkingMemory(ctx context.Context, namespace, focus string) (WorkingMemory, error) {
	now := time.Now().UTC()
	contextIDWithNamespace := namespace + ":" + contextID
	
	wm := WorkingMemory{
		ID:        contextIDWithNamespace,
		Focus:     focus,
		EventIDs:  nil,
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: now.Add(contextDuration),
	}

	if focus != "" {
		events, err := m.Recall(ctx, []string{namespace}, focus, 10)
		if err == nil {
			wm.EventIDs = make([]string, 0, len(events))
			for _, e := range events {
				wm.EventIDs = append(wm.EventIDs, e.ID)
			}
		}
	}

	recordMeta := map[string]any{
		"_memory": map[string]any{
			"type":       typeContext,
			"focus":      focus,
			"event_ids":  wm.EventIDs,
			"created_at": wm.CreatedAt.Format(time.RFC3339),
			"updated_at": wm.UpdatedAt.Format(time.RFC3339),
			"expires_at": wm.ExpiresAt.Format(time.RFC3339),
		},
	}

	record := store.Record{
		ID:        contextIDWithNamespace,
		Namespace: namespace,
		Metadata:  recordMeta,
	}

	if err := m.store.Put(ctx, record); err != nil {
		return WorkingMemory{}, err
	}

	return wm, nil
}

func (m *Memory) updateWorkingMemory(ctx context.Context, namespace string, existing WorkingMemory, focus string) (WorkingMemory, error) {
	now := time.Now().UTC()
	contextIDWithNamespace := namespace + ":" + contextID

	wm := WorkingMemory{
		ID:        contextIDWithNamespace,
		Focus:     focus,
		CreatedAt: existing.CreatedAt,
		UpdatedAt: now,
		ExpiresAt: now.Add(contextDuration),
	}

	events, err := m.Recall(ctx, []string{namespace}, focus, 10)
	if err == nil {
		wm.EventIDs = make([]string, 0, len(events))
		for _, e := range events {
			wm.EventIDs = append(wm.EventIDs, e.ID)
		}
	}

	recordMeta := map[string]any{
		"_memory": map[string]any{
			"type":       typeContext,
			"focus":      focus,
			"event_ids":  wm.EventIDs,
			"created_at": wm.CreatedAt.Format(time.RFC3339),
			"updated_at": wm.UpdatedAt.Format(time.RFC3339),
			"expires_at": wm.ExpiresAt.Format(time.RFC3339),
		},
	}

	record := store.Record{
		ID:        contextIDWithNamespace,
		Namespace: namespace,
		Metadata:  recordMeta,
	}

	if err := m.store.Put(ctx, record); err != nil {
		return WorkingMemory{}, err
	}

	return wm, nil
}

func validateMetadata(metadata map[string]any) error {
	if metadata == nil {
		return nil
	}
	for k := range metadata {
		if hasMemoryPrefix(k) {
			return ErrInvalidMetadata
		}
	}
	return nil
}

// cosineSimilarity returns the cosine similarity between two vectors.
// Both vectors must have the same length.

func hasMemoryPrefix(key string) bool {
	return len(key) >= 7 && key[:7] == "_memory"
}

func recordToEvent(r store.Record, score float32) (Event, error) {
	memMeta, ok := r.Metadata["_memory"].(map[string]any)
	if !ok {
		return Event{}, ErrEventNotFound
	}

	content, _ := memMeta["content"].(string)
	tsStr, _ := memMeta["timestamp"].(string)
	var timestamp time.Time
	if tsStr != "" {
		timestamp, _ = time.Parse(time.RFC3339, tsStr)
	}

	callerMeta := make(map[string]any)
	for k, v := range r.Metadata {
		if k != "_memory" {
			callerMeta[k] = v
		}
	}

	return Event{
		ID:        r.ID,
		Namespace: r.Namespace,
		Content:   content,
		Timestamp: timestamp,
		Metadata:  callerMeta,
		Score:     score,
	}, nil
}

func recordToWorkingMemory(r store.Record) (WorkingMemory, error) {
	memMeta, ok := r.Metadata["_memory"].(map[string]any)
	if !ok {
		return WorkingMemory{}, ErrEventNotFound
	}

	focus, _ := memMeta["focus"].(string)
	eventIDs := parseStringSlice(memMeta["event_ids"])

	createdAtStr, _ := memMeta["created_at"].(string)
	updatedAtStr, _ := memMeta["updated_at"].(string)
	expiresAtStr, _ := memMeta["expires_at"].(string)

	var createdAt, updatedAt, expiresAt time.Time
	if createdAtStr != "" {
		createdAt, _ = time.Parse(time.RFC3339, createdAtStr)
	}
	if updatedAtStr != "" {
		updatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
	}
	if expiresAtStr != "" {
		expiresAt, _ = time.Parse(time.RFC3339, expiresAtStr)
	}

	return WorkingMemory{
		ID:        r.ID,
		Focus:     focus,
		EventIDs:  eventIDs,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		ExpiresAt: expiresAt,
	}, nil
}

func parseStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	if ss, ok := v.([]string); ok {
		return ss
	}
	if si, ok := v.([]any); ok {
		result := make([]string, 0, len(si))
		for _, item := range si {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}
