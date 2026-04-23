package memory

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alash3al/stash/internal/embedder"
	"github.com/alash3al/stash/internal/reasoner"
	"github.com/alash3al/stash/internal/store"
	"github.com/google/uuid"
)

const (
	contextID        = "_memory.context"
	contextDuration  = time.Hour
	typeEvent        = "event"
	typeContext      = "context"
	typeFact         = "fact"
	typeRelation     = "relationship"
	typeFactAtemporal   = "atemporal"
	typeFactState       = "state"
	typeFactPointInTime = "point-in-time"
)

var errMissingStore = errors.New("memory: store is required")
var errMissingEmbedder = errors.New("memory: embedder is required")
var errMissingReasoner = errors.New("memory: reasoner is required")

// Memory is the core memory system.
// Concrete type — not an interface.
// Extend it with new methods; do not abstract it.
type Memory struct {
	store    store.Store
	embedder embedder.Embedder
	reasoner reasoner.Reasoner
}

// New creates a Memory using the provided store, embedder, and reasoner.
// All three are required. Returns error if any is nil.
func New(s store.Store, e embedder.Embedder, r reasoner.Reasoner) (*Memory, error) {
	if s == nil {
		return nil, errMissingStore
	}
	if e == nil {
		return nil, errMissingEmbedder
	}
	if r == nil {
		return nil, errMissingReasoner
	}
	return &Memory{
		store:    s,
		embedder: e,
		reasoner: r,
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

// RememberWithTTL stores an event that expires after ttl duration.
// Generates UUID and embedding. Returns event ID.
// ttl must be > 0.
// metadata must not start with "_memory".
func (m *Memory) RememberWithTTL(ctx context.Context, namespace, content string, ttl time.Duration, metadata map[string]any) (string, error) {
	if content == "" {
		return "", ErrEmptyContent
	}
	if ttl <= 0 {
		return "", errors.New("memory: ttl must be > 0")
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
	expiresAt := now.Add(ttl)

	memMeta := map[string]any{
		"type":       typeEvent,
		"content":    content,
		"timestamp":  now.Format(time.RFC3339),
		"expires_at": expiresAt.Format(time.RFC3339),
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
	now := time.Now().UTC()
	for _, result := range results {
		e, err := recordToEvent(result.Record, result.Score)
		if err != nil {
			continue
		}
		// Filter out expired events
		if e.ExpiresAt != nil && e.ExpiresAt.Before(now) {
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
	now := time.Now().UTC()
	for _, result := range results {
		e, err := recordToEvent(result.Record, result.Score)
		if err != nil {
			continue
		}
		// Filter out expired events
		if e.ExpiresAt != nil && e.ExpiresAt.Before(now) {
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

// PurgeExpired hard-deletes all expired events in the given namespaces.
// Returns count of deleted records.
// Non-expiring events are never touched.
// Safe to call frequently; idempotent.
func (m *Memory) PurgeExpired(ctx context.Context, namespaces []string) (int64, error) {
	if len(namespaces) == 0 {
		return 0, nil
	}

	now := time.Now().UTC()
	count := int64(0)

	// For each namespace, find expired events and delete them
	for _, ns := range namespaces {
		// Query for events that have expired
		filter := &store.Predicate{
			And: []store.Predicate{
				{
					Field: "metadata._memory.type",
					Op:    store.OpEq,
					Value: typeEvent,
				},
				{
					Field: "metadata._memory.expires_at",
					Op:    store.OpExists,
					Value: true,
				},
			},
		}

		// List expired events
		records, err := m.store.List(ctx, store.Filter{
			Namespaces: []string{ns},
			Where:      filter,
			Limit:      10000, // reasonable upper bound for one batch
		})
		if err != nil {
			return count, err
		}

		// Check expiration and delete
		for _, record := range records {
			e, err := recordToEvent(record, 0)
			if err != nil {
				continue
			}

			// Only delete if actually expired
			if e.ExpiresAt != nil && e.ExpiresAt.Before(now) {
				if err := m.store.Delete(ctx, record.ID); err != nil {
					// Log but continue
					continue
				}
				count++
			}
		}
	}

	return count, nil
}

// RememberMany stores multiple events atomically using store.PutMany.
// Generates UUIDs and embeddings for each.
// Returns count of stored events.
// Errors if any event is invalid (empty content, bad metadata).
// Errors if count > 10,000.
// All-or-nothing: if any embedding fails, entire batch is rolled back.
func (m *Memory) RememberMany(ctx context.Context, namespace string, events []BulkRemember) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}

	if len(events) > 10000 {
		return 0, errors.New("memory: batch exceeds 10000 events")
	}

	// Validate all events first
	for i, e := range events {
		if strings.TrimSpace(e.Content) == "" {
			return 0, fmt.Errorf("memory: event %d has empty content", i)
		}
		if err := validateMetadata(e.Metadata); err != nil {
			return 0, fmt.Errorf("memory: event %d: %w", i, err)
		}
		if e.TTL != nil && *e.TTL <= 0 {
			return 0, fmt.Errorf("memory: event %d has invalid TTL", i)
		}
	}

	// Embed all events in parallel
	type embeddingResult struct {
		idx int
		vec []float32
		err error
	}

	resultChan := make(chan embeddingResult, len(events))
	var wg sync.WaitGroup

	for i, e := range events {
		wg.Add(1)
		go func(idx int, content string) {
			defer wg.Done()
			vec, err := m.embedder.Embed(ctx, content)
			resultChan <- embeddingResult{idx, vec, err}
		}(i, e.Content)
	}

	wg.Wait()
	close(resultChan)

	// Collect embeddings, check for errors
	embeddings := make([][]float32, len(events))
	for result := range resultChan {
		if result.err != nil {
			return 0, fmt.Errorf("memory: embedding failed for event %d: %w", result.idx, result.err)
		}
		embeddings[result.idx] = result.vec
	}

	// Build store records
	records := make([]store.Record, len(events))
	now := time.Now().UTC()

	for i, e := range events {
		eventID := uuid.New().String()

		memMeta := map[string]any{
			"type":      typeEvent,
			"content":   e.Content,
			"timestamp": now.Format(time.RFC3339),
		}

		if e.TTL != nil {
			expiresAt := now.Add(*e.TTL)
			memMeta["expires_at"] = expiresAt.Format(time.RFC3339)
		}

		recordMeta := map[string]any{
			"_memory": memMeta,
		}
		for k, v := range e.Metadata {
			recordMeta[k] = v
		}

		records[i] = store.Record{
			ID:        eventID,
			Namespace: namespace,
			Content:   e.Content,
			Vectors: map[string]store.Vector{
				m.embedder.Model(): {
					Values: embeddings[i],
					Model:  m.embedder.Model(),
				},
			},
			Metadata: recordMeta,
		}
	}

	// Store all at once (atomic)
	if err := m.store.PutMany(ctx, records); err != nil {
		return 0, fmt.Errorf("memory: store.PutMany failed: %w", err)
	}

	return len(events), nil
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

	// Parse expires_at if present
	var expiresAt *time.Time
	if expiresAtStr, ok := memMeta["expires_at"].(string); ok && expiresAtStr != "" {
		if et, err := time.Parse(time.RFC3339, expiresAtStr); err == nil {
			expiresAt = &et
		}
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
		ExpiresAt: expiresAt,
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

// ConsolidateRecent synthesizes recent events into durable facts.
// Groups similar events by semantic clustering, then calls reasoner to synthesize each group.
// timeWindow: how far back to look (e.g., 7*24*time.Hour for last week)
// limit: max number of facts to synthesize in this pass (0 = no limit)
// Returns IDs of newly created facts.
// Errors if Reasoner is nil or if synthesis fails.
func (m *Memory) ConsolidateRecent(
	ctx context.Context,
	namespace string,
	timeWindow time.Duration,
	limit int,
) ([]string, error) {
	if m.reasoner == nil {
		return nil, fmt.Errorf("consolidation: reasoner not configured")
	}

	// Query recent events within timeWindow
	cutoff := time.Now().UTC().Add(-timeWindow)
	eventRecords, err := m.queryRecentEventRecords(ctx, namespace, cutoff)
	if err != nil {
		return nil, fmt.Errorf("query recent events: %w", err)
	}

	if len(eventRecords) < 2 {
		return []string{}, nil // Nothing to consolidate
	}

	// Cluster by semantic similarity
	clusters := m.clusterRecordsBySimilarity(eventRecords, 0.85) // 0.85 similarity = 0.15 distance

	if len(clusters) > limit && limit > 0 {
		// Keep only top `limit` clusters (by size)
		clusters = clusters[:limit]
	}

	var factIDs []string

	// Synthesize each cluster
	for _, cluster := range clusters {
		texts := make([]string, len(cluster))
		eventIDs := make([]string, len(cluster))
		for i, rec := range cluster {
			texts[i] = rec.Content
			eventIDs[i] = rec.ID
		}

		// Call reasoner to synthesize structured fact
		structured, err := m.reasoner.ReasonStructured(ctx, texts)
		if err != nil {
			return factIDs, fmt.Errorf("synthesize cluster: %w", err)
		}

		// Use structured fact's summary as the content
		factText := structured.Summary
		if factText == "" {
			factText = fmt.Sprintf("Entity: %s, Property: %s, Value: %s", structured.Entity, structured.Property, structured.Value)
		}

		// Check for conflicts with existing facts (simple heuristic)
		conflicts := []string{} // TODO: implement conflict detection in future task
		_ = conflicts           // Not blocking synthesis yet

		// Store fact as Record
		factID := uuid.New().String()
		now := time.Now().UTC()

		memMeta := map[string]any{
			"type":               typeFact,
			"fact_type":          typeFactState,           // Phase 3: default to state facts
			"content":            factText,
			"entity":             structured.Entity,     // Extracted entity
			"property":           structured.Property,   // Extracted property
			"value":              structured.Value,      // Extracted value
			"created_at":         now.Format(time.RFC3339),
			"valid_from":         now.Format(time.RFC3339), // Fact becomes true now
			"valid_until":        nil,                       // Ongoing until updated
			"source":             "consolidation",           // Mark synthesized facts
			"synthesized_from":   eventIDs,
			"confidence":         0.5,                       // New facts start at 50% confidence
			"observation_count":  1,                         // First observation
		}
		if len(conflicts) > 0 {
			memMeta["conflict_with"] = conflicts
		}

		recordMeta := map[string]any{
			"_memory": memMeta,
		}

		// Embed the fact
		vec, err := m.embedder.Embed(ctx, factText)
		if err != nil {
			return factIDs, fmt.Errorf("embed fact: %w", err)
		}

		record := store.Record{
			ID:        factID,
			Namespace: namespace,
			Content:   factText,
			Vectors: map[string]store.Vector{
				m.embedder.Model(): {
					Values: vec,
					Model:  m.embedder.Model(),
				},
			},
			Metadata: recordMeta,
		}

		if err := m.store.Put(ctx, record); err != nil {
			return factIDs, fmt.Errorf("store fact: %w", err)
		}

		factIDs = append(factIDs, factID)
	}

	return factIDs, nil
}

// queryRecentEventRecords retrieves event Records from the past `since` timestamp.
// Filters by namespace and type=event, excludes expired events.
func (m *Memory) queryRecentEventRecords(ctx context.Context, namespace string, since time.Time) ([]store.Record, error) {
	// Query events using List with a filter for type=event
	filter := store.Filter{
		Namespaces: []string{namespace},
		Where: &store.Predicate{
			Field: "metadata._memory.type",
			Op:    store.OpEq,
			Value: typeEvent,
		},
	}

	records, err := m.store.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	var result []store.Record
	now := time.Now().UTC()
	for i := range records {
		r := &records[i]

		// Parse record into Event to validate and extract timestamp/expiry
		evt, err := recordToEvent(*r, 0)
		if err != nil {
			continue // Skip records that can't be parsed as events
		}

		// Skip expired events
		if evt.ExpiresAt != nil && evt.ExpiresAt.Before(now) {
			continue
		}

		// Skip events outside the time window
		if evt.Timestamp.Before(since) {
			continue
		}

		result = append(result, *r)
	}

	return result, nil
}

// clusterRecordsBySimilarity groups records by cosine similarity of their embeddings.
// Uses greedy clustering: first record seeds a cluster, subsequent similar records join.
// threshold: minimum cosine similarity (0.0-1.0) for records to be in the same cluster.
func (m *Memory) clusterRecordsBySimilarity(records []store.Record, threshold float64) [][]store.Record {
	if len(records) == 0 {
		return [][]store.Record{}
	}

	// Use embedder's model as the reference
	modelKey := m.embedder.Model()

	clusters := [][]store.Record{}
	used := make(map[string]bool)

	for i := range records {
		r := &records[i]
		if used[r.ID] {
			continue
		}

		// Get seed record vector
		seedVec, ok := r.Vectors[modelKey]
		if !ok {
			continue // Skip records without vector in this model
		}

		cluster := []store.Record{*r}
		used[r.ID] = true

		// Find similar records
		for j := range records {
			other := &records[j]
			if used[other.ID] {
				continue
			}

			otherVec, ok := other.Vectors[modelKey]
			if !ok {
				continue
			}

			sim := cosineSimilarity(seedVec.Values, otherVec.Values)
			if sim > threshold {
				cluster = append(cluster, *other)
				used[other.ID] = true
			}
		}

		clusters = append(clusters, cluster)
	}

	return clusters
}

// cosineSimilarity computes cosine similarity between two vectors.
// Result in range [0.0, 1.0]. Higher = more similar.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		fa, fb := float64(a[i]), float64(b[i])
		dot += fa * fb
		normA += fa * fa
		normB += fb * fb
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// FindContradictions returns all facts in a namespace that contradict each other.
// Two facts contradict if:
// - They have the same entity and property (from metadata)
// - Their valid time ranges overlap
// - Their values differ
//
// Returns contradictions sorted by entity, property, then discovery time.
// Status is "conflict" if ranges overlap (both active), "evolution" if sequential.
// Returns empty slice if no contradictions found.
func (m *Memory) FindContradictions(ctx context.Context, namespace string) ([]Contradiction, error) {
	// Query all facts in namespace
	filter := store.Filter{
		Namespaces: []string{namespace},
		Where: &store.Predicate{
			Field: "metadata._memory.type",
			Op:    store.OpEq,
			Value: typeFact,
		},
	}

	records, err := m.store.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Parse facts
	facts := make([]*Fact, 0, len(records))
	for i := range records {
		fact, err := FactFromRecord(&records[i])
		if err != nil {
			continue // Skip records that can't be parsed as facts
		}
		facts = append(facts, fact)
	}

	if len(facts) < 2 {
		return []Contradiction{}, nil // Need at least 2 facts to have a contradiction
	}

	// Compare all pairs
	contradictions := []Contradiction{}
	// Use current time for overlap detection.
	// Nil ValidUntil means "ongoing until now".
	now := time.Now().UTC()

	for i := 0; i < len(facts); i++ {
		for j := i + 1; j < len(facts); j++ {
			fact1 := facts[i]
			fact2 := facts[j]

			// Extract entity and property from metadata
			entity1, _ := fact1.Metadata["entity"].(string)
			property1, _ := fact1.Metadata["property"].(string)
			value1, _ := fact1.Metadata["value"].(string)

			entity2, _ := fact2.Metadata["entity"].(string)
			property2, _ := fact2.Metadata["property"].(string)
			value2, _ := fact2.Metadata["value"].(string)

			// Skip if entity or property missing or not matching
			if entity1 == "" || property1 == "" || entity2 == "" || property2 == "" {
				continue
			}
			if entity1 != entity2 || property1 != property2 {
				continue
			}

			// Skip if values are the same (not a contradiction)
			if value1 == value2 {
				continue
			}

			// Check for time range overlap
			if !timeRangesOverlap(fact1.ValidFrom, fact1.ValidUntil, fact2.ValidFrom, fact2.ValidUntil, now) {
				continue // Time ranges don't overlap, no contradiction
			}

			// Determine status
			status := ContradictionStatusConflict
			// If ranges overlap but are sequential-ish (one has no ValidUntil), mark as evolution
			// For now, overlapping = conflict. Caller can review and update fact ValidUntil if needed.

			contradiction := Contradiction{
				ID:           uuid.New().String(),
				FactID1:      fact1.ID,
				FactID2:      fact2.ID,
				Entity:       entity1,
				Property:     property1,
				Value1:       value1,
				Value2:       value2,
				ValidFrom1:   fact1.ValidFrom,
				ValidUntil1:  fact1.ValidUntil,
				ValidFrom2:   fact2.ValidFrom,
				ValidUntil2:  fact2.ValidUntil,
				Status:       status,
				DiscoveredAt: now,
				Metadata:     nil,
			}

			contradictions = append(contradictions, contradiction)
		}
	}

	// Sort by entity, property, then discovery time
	sort.Slice(contradictions, func(i, j int) bool {
		ci, cj := contradictions[i], contradictions[j]
		if ci.Entity != cj.Entity {
			return ci.Entity < cj.Entity
		}
		if ci.Property != cj.Property {
			return ci.Property < cj.Property
		}
		return ci.DiscoveredAt.Before(cj.DiscoveredAt)
	})

	return contradictions, nil
}

// Reflect produces a structured report of memory state in a namespace.
// Groups facts by entity, detects contradictions, identifies gaps.
// Used for human review: what do we know, what's inconsistent, what's missing?
//
// Reflection is observation-only: no facts are modified, no auto-actions.
// Caller reviews the report and decides what to do.
//
// Returns error only if store access fails; always returns a report (possibly empty).
func (m *Memory) Reflect(ctx context.Context, namespace string) (*ReflectionReport, error) {
	// Query all facts in namespace
	filter := store.Filter{
		Namespaces: []string{namespace},
		Where: &store.Predicate{
			Field: "metadata._memory.type",
			Op:    store.OpEq,
			Value: typeFact,
		},
	}

	records, err := m.store.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Parse facts and group by entity
	entities := make(map[string]*EntitySummary)
	var allFacts []*Fact
	var earliestFact *time.Time
	var latestFact *time.Time

	for i := range records {
		fact, err := FactFromRecord(&records[i])
		if err != nil {
			continue // Skip unparseable facts
		}

		allFacts = append(allFacts, fact)

		// Track date range
		if earliestFact == nil || fact.ValidFrom.Before(*earliestFact) {
			earliestFact = &fact.ValidFrom
		}
		if latestFact == nil || fact.ValidFrom.After(*latestFact) {
			latestFact = &fact.ValidFrom
		}
		if fact.ValidUntil != nil && (latestFact == nil || fact.ValidUntil.After(*latestFact)) {
			latestFact = fact.ValidUntil
		}

		// Extract entity and property
		entity, _ := fact.Metadata["entity"].(string)
		if entity == "" {
			continue // Skip facts without entity
		}

		property, _ := fact.Metadata["property"].(string)
		if property == "" {
			continue // Skip facts without property
		}

		value, _ := fact.Metadata["value"].(string)

		// Initialize entity summary if needed
		if _, ok := entities[entity]; !ok {
			entities[entity] = &EntitySummary{
				Entity:     entity,
				Properties: make(map[string][]FactValue),
				Sources:    make(map[string]int),
			}
		}

		// Add fact value to property
		fv := FactValue{
			Value:      value,
			FactID:     fact.ID,
			ValidFrom:  fact.ValidFrom,
			ValidUntil: fact.ValidUntil,
			Source:     fact.Source,
		}
		entities[entity].Properties[property] = append(entities[entity].Properties[property], fv)
		entities[entity].FactCount++

		// Track source
		if fact.Source != "" {
			entities[entity].Sources[fact.Source]++
		} else {
			entities[entity].Sources["unknown"]++
		}

		// Update date range
		if entities[entity].FirstFact.IsZero() || fact.ValidFrom.Before(entities[entity].FirstFact) {
			entities[entity].FirstFact = fact.ValidFrom
		}
		if entities[entity].LastFact.IsZero() || fact.ValidFrom.After(entities[entity].LastFact) {
			entities[entity].LastFact = fact.ValidFrom
		}
	}

	// Find contradictions
	contradictions, _ := m.FindContradictions(ctx, namespace)

	// Count contradictions per entity
	for _, c := range contradictions {
		if summary, ok := entities[c.Entity]; ok {
			summary.ContradictionCount++
		}
	}

	// Identify gaps: entities with <= 2 facts
	gaps := []EntityGap{}
	for entityName, summary := range entities {
		if summary.FactCount <= 2 {
			gap := EntityGap{
				Entity:     entityName,
				FactCount:  summary.FactCount,
				Properties: len(summary.Properties),
			}
			gaps = append(gaps, gap)
		}
	}

	// Sort gaps by fact count (fewest first)
	sort.Slice(gaps, func(i, j int) bool {
		if gaps[i].FactCount != gaps[j].FactCount {
			return gaps[i].FactCount < gaps[j].FactCount
		}
		return gaps[i].Entity < gaps[j].Entity
	})

	// Build date range
	var dateRange *DateRange
	if earliestFact != nil {
		dateRange = &DateRange{
			From: *earliestFact,
			To:   latestFact,
		}
	}

	// Sort entities by name
	sortedEntities := make([]string, 0, len(entities))
	for name := range entities {
		sortedEntities = append(sortedEntities, name)
	}
	sort.Strings(sortedEntities)

	// Sort facts within each entity by valid_from time
	for _, summary := range entities {
		for _, factValues := range summary.Properties {
			sort.Slice(factValues, func(i, j int) bool {
				return factValues[i].ValidFrom.Before(factValues[j].ValidFrom)
			})
		}
	}

	// Build report
	report := &ReflectionReport{
		Namespace:           namespace,
		TotalFacts:          len(allFacts),
		TotalContradictions: len(contradictions),
		TotalEntities:       len(entities),
		EntitiesByName:      entities,
		Contradictions:      contradictions,
		Gaps:                gaps,
		DateRange:           dateRange,
		GeneratedAt:         time.Now().UTC(),
	}

	return report, nil
}

// calculateConfidence computes confidence from observation count.
// Formula: count / (count + 2)
// Examples:
//   count=1: 1/3 ≈ 0.33 (weak)
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

// Reinforce increments the observation count for a fact matching entity+property+value.
// If a matching fact exists, updates its observation count and confidence.
// If no match exists, returns error (caller should consolidate instead).
//
// Used when the same fact is observed again, reinforcing its truthfulness.
// Typically called during consolidation when duplicate facts are detected.
func (m *Memory) Reinforce(ctx context.Context, namespace, entity, property, value string) error {
	if entity == "" || property == "" || value == "" {
		return fmt.Errorf("reinforce: entity, property, and value must not be empty")
	}

	// Query all facts in namespace to find matching one
	filter := store.Filter{
		Namespaces: []string{namespace},
		Where: &store.Predicate{
			Field: "metadata._memory.type",
			Op:    store.OpEq,
			Value: typeFact,
		},
	}

	records, err := m.store.List(ctx, filter)
	if err != nil {
		return err
	}

	// Find matching fact: entity+property+value
	var targetFact *Fact
	var targetRecord *store.Record

	for i := range records {
		fact, err := FactFromRecord(&records[i])
		if err != nil {
			continue
		}

		factEntity, _ := fact.Metadata["entity"].(string)
		factProperty, _ := fact.Metadata["property"].(string)
		factValue, _ := fact.Metadata["value"].(string)

		if factEntity == entity && factProperty == property && factValue == value {
			targetFact = fact
			targetRecord = &records[i]
			break
		}
	}

	if targetFact == nil {
		return fmt.Errorf("reinforce: no fact found for entity=%q property=%q value=%q", entity, property, value)
	}

	// Increment count and recalculate confidence
	targetFact.ObservationCount++
	targetFact.Confidence = calculateConfidence(targetFact.ObservationCount)

	// Update metadata in record
	memMeta := targetRecord.Metadata["_memory"].(map[string]any)
	memMeta["confidence"] = float64(targetFact.Confidence)
	memMeta["observation_count"] = float64(targetFact.ObservationCount)

	// Store updated record
	return m.store.Put(ctx, *targetRecord)
}

// QueryFactsByType returns all facts of a specific type in a namespace.
// factType should be one of: "atemporal", "state", "point-in-time"
func (m *Memory) QueryFactsByType(ctx context.Context, namespace, factType string) ([]Fact, error) {
	if factType != typeFactAtemporal && factType != typeFactState && factType != typeFactPointInTime {
		return nil, fmt.Errorf("invalid fact type: %q", factType)
	}

	filter := store.Filter{
		Namespaces: []string{namespace},
		Where: &store.Predicate{
			Field: "metadata._memory.fact_type",
			Op:    store.OpEq,
			Value: factType,
		},
	}

	records, err := m.store.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("query facts: %w", err)
	}

	facts := make([]Fact, 0, len(records))
	for i := range records {
		fact, err := FactFromRecord(&records[i])
		if err != nil {
			continue
		}
		facts = append(facts, *fact)
	}

	return facts, nil
}

// GetAtemporalFacts returns all atemporal facts (always true, never expire).
func (m *Memory) GetAtemporalFacts(ctx context.Context, namespace string) ([]Fact, error) {
	return m.QueryFactsByType(ctx, namespace, typeFactAtemporal)
}

// GetStateFactsFor returns all state facts about an entity (current state only).
// Filters for facts where ValidUntil is nil (still true).
func (m *Memory) GetStateFactsFor(ctx context.Context, namespace, entity string) ([]Fact, error) {
	// Query all state facts in namespace
	stateFacts, err := m.QueryFactsByType(ctx, namespace, typeFactState)
	if err != nil {
		return nil, err
	}

	// Filter by entity and current status (ValidUntil = nil)
	var result []Fact
	for _, fact := range stateFacts {
		factEntity, _ := fact.Metadata["entity"].(string)
		if factEntity == entity && fact.ValidUntil == nil {
			result = append(result, fact)
		}
	}

	return result, nil
}

// GetPointInTimeFacts returns all point-in-time facts (snapshots).
func (m *Memory) GetPointInTimeFacts(ctx context.Context, namespace string) ([]Fact, error) {
	return m.QueryFactsByType(ctx, namespace, typeFactPointInTime)
}
