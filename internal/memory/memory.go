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
func (m *Memory) Remember(ctx context.Context, content string, metadata map[string]any) (string, error) {
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
		ID:      eventID,
		Content: content,
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
func (m *Memory) Recall(ctx context.Context, query string, limit int) ([]Event, error) {
	if limit <= 0 {
		return nil, ErrInvalidLimit
	}

	vec, err := m.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	results, err := m.store.Search(ctx, store.Query{
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

// WorkingMemory returns the current working memory state.
// Creates a new working memory if none exists.
// Replaces the working memory (lazy) if the existing one has expired.
// When input is non-empty and a valid context exists, updates the focus,
// re-searches for relevant events, and resets the expiry.
// Empty input returns existing context unchanged.
// Does not start background goroutines.
func (m *Memory) WorkingMemory(ctx context.Context, input string) (WorkingMemory, error) {
	record, err := m.store.Get(ctx, contextID)
	if errors.Is(err, store.ErrNotFound) {
		return m.createWorkingMemory(ctx, input)
	}
	if err != nil {
		return WorkingMemory{}, err
	}

	wm, err := recordToWorkingMemory(record)
	if err != nil {
		return WorkingMemory{}, err
	}

	if time.Now().UTC().After(wm.ExpiresAt) {
		return m.createWorkingMemory(ctx, input)
	}

	if input != "" {
		return m.updateWorkingMemory(ctx, wm, input)
	}

	// input == "" → return existing context unchanged
	return wm, nil
}

// Close releases any resources held by Memory.
func (m *Memory) Close() error {
	return nil
}

func (m *Memory) createWorkingMemory(ctx context.Context, focus string) (WorkingMemory, error) {
	now := time.Now().UTC()
	wm := WorkingMemory{
		ID:        contextID,
		Focus:     focus,
		EventIDs:  nil,
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: now.Add(contextDuration),
	}

	if focus != "" {
		events, err := m.Recall(ctx, focus, 10)
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
		ID:       contextID,
		Metadata: recordMeta,
	}

	if err := m.store.Put(ctx, record); err != nil {
		return WorkingMemory{}, err
	}

	return wm, nil
}

func (m *Memory) updateWorkingMemory(ctx context.Context, existing WorkingMemory, focus string) (WorkingMemory, error) {
	now := time.Now().UTC()
	wm := WorkingMemory{
		ID:        contextID,
		Focus:     focus,
		CreatedAt: existing.CreatedAt,
		UpdatedAt: now,
		ExpiresAt: now.Add(contextDuration),
	}

	events, err := m.Recall(ctx, focus, 10)
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
		ID:       contextID,
		Metadata: recordMeta,
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
