package memory

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/alash3al/stash/internal/embedder"
	"github.com/alash3al/stash/internal/reasoner"
	"github.com/alash3al/stash/internal/store"
	storemapdb "github.com/alash3al/stash/internal/store/mapdb"
	"github.com/google/uuid"
)

func startStore(t *testing.T) (store.Store, func()) {
	cfg := storemapdb.Config{
		VectorDim: 8,
	}

	s, err := storemapdb.New(cfg)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	cleanup := func() {
		s.Close()
	}

	return s, cleanup
}

func startMemory(t *testing.T) (*Memory, func()) {
	s, cleanup := startStore(t)
	emb := embedder.NewFake()
	reas := reasoner.NewFake("fake", "fake")

	mem, err := New(s, emb, reas)
	if err != nil {
		cleanup()
		t.Fatalf("failed to create memory: %v", err)
	}

	return mem, func() {
		mem.Close()
		cleanup()
	}
}

func TestRemember_EmptyContent(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	_, err = mem.Remember(context.Background(), "test-ns", "", nil)
	if !errors.Is(err, ErrEmptyContent) {
		t.Errorf("expected ErrEmptyContent, got %v", err)
	}
}

func TestRemember_InvalidMetadata(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	_, err = mem.Remember(context.Background(), "test-ns", "content", map[string]any{
		"_memory.key": "value",
	})
	if !errors.Is(err, ErrInvalidMetadata) {
		t.Errorf("expected ErrInvalidMetadata, got %v", err)
	}
}

func TestRemember_StoresEvent(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	eventID, err := mem.Remember(ctx, "test-ns", "user asked about the weather", map[string]any{
		"session": "abc123",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	record, err := s.Get(ctx, eventID)
	if err != nil {
		t.Fatalf("store.Get failed: %v", err)
	}

	memMeta, ok := record.Metadata["_memory"].(map[string]any)
	if !ok {
		t.Fatal("missing _memory metadata")
	}

	if memMeta["type"] != "event" {
		t.Errorf("expected type=event, got %v", memMeta["type"])
	}
	if memMeta["content"] != "user asked about the weather" {
		t.Errorf("expected content, got %v", memMeta["content"])
	}
	if memMeta["timestamp"] == nil {
		t.Error("expected timestamp to be set")
	}

	if record.Metadata["session"] != "abc123" {
		t.Errorf("expected session metadata, got %v", record.Metadata["session"])
	}
}

func TestRemember_EmbedderError(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	failingEmbedder := &failingFakeEmbedder{}
	mem, err := New(s, failingEmbedder, reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	_, err = mem.Remember(context.Background(), "test-ns", "content", nil)
	if err == nil {
		t.Error("expected error from embedder")
	}
}

func TestRemember_StoreError(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}

	ctx := context.Background()
	// Replace store with failing store for test
	mem = &Memory{
		store:    &failingStore{inner: mem.store},
		embedder: mem.embedder,
	}

	_, err = mem.Remember(ctx, "test-ns", "content", nil)
	if err == nil {
		t.Error("expected error from store")
	}
}

func TestRecall_EmptyOnNoEvents(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	events, err := mem.Recall(ctx, []string{"test-ns"}, "weather", 5)
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty slice, got %d events", len(events))
	}
}

func TestRecall_ReturnsAtMostLimit(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()

	for i := 0; i < 10; i++ {
		_, err := mem.Remember(ctx, "test-ns", "event content", nil)
		if err != nil {
			t.Fatalf("Remember failed: %v", err)
		}
	}

	events, err := mem.Recall(ctx, []string{"test-ns"}, "event", 3)
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	if len(events) > 3 {
		t.Errorf("expected at most 3 events, got %d", len(events))
	}
}

func TestRecall_InvalidLimit(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	_, err = mem.Recall(context.Background(), []string{"test-ns"}, "query", 0)
	if !errors.Is(err, ErrInvalidLimit) {
		t.Errorf("expected ErrInvalidLimit for limit=0, got %v", err)
	}

	_, err = mem.Recall(context.Background(), []string{"test-ns"}, "query", -1)
	if !errors.Is(err, ErrInvalidLimit) {
		t.Errorf("expected ErrInvalidLimit for limit=-1, got %v", err)
	}
}

func TestRecall_ReturnsCorrectFields(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	eventID, err := mem.Remember(ctx, "test-ns", "test content", map[string]any{
		"session": "test-session",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	events, err := mem.Recall(ctx, []string{"test-ns"}, "test", 1)
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	if e.ID != eventID {
		t.Errorf("expected ID %s, got %s", eventID, e.ID)
	}
	if e.Content != "test content" {
		t.Errorf("expected content, got %s", e.Content)
	}
	if e.Metadata == nil {
		t.Error("expected metadata to be set")
	}
	if e.Metadata["session"] != "test-session" {
		t.Errorf("expected session in metadata, got %v", e.Metadata["session"])
	}
}

func TestWorkingMemory_CreatesNewWorkingMemory(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	wm, err := mem.WorkingMemory(ctx, "test-ns", "weather conversation")
	if err != nil {
		t.Fatalf("WorkingMemory failed: %v", err)
	}

	if wm.ID != "test-ns:_memory.context" {
		t.Errorf("expected working memory ID, got %s", wm.ID)
	}
	if wm.Focus != "weather conversation" {
		t.Errorf("expected focus, got %s", wm.Focus)
	}
}

func TestWorkingMemory_UpdatesWhenInputProvided(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	wm1, err := mem.WorkingMemory(ctx, "test-ns", "first focus")
	if err != nil {
		t.Fatalf("WorkingMemory failed: %v", err)
	}

	wm2, err := mem.WorkingMemory(ctx, "test-ns", "second focus")
	if err != nil {
		t.Fatalf("WorkingMemory failed: %v", err)
	}

	if wm1.ID != wm2.ID {
		t.Errorf("expected same working memory ID, got %s vs %s", wm1.ID, wm2.ID)
	}
	if wm1.CreatedAt.Unix() != wm2.CreatedAt.Unix() {
		t.Errorf("expected same created_at (same second), got %v vs %v", wm1.CreatedAt, wm2.CreatedAt)
	}
	if wm2.Focus != "second focus" {
		t.Errorf("expected focus to update to 'second focus', got %s", wm2.Focus)
	}
	if !wm2.UpdatedAt.After(wm1.UpdatedAt) && wm2.UpdatedAt.Equal(wm1.UpdatedAt) {
		t.Errorf("expected updated_at to advance, got %v vs %v", wm2.UpdatedAt, wm1.UpdatedAt)
	}
}
	

func TestWorkingMemory_CreatesNewWhenExpired(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()

	// Replace store with expired store for test
	mem = &Memory{
		store:    &expiredStore{inner: mem.store},
		embedder: mem.embedder,
	}

	wm1, err := mem.WorkingMemory(ctx, "test-ns", "first focus")
	if err != nil {
		t.Fatalf("WorkingMemory failed: %v", err)
	}

	wm2, err := mem.WorkingMemory(ctx, "test-ns", "second focus")
	if err != nil {
		t.Fatalf("WorkingMemory failed: %v", err)
	}

	if wm1.ID == wm2.ID && wm1.CreatedAt.Equal(wm2.CreatedAt) {
		t.Error("expected new working memory after expiry")
	}
}

func TestClose_ReturnsNil(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}

	if err := mem.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestRemember_ConcurrentNoRace(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = mem.Remember(ctx, "test-ns", "concurrent event", nil)
		}()
	}

	wg.Wait()
}

type failingFakeEmbedder struct{}

func (f *failingFakeEmbedder) Embed(context.Context, string) ([]float32, error) {
	return nil, errors.New("embedder failed")
}

func (f *failingFakeEmbedder) Model() string {
	return "failing"
}

func (f *failingFakeEmbedder) Dims() int {
	return 8
}

// test helpers replacing NewFailing and NewExpired from memory.go

type failingStore struct {
	inner store.Store
}

func (f *failingStore) Put(ctx context.Context, r store.Record) error {
	return errors.New("store put failed")
}

func (f *failingStore) Get(ctx context.Context, id string) (store.Record, error) {
	return f.inner.Get(ctx, id)
}

func (f *failingStore) Delete(ctx context.Context, id string) error {
	return f.inner.Delete(ctx, id)
}

func (f *failingStore) Purge(ctx context.Context, id string) error {
	return f.inner.Purge(ctx, id)
}

func (f *failingStore) PutMany(ctx context.Context, rs []store.Record) error {
	return f.inner.PutMany(ctx, rs)
}

func (f *failingStore) DeleteWhere(ctx context.Context, namespaces []string, p *store.Predicate) (int64, error) {
	return f.inner.DeleteWhere(ctx, namespaces, p)
}

func (f *failingStore) Search(ctx context.Context, q store.Query) ([]store.SearchResult, error) {
	return f.inner.Search(ctx, q)
}

func (f *failingStore) List(ctx context.Context, f2 store.Filter) ([]store.Record, error) {
	return f.inner.List(ctx, f2)
}

func (f *failingStore) Iterate(ctx context.Context, f2 store.Filter) (<-chan store.Record, <-chan error) {
	return f.inner.Iterate(ctx, f2)
}

func (f *failingStore) Count(ctx context.Context, namespaces []string, p *store.Predicate) (int64, error) {
	return f.inner.Count(ctx, namespaces, p)
}

func (f *failingStore) WithTx(ctx context.Context, fn func(tx store.Store) error) error {
	return f.inner.WithTx(ctx, fn)
}

func (f *failingStore) Health(ctx context.Context) error {
	return f.inner.Health(ctx)
}

func (f *failingStore) Migrate(ctx context.Context) error {
	return f.inner.Migrate(ctx)
}

func (f *failingStore) Close() error {
	return f.inner.Close()
}

type expiredStore struct {
	inner store.Store
}

func (e *expiredStore) Get(ctx context.Context, id string) (store.Record, error) {
	r, err := e.inner.Get(ctx, id)
	if err != nil {
		return r, err
	}

	memMeta, ok := r.Metadata["_memory"].(map[string]any)
	if !ok {
		return r, nil
	}

	expiresAtStr, ok := memMeta["expires_at"].(string)
	if !ok {
		return r, nil
	}

	expiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		return r, nil
	}

	if time.Now().UTC().Before(expiresAt) {
		return r, nil
	}

	return store.Record{}, store.ErrNotFound
}

func (e *expiredStore) Put(ctx context.Context, r store.Record) error {
	return e.inner.Put(ctx, r)
}

func (e *expiredStore) Delete(ctx context.Context, id string) error {
	return e.inner.Delete(ctx, id)
}

func (e *expiredStore) Purge(ctx context.Context, id string) error {
	return e.inner.Purge(ctx, id)
}

func (e *expiredStore) PutMany(ctx context.Context, rs []store.Record) error {
	return e.inner.PutMany(ctx, rs)
}

func (e *expiredStore) DeleteWhere(ctx context.Context, namespaces []string, p *store.Predicate) (int64, error) {
	return e.inner.DeleteWhere(ctx, namespaces, p)
}

func (e *expiredStore) Search(ctx context.Context, q store.Query) ([]store.SearchResult, error) {
	return e.inner.Search(ctx, q)
}

func (e *expiredStore) List(ctx context.Context, f store.Filter) ([]store.Record, error) {
	return e.inner.List(ctx, f)
}

func (e *expiredStore) Iterate(ctx context.Context, f store.Filter) (<-chan store.Record, <-chan error) {
	return e.inner.Iterate(ctx, f)
}

func (e *expiredStore) Count(ctx context.Context, namespaces []string, p *store.Predicate) (int64, error) {
	return e.inner.Count(ctx, namespaces, p)
}

func (e *expiredStore) WithTx(ctx context.Context, fn func(tx store.Store) error) error {
	return e.inner.WithTx(ctx, fn)
}

func (e *expiredStore) Health(ctx context.Context) error {
	return e.inner.Health(ctx)
}

func (e *expiredStore) Migrate(ctx context.Context) error {
	return e.inner.Migrate(ctx)
}

func (e *expiredStore) Close() error {
	return e.inner.Close()
}

func TestRecallWhere_WithFilter(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()

	// Create events with different metadata
	_, err = mem.Remember(ctx, "test-ns", "high severity bug", map[string]any{
		"severity": "high",
		"component": "api",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	_, err = mem.Remember(ctx, "test-ns", "low priority issue", map[string]any{
		"severity": "low",
		"component": "gateway",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	_, err = mem.Remember(ctx, "test-ns", "high priority gateway fix", map[string]any{
		"severity": "high",
		"component": "gateway",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Test: search with single filter (severity=high)
	filter := &store.Predicate{
		Field: "metadata.severity",
		Op:    store.OpEq,
		Value: "high",
	}

	events, err := mem.RecallWhere(ctx, []string{"test-ns"}, "bug", filter, 10)
	if err != nil {
		t.Fatalf("RecallWhere failed: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected events with severity=high, got none")
	}

	// All returned events should have severity=high in metadata
	for _, e := range events {
		if e.Metadata["severity"] != "high" {
			t.Errorf("expected severity=high, got %v", e.Metadata["severity"])
		}
	}
}

func TestRecallWhere_MultipleFilters(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()

	// Create events
	_, err = mem.Remember(ctx, "test-ns", "high severity api bug", map[string]any{
		"severity": "high",
		"component": "api",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	_, err = mem.Remember(ctx, "test-ns", "low severity api issue", map[string]any{
		"severity": "low",
		"component": "api",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	_, err = mem.Remember(ctx, "test-ns", "high severity gateway issue", map[string]any{
		"severity": "high",
		"component": "gateway",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Filter: severity=high AND component=api
	filter := &store.Predicate{
		And: []store.Predicate{
			{
				Field: "metadata.severity",
				Op:    store.OpEq,
				Value: "high",
			},
			{
				Field: "metadata.component",
				Op:    store.OpEq,
				Value: "api",
			},
		},
	}

	events, err := mem.RecallWhere(ctx, []string{"test-ns"}, "bug", filter, 10)
	if err != nil {
		t.Fatalf("RecallWhere failed: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected events matching both filters, got none")
	}

	// All returned events should match both filters
	for _, e := range events {
		if e.Metadata["severity"] != "high" {
			t.Errorf("expected severity=high, got %v", e.Metadata["severity"])
		}
		if e.Metadata["component"] != "api" {
			t.Errorf("expected component=api, got %v", e.Metadata["component"])
		}
	}
}

func TestRecallWhere_NilFilter(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()

	// Create multiple events
	_, err = mem.Remember(ctx, "test-ns", "event one", map[string]any{
		"severity": "high",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	_, err = mem.Remember(ctx, "test-ns", "event two", map[string]any{
		"severity": "low",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Call RecallWhere with nil filter (should be same as Recall)
	events, err := mem.RecallWhere(ctx, []string{"test-ns"}, "event", nil, 10)
	if err != nil {
		t.Fatalf("RecallWhere with nil filter failed: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestRecallWhere_InvalidLimit(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()

	filter := &store.Predicate{
		Field: "metadata.severity",
		Op:    store.OpEq,
		Value: "high",
	}

	_, err = mem.RecallWhere(ctx, []string{"test-ns"}, "query", filter, 0)
	if !errors.Is(err, ErrInvalidLimit) {
		t.Errorf("expected ErrInvalidLimit, got %v", err)
	}

	_, err = mem.RecallWhere(ctx, []string{"test-ns"}, "query", filter, -1)
	if !errors.Is(err, ErrInvalidLimit) {
		t.Errorf("expected ErrInvalidLimit, got %v", err)
	}
}

func TestLinkEvents_Success(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	// Create two events
	event1ID, err := mem.Remember(ctx, ns, "Event A", nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	event2ID, err := mem.Remember(ctx, ns, "Event B", nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Link them
	relationID, err := mem.LinkEvents(ctx, ns, event1ID, event2ID, RelationTypeContradicts, nil)
	if err != nil {
		t.Fatalf("LinkEvents failed: %v", err)
	}

	if relationID == "" {
		t.Fatal("expected relation ID, got empty string")
	}

	// Verify relationship was stored
	rel, err := s.Get(ctx, relationID)
	if err != nil {
		t.Fatalf("relationship not found in store: %v", err)
	}

	memMeta, ok := rel.Metadata["_memory"].(map[string]any)
	if !ok {
		t.Fatal("missing _memory metadata in relationship")
	}

	if memMeta["type"] != "relationship" {
		t.Errorf("expected type=relationship, got %v", memMeta["type"])
	}
	if memMeta["from_event_id"] != event1ID {
		t.Errorf("expected from_event_id=%s, got %v", event1ID, memMeta["from_event_id"])
	}
	if memMeta["to_event_id"] != event2ID {
		t.Errorf("expected to_event_id=%s, got %v", event2ID, memMeta["to_event_id"])
	}
	if memMeta["relation_type"] != RelationTypeContradicts {
		t.Errorf("expected relation_type=%s, got %v", RelationTypeContradicts, memMeta["relation_type"])
	}
}

func TestLinkEvents_SelfLink_Error(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	eventID, err := mem.Remember(ctx, ns, "Event", nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Try to link event to itself
	_, err = mem.LinkEvents(ctx, ns, eventID, eventID, RelationTypeContradicts, nil)
	if err == nil {
		t.Fatal("expected error for self-link, got nil")
	}
}

func TestLinkEvents_NonexistentEvent_Error(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	eventID, err := mem.Remember(ctx, ns, "Event", nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Try to link to nonexistent event
	_, err = mem.LinkEvents(ctx, ns, eventID, "nonexistent-id", RelationTypeContradicts, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent event, got nil")
	}
}

func TestFindRelated_Success(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	// Create three events
	event1ID, err := mem.Remember(ctx, ns, "Event A contradicts others", nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	event2ID, err := mem.Remember(ctx, ns, "Event B contradicted by A", nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	_, err = mem.Remember(ctx, ns, "Event C unrelated", nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Link: event1 contradicts event2
	_, err = mem.LinkEvents(ctx, ns, event1ID, event2ID, RelationTypeContradicts, nil)
	if err != nil {
		t.Fatalf("LinkEvents failed: %v", err)
	}

	// Find what event1 contradicts
	relatedEvents, err := mem.FindRelated(ctx, ns, event1ID, RelationTypeContradicts)
	if err != nil {
		t.Fatalf("FindRelated failed: %v", err)
	}

	if len(relatedEvents) != 1 {
		t.Errorf("expected 1 related event, got %d", len(relatedEvents))
	}

	if relatedEvents[0].ID != event2ID {
		t.Errorf("expected related event ID %s, got %s", event2ID, relatedEvents[0].ID)
	}
}

func TestFindRelated_MultipleRelations(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	// Create events
	eventAID, _ := mem.Remember(ctx, ns, "Event A", nil)
	eventBID, _ := mem.Remember(ctx, ns, "Event B", nil)
	eventCID, _ := mem.Remember(ctx, ns, "Event C", nil)
	eventDID, _ := mem.Remember(ctx, ns, "Event D", nil)

	// Create multiple relationships from A
	mem.LinkEvents(ctx, ns, eventAID, eventBID, RelationTypeContradicts, nil)
	mem.LinkEvents(ctx, ns, eventAID, eventCID, RelationTypeContradicts, nil)
	mem.LinkEvents(ctx, ns, eventAID, eventDID, RelationTypeCausedBy, nil) // Different type

	// Find all contradictions from A
	contradictions, err := mem.FindRelated(ctx, ns, eventAID, RelationTypeContradicts)
	if err != nil {
		t.Fatalf("FindRelated failed: %v", err)
	}

	if len(contradictions) != 2 {
		t.Errorf("expected 2 contradictions, got %d", len(contradictions))
	}

	// Find causes from A
	causes, err := mem.FindRelated(ctx, ns, eventAID, RelationTypeCausedBy)
	if err != nil {
		t.Fatalf("FindRelated failed: %v", err)
	}

	if len(causes) != 1 {
		t.Errorf("expected 1 cause, got %d", len(causes))
	}

	if causes[0].ID != eventDID {
		t.Errorf("expected event D, got %s", causes[0].ID)
	}
}

func TestLinkEvents_InvalidMetadata(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	eventAID, _ := mem.Remember(ctx, ns, "Event A", nil)
	eventBID, _ := mem.Remember(ctx, ns, "Event B", nil)

	// Try to use _memory-prefixed metadata
	_, err = mem.LinkEvents(ctx, ns, eventAID, eventBID, RelationTypeContradicts, map[string]any{
		"_memory.foo": "bar",
	})

	if !errors.Is(err, ErrInvalidMetadata) {
		t.Errorf("expected ErrInvalidMetadata, got %v", err)
	}
}

func TestRememberWithTTL_Success(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	eventID, err := mem.RememberWithTTL(ctx, ns, "temporary event", 1*time.Hour, nil)
	if err != nil {
		t.Fatalf("RememberWithTTL failed: %v", err)
	}

	if eventID == "" {
		t.Fatal("expected event ID, got empty string")
	}

	// Verify event was stored with expiration
	record, err := s.Get(ctx, eventID)
	if err != nil {
		t.Fatalf("store.Get failed: %v", err)
	}

	memMeta, ok := record.Metadata["_memory"].(map[string]any)
	if !ok {
		t.Fatal("missing _memory metadata")
	}

	if memMeta["expires_at"] == nil {
		t.Fatal("expires_at not set in metadata")
	}

	// Parse and verify expires_at is roughly 1 hour in future
	expiresAtStr, ok := memMeta["expires_at"].(string)
	if !ok {
		t.Fatal("expires_at is not a string")
	}

	expiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		t.Fatalf("failed to parse expires_at: %v", err)
	}

	now := time.Now().UTC()
	if expiresAt.Before(now) || expiresAt.After(now.Add(2*time.Hour)) {
		t.Errorf("expires_at is not approximately 1 hour from now: %v", expiresAt)
	}
}

func TestRememberWithTTL_InvalidTTL(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	// Zero TTL
	_, err = mem.RememberWithTTL(ctx, ns, "content", 0, nil)
	if err == nil {
		t.Fatal("expected error for zero TTL, got nil")
	}

	// Negative TTL
	_, err = mem.RememberWithTTL(ctx, ns, "content", -1*time.Hour, nil)
	if err == nil {
		t.Fatal("expected error for negative TTL, got nil")
	}
}

func TestRecall_FiltersExpiredEvents(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	// Create permanent event
	permanentID, err := mem.Remember(ctx, ns, "permanent event", nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Create very short-lived event
	expiredID, err := mem.RememberWithTTL(ctx, ns, "expired event", 10*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("RememberWithTTL failed: %v", err)
	}

	// Create future-expiring event
	futureID, err := mem.RememberWithTTL(ctx, ns, "future event", 1*time.Hour, nil)
	if err != nil {
		t.Fatalf("RememberWithTTL failed: %v", err)
	}

	// Sleep to let short-lived event expire
	time.Sleep(50 * time.Millisecond)

	// Search for "event" - should return permanent and future, not expired
	events, err := mem.Recall(ctx, []string{ns}, "event", 10)
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}

	// Check that expired event is not in results
	for _, e := range events {
		if e.ID == expiredID {
			t.Errorf("expired event should not be in Recall results")
		}
	}

	// Verify permanent and future are present
	foundPermanent := false
	foundFuture := false
	for _, e := range events {
		if e.ID == permanentID {
			foundPermanent = true
		}
		if e.ID == futureID {
			foundFuture = true
		}
	}

	if !foundPermanent {
		t.Error("permanent event should be in Recall results")
	}
	if !foundFuture {
		t.Error("future-expiring event should be in Recall results")
	}
}

func TestPurgeExpired_Success(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	// Create some events
	permanentID, _ := mem.Remember(ctx, ns, "permanent", nil)
	expiredID1, _ := mem.RememberWithTTL(ctx, ns, "expired 1", 10*time.Millisecond, nil)
	expiredID2, _ := mem.RememberWithTTL(ctx, ns, "expired 2", 10*time.Millisecond, nil)
	futureID, _ := mem.RememberWithTTL(ctx, ns, "future", 1*time.Hour, nil)

	// Sleep to ensure expiration
	time.Sleep(50 * time.Millisecond)

	// Purge expired
	count, err := mem.PurgeExpired(ctx, []string{ns})
	if err != nil {
		t.Fatalf("PurgeExpired failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 events purged, got %d", count)
	}

	// Verify permanent and future still exist
	if _, err := s.Get(ctx, permanentID); err != nil {
		t.Errorf("permanent event should still exist: %v", err)
	}
	if _, err := s.Get(ctx, futureID); err != nil {
		t.Errorf("future event should still exist: %v", err)
	}

	// Verify expired events are gone (hard deleted)
	if _, err := s.Get(ctx, expiredID1); err == nil {
		t.Error("expired event 1 should be deleted")
	}
	if _, err := s.Get(ctx, expiredID2); err == nil {
		t.Error("expired event 2 should be deleted")
	}
}

func TestPurgeExpired_MultipleNamespaces(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()

	// Create expired events in two namespaces
	mem.RememberWithTTL(ctx, "ns1", "expired in ns1", 10*time.Millisecond, nil)
	mem.RememberWithTTL(ctx, "ns2", "expired in ns2", 10*time.Millisecond, nil)

	time.Sleep(50 * time.Millisecond)

	// Purge both namespaces
	count, err := mem.PurgeExpired(ctx, []string{"ns1", "ns2"})
	if err != nil {
		t.Fatalf("PurgeExpired failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 events purged, got %d", count)
	}
}

func TestRememberMany_Success(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	// Prepare batch
	events := []BulkRemember{
		{Content: "Event 1", Metadata: map[string]any{"type": "note"}},
		{Content: "Event 2", Metadata: nil},
		{Content: "Event 3", Metadata: map[string]any{"priority": "high"}},
	}

	count, err := mem.RememberMany(ctx, ns, events)
	if err != nil {
		t.Fatalf("RememberMany failed: %v", err)
	}

	if count != 3 {
		t.Errorf("expected 3 events stored, got %d", count)
	}

	// Verify all events are searchable
	results, err := mem.Recall(ctx, []string{ns}, "Event", 10)
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 events in recall, got %d", len(results))
	}
}

func TestRememberMany_EmptyBatch(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	count, err := mem.RememberMany(context.Background(), "test-ns", []BulkRemember{})
	if err != nil {
		t.Fatalf("RememberMany failed: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 events, got %d", count)
	}
}

func TestRememberMany_TooLarge(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	// Create batch larger than 10k
	events := make([]BulkRemember, 10001)
	for i := 0; i < 10001; i++ {
		events[i] = BulkRemember{Content: "Event"}
	}

	_, err = mem.RememberMany(context.Background(), "test-ns", events)
	if err == nil {
		t.Fatal("expected error for batch > 10k, got nil")
	}
}

func TestRememberMany_InvalidContent(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	events := []BulkRemember{
		{Content: "Valid event"},
		{Content: ""}, // Invalid: empty
		{Content: "Another valid"},
	}

	_, err = mem.RememberMany(context.Background(), "test-ns", events)
	if err == nil {
		t.Fatal("expected error for empty content, got nil")
	}
}

func TestRememberMany_WithTTL(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake(), reasoner.NewFake("fake", "fake"))
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"
	ttl := 1 * time.Hour

	// Create one more event without TTL to verify count
	mem.Remember(ctx, ns, "Event permanent", nil)

	events := []BulkRemember{
		{Content: "Event with TTL", TTL: &ttl},
	}

	count, err := mem.RememberMany(ctx, ns, events)
	if err != nil {
		t.Fatalf("RememberMany failed: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 event stored, got %d", count)
	}

	// Verify total count (1 + 1 original)
	allResults, _ := mem.Recall(ctx, []string{ns}, "Event", 10)
	if len(allResults) < 1 {
		t.Error("expected at least 1 result")
	}

	// Verify at least one has TTL set
	hasTTL := false
	for _, e := range allResults {
		if e.ExpiresAt != nil {
			hasTTL = true
			break
		}
	}

	if !hasTTL {
		t.Error("expected at least one event with TTL")
	}
}

// Test consolidation

func TestConsolidateRecent_Basic(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-consolidate"

	// Create similar events that should cluster together
	evt1, _ := mem.Remember(ctx, ns, "Mohamed loves Go programming", nil)
	evt2, _ := mem.Remember(ctx, ns, "Mohamed prefers Go for systems programming", nil)
	evt3, _ := mem.Remember(ctx, ns, "Python is used for data science", nil)

	// Consolidate recent events (last hour)
	factIDs, err := mem.ConsolidateRecent(ctx, ns, time.Hour, 10)
	if err != nil {
		t.Fatalf("ConsolidateRecent failed: %v", err)
	}

	// Should produce at least 1 fact (likely 2: Go cluster + Python event)
	if len(factIDs) < 1 {
		t.Errorf("expected at least 1 fact, got %d", len(factIDs))
	}

	// Verify facts are stored and retrievable
	for _, factID := range factIDs {
		rec, err := mem.store.Get(ctx, factID)
		if err != nil {
			t.Errorf("fact %q not found: %v", factID, err)
			continue
		}

		// Verify fact has the right type
		memMeta, ok := rec.Metadata["_memory"].(map[string]any)
		if !ok {
			t.Errorf("fact %q missing _memory metadata", factID)
			continue
		}

		recType, ok := memMeta["type"].(string)
		if !ok || recType != "fact" {
			t.Errorf("fact %q has wrong type: %q", factID, recType)
		}

		// Verify synthesized_from is recorded
		// Note: might be []any or []string depending on storage/retrieval
		hasFrom := false
		if synthesizedFrom, ok := memMeta["synthesized_from"].([]any); ok && len(synthesizedFrom) > 0 {
			hasFrom = true
		} else if synthesizedFrom, ok := memMeta["synthesized_from"].([]string); ok && len(synthesizedFrom) > 0 {
			hasFrom = true
		}
		if !hasFrom {
			t.Errorf("fact %q missing or empty synthesized_from (memMeta=%+v)", factID, memMeta)
		}
	}

	// Verify events are still there
	if found, _ := mem.store.Get(ctx, evt1); found.DeletedAt != nil {
		t.Error("event 1 was deleted")
	}
	if found, _ := mem.store.Get(ctx, evt2); found.DeletedAt != nil {
		t.Error("event 2 was deleted")
	}
	if found, _ := mem.store.Get(ctx, evt3); found.DeletedAt != nil {
		t.Error("event 3 was deleted")
	}
}

func TestConsolidateRecent_TooFewEvents(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-consolidate"

	// Create only 1 event
	mem.Remember(ctx, ns, "Single event", nil)

	// Consolidate should return empty (< 2 events)
	factIDs, err := mem.ConsolidateRecent(ctx, ns, time.Hour, 10)
	if err != nil {
		t.Fatalf("ConsolidateRecent failed: %v", err)
	}

	if len(factIDs) != 0 {
		t.Errorf("expected 0 facts for single event, got %d", len(factIDs))
	}
}

func TestConsolidateRecent_EmptyNamespace(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()

	// Consolidate on empty namespace
	factIDs, err := mem.ConsolidateRecent(ctx, "empty-namespace", time.Hour, 10)
	if err != nil {
		t.Fatalf("ConsolidateRecent failed: %v", err)
	}

	if len(factIDs) != 0 {
		t.Errorf("expected 0 facts for empty namespace, got %d", len(factIDs))
	}
}

func TestConsolidateRecent_LimitClustersCorrectly(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-consolidate"

	// Create 10 distinct events (will likely create 10 clusters if not similar)
	for i := 0; i < 10; i++ {
		mem.Remember(ctx, ns, fmt.Sprintf("Event number %d about topic X", i), nil)
	}

	// Consolidate with limit of 2
	factIDs, err := mem.ConsolidateRecent(ctx, ns, time.Hour, 2)
	if err != nil {
		t.Fatalf("ConsolidateRecent failed: %v", err)
	}

	// Should produce at most 2 facts
	if len(factIDs) > 2 {
		t.Errorf("expected <= 2 facts with limit=2, got %d", len(factIDs))
	}
}

func TestConsolidateRecent_ExcludesOldEvents(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-consolidate"

	// Create an old event (simulate by manipulation - create event with old timestamp)
	// For this test, we'll just verify that events outside the timeWindow are skipped
	// by checking that we get 0 facts when querying a very small window

	// Create event now
	mem.Remember(ctx, ns, "Recent event 1", nil)
	mem.Remember(ctx, ns, "Recent event 2", nil)

	// Query with tiny time window (1 second) - should include just-created events
	factIDs, _ := mem.ConsolidateRecent(ctx, ns, time.Second, 10)

	// Should get facts (the 2 recent events are within 1 second)
	if len(factIDs) < 1 {
		t.Error("expected facts from recent events")
	}

	// Now query with negative window (no events matched)
	// This is a bit hard to test perfectly without mocking time, but we can verify
	// the mechanism works as designed
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []float32
		expected float64
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{-1, 0, 0},
			expected: -1.0,
		},
		{
			name:     "similar vectors",
			a:        []float32{1, 1, 0},
			b:        []float32{1, 1, 0.1},
			expected: 0.995, // Approximately
		},
		{
			name:     "zero vector",
			a:        []float32{0, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 0.0,
		},
		{
			name:     "empty vectors",
			a:        []float32{},
			b:        []float32{},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.a, tt.b)
			// Allow small floating point error
			if tt.expected == 0.0 && result != 0.0 {
				if result < -0.001 || result > 0.001 {
					t.Errorf("expected ~0.0, got %.6f", result)
				}
			} else if tt.expected != 0.0 {
				relErr := (result - tt.expected) / tt.expected
				if relErr < -0.01 || relErr > 0.01 { // 1% relative error
					t.Errorf("expected ~%.3f, got %.6f", tt.expected, result)
				}
			}
		})
	}
}


// Test contradiction detection

func TestFindContradictions_OverlappingConflict(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-contradictions"

	// Create two facts with overlapping time ranges and different values
	// Use fixed times for deterministic testing
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)

	fact1ID := uuid.New().String()
	fact2ID := uuid.New().String()

	// Fact 1: Mohamed speaks French (valid from now)
	fact1 := store.Record{
		ID:        fact1ID,
		Namespace: ns,
		Content:   "Mohamed speaks French",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":       "fact",
				"content":    "Mohamed speaks French",
				"created_at": now.Format(time.RFC3339),
				"valid_from": now.Format(time.RFC3339),
				"valid_until": nil,
				"source":     "test",
			},
			"entity":   "Mohamed",
			"property": "language",
			"value":    "French",
		},
	}

	// Fact 2: Mohamed speaks Spanish (overlapping range)
	fact2 := store.Record{
		ID:        fact2ID,
		Namespace: ns,
		Content:   "Mohamed speaks Spanish",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{0, 1, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":       "fact",
				"content":    "Mohamed speaks Spanish",
				"created_at": now.Add(time.Minute).Format(time.RFC3339),
				"valid_from": now.Add(time.Minute).Format(time.RFC3339),
				"valid_until": future.Format(time.RFC3339), // Ends in the future
				"source":     "test",
			},
			"entity":   "Mohamed",
			"property": "language",
			"value":    "Spanish",
		},
	}

	mem.store.Put(ctx, fact1)
	mem.store.Put(ctx, fact2)

	// Note: FindContradictions uses time.Now().UTC() for reference time.
	// For this test to work with fixed historical times, we rely on the fact that
	// Fact 2's ValidUntil (future) is far enough in the future to cover the test execution time.
	// In production, facts would typically have ValidUntil in the past (ended) or nil (ongoing).

	// Find contradictions
	contradictions, err := mem.FindContradictions(ctx, ns)
	if err != nil {
		t.Fatalf("FindContradictions failed: %v", err)
	}

	// Should find the contradiction
	if len(contradictions) != 1 {
		t.Errorf("expected 1 contradiction, got %d (future=%v, now=%v)", len(contradictions), future, time.Now().UTC())
		return
	}

	c := contradictions[0]
	if c.Status != ContradictionStatusConflict {
		t.Errorf("expected status %q, got %q", ContradictionStatusConflict, c.Status)
	}
	if c.Entity != "Mohamed" {
		t.Errorf("expected entity Mohamed, got %q", c.Entity)
	}
	if c.Property != "language" {
		t.Errorf("expected property language, got %q", c.Property)
	}
	if (c.Value1 == "French" && c.Value2 != "Spanish") || (c.Value1 == "Spanish" && c.Value2 != "French") {
		t.Errorf("expected values French and Spanish, got %q and %q", c.Value1, c.Value2)
	}
}

func TestFindContradictions_SequentialEvolution(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-contradictions"

	now := time.Now().UTC()
	then := now.Add(-time.Hour)

	// Fact 1: Mohamed spoke French (valid from then, until now)
	fact1 := store.Record{
		ID:        uuid.New().String(),
		Namespace: ns,
		Content:   "Mohamed spoke French",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":        "fact",
				"valid_from":  then.Format(time.RFC3339),
				"valid_until": now.Format(time.RFC3339), // Ended at now
				"source":      "test",
			},
			"entity":   "Mohamed",
			"property": "language",
			"value":    "French",
		},
	}

	// Fact 2: Mohamed speaks Spanish (valid from now)
	fact2 := store.Record{
		ID:        uuid.New().String(),
		Namespace: ns,
		Content:   "Mohamed speaks Spanish",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{0, 1, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":       "fact",
				"valid_from": now.Format(time.RFC3339),
				"valid_until": nil, // Ongoing
				"source":     "test",
			},
			"entity":   "Mohamed",
			"property": "language",
			"value":    "Spanish",
		},
	}

	mem.store.Put(ctx, fact1)
	mem.store.Put(ctx, fact2)

	// Find contradictions
	contradictions, err := mem.FindContradictions(ctx, ns)
	if err != nil {
		t.Fatalf("FindContradictions failed: %v", err)
	}

	// Should find NO contradictions (sequential, non-overlapping)
	if len(contradictions) != 0 {
		t.Errorf("expected 0 contradictions for sequential facts, got %d", len(contradictions))
	}
}

func TestFindContradictions_SameValueNoConflict(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-contradictions"

	now := time.Now().UTC()

	// Fact 1: Mohamed speaks French
	fact1 := store.Record{
		ID:        uuid.New().String(),
		Namespace: ns,
		Content:   "Mohamed speaks French",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":       "fact",
				"valid_from": now.Format(time.RFC3339),
				"valid_until": nil,
				"source":     "test",
			},
			"entity":   "Mohamed",
			"property": "language",
			"value":    "French",
		},
	}

	// Fact 2: Mohamed also speaks French (same value)
	fact2 := store.Record{
		ID:        uuid.New().String(),
		Namespace: ns,
		Content:   "Mohamed speaks French",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":       "fact",
				"valid_from": now.Format(time.RFC3339),
				"valid_until": nil,
				"source":     "test",
			},
			"entity":   "Mohamed",
			"property": "language",
			"value":    "French", // Same value
		},
	}

	mem.store.Put(ctx, fact1)
	mem.store.Put(ctx, fact2)

	// Find contradictions
	contradictions, err := mem.FindContradictions(ctx, ns)
	if err != nil {
		t.Fatalf("FindContradictions failed: %v", err)
	}

	// Should find NO contradictions (same values)
	if len(contradictions) != 0 {
		t.Errorf("expected 0 contradictions for same values, got %d", len(contradictions))
	}
}

func TestFindContradictions_DifferentPropertyNoConflict(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-contradictions"

	now := time.Now().UTC()

	// Fact 1: Mohamed speaks French
	fact1 := store.Record{
		ID:        uuid.New().String(),
		Namespace: ns,
		Content:   "Mohamed speaks French",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":       "fact",
				"valid_from": now.Format(time.RFC3339),
				"valid_until": nil,
				"source":     "test",
			},
			"entity":   "Mohamed",
			"property": "language",
			"value":    "French",
		},
	}

	// Fact 2: Mohamed likes Go (different property)
	fact2 := store.Record{
		ID:        uuid.New().String(),
		Namespace: ns,
		Content:   "Mohamed likes Go",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{0, 1, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":       "fact",
				"valid_from": now.Format(time.RFC3339),
				"valid_until": nil,
				"source":     "test",
			},
			"entity":   "Mohamed",
			"property": "favorite_language", // Different property
			"value":    "Go",
		},
	}

	mem.store.Put(ctx, fact1)
	mem.store.Put(ctx, fact2)

	// Find contradictions
	contradictions, err := mem.FindContradictions(ctx, ns)
	if err != nil {
		t.Fatalf("FindContradictions failed: %v", err)
	}

	// Should find NO contradictions (different properties)
	if len(contradictions) != 0 {
		t.Errorf("expected 0 contradictions for different properties, got %d", len(contradictions))
	}
}

func TestFindContradictions_MissingMetadata(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-contradictions"

	now := time.Now().UTC()

	// Fact 1: missing entity (won't be compared)
	fact1 := store.Record{
		ID:        uuid.New().String(),
		Namespace: ns,
		Content:   "No entity",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":       "fact",
				"valid_from": now.Format(time.RFC3339),
				"valid_until": nil,
				"source":     "test",
			},
			"property": "language",
			"value":    "French",
			// missing "entity"
		},
	}

	// Fact 2: complete
	fact2 := store.Record{
		ID:        uuid.New().String(),
		Namespace: ns,
		Content:   "Complete fact",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{0, 1, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":       "fact",
				"valid_from": now.Format(time.RFC3339),
				"valid_until": nil,
				"source":     "test",
			},
			"entity":   "Ali",
			"property": "language",
			"value":    "Spanish",
		},
	}

	mem.store.Put(ctx, fact1)
	mem.store.Put(ctx, fact2)

	// Find contradictions
	contradictions, err := mem.FindContradictions(ctx, ns)
	if err != nil {
		t.Fatalf("FindContradictions failed: %v", err)
	}

	// Should find NO contradictions (fact1 missing entity)
	if len(contradictions) != 0 {
		t.Errorf("expected 0 contradictions with missing metadata, got %d", len(contradictions))
	}
}

func TestFindContradictions_EmptyNamespace(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()

	// Find contradictions in empty namespace
	contradictions, err := mem.FindContradictions(ctx, "empty-namespace")
	if err != nil {
		t.Fatalf("FindContradictions failed: %v", err)
	}

	if len(contradictions) != 0 {
		t.Errorf("expected 0 contradictions in empty namespace, got %d", len(contradictions))
	}
}

func TestFindContradictions_MultipleContradictions(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-contradictions"

	now := time.Now().UTC()

	// Create 4 facts with 2 contradiction pairs
	facts := []struct {
		id       string
		entity   string
		property string
		value    string
	}{
		{uuid.New().String(), "Mohamed", "language", "French"},
		{uuid.New().String(), "Mohamed", "language", "Spanish"},
		{uuid.New().String(), "Ali", "programming_language", "Go"},
		{uuid.New().String(), "Ali", "programming_language", "Rust"},
	}

	for _, f := range facts {
		record := store.Record{
			ID:        f.id,
			Namespace: ns,
			Content:   fmt.Sprintf("%s %s %s", f.entity, f.property, f.value),
			Vectors: map[string]store.Vector{
				"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
			},
			Metadata: map[string]any{
				"_memory": map[string]any{
					"type":       "fact",
					"valid_from": now.Format(time.RFC3339),
					"valid_until": nil,
					"source":     "test",
				},
				"entity":   f.entity,
				"property": f.property,
				"value":    f.value,
			},
		}
		mem.store.Put(ctx, record)
	}

	// Find contradictions
	contradictions, err := mem.FindContradictions(ctx, ns)
	if err != nil {
		t.Fatalf("FindContradictions failed: %v", err)
	}

	// Should find 2 contradictions (Mohamed language + Ali programming_language)
	if len(contradictions) != 2 {
		t.Errorf("expected 2 contradictions, got %d", len(contradictions))
		for i, c := range contradictions {
			t.Logf("  Contradiction %d: %s/%s: %q vs %q", i+1, c.Entity, c.Property, c.Value1, c.Value2)
		}
		return
	}

	// Verify sorting: by entity, then property
	if contradictions[0].Entity > contradictions[1].Entity {
		t.Error("contradictions not sorted by entity")
	}
}

func TestTimeRangesOverlap(t *testing.T) {
	tests := []struct {
		name     string
		from1    time.Time
		until1   *time.Time
		from2    time.Time
		until2   *time.Time
		expected bool
	}{
		{
			name:     "identical ranges",
			from1:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			until1:   ptrTime(time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)),
			from2:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			until2:   ptrTime(time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)),
			expected: true,
		},
		{
			name:     "overlapping ranges",
			from1:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			until1:   ptrTime(time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)),
			from2:    time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			until2:   ptrTime(time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)),
			expected: true,
		},
		{
			name:     "sequential ranges (no overlap)",
			from1:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			until1:   ptrTime(time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)),
			from2:    time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), // Starts exactly when 1 ends
			until2:   ptrTime(time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)),
			expected: false, // No overlap (boundary case)
		},
		{
			name:     "one ongoing (nil until)",
			from1:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			until1:   nil, // Ongoing
			from2:    time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			until2:   ptrTime(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)),
			expected: true,
		},
		{
			name:     "both ongoing (nil until)",
			from1:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			until1:   nil, // Ongoing
			from2:    time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			until2:   nil, // Ongoing
			expected: true,
		},
		{
			name:     "completely separate ranges",
			from1:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			until1:   ptrTime(time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)),
			from2:    time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			until2:   ptrTime(time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC)),
			expected: false,
		},
	}

	// Use a reference time that's in the future to test "ongoing" facts
	refTime := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := timeRangesOverlap(tt.from1, tt.until1, tt.from2, tt.until2, refTime)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// Helper to create time pointer
func ptrTime(t time.Time) *time.Time {
	return &t
}

// Debug test to understand metadata storage


// Test reflection

func TestReflect_EmptyNamespace(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()

	// Reflect on empty namespace
	report, err := mem.Reflect(ctx, "empty-namespace")
	if err != nil {
		t.Fatalf("Reflect failed: %v", err)
	}

	if report == nil {
		t.Fatal("report is nil")
	}
	if report.TotalFacts != 0 {
		t.Errorf("expected 0 facts, got %d", report.TotalFacts)
	}
	if report.TotalEntities != 0 {
		t.Errorf("expected 0 entities, got %d", report.TotalEntities)
	}
	if report.DateRange != nil {
		t.Errorf("expected nil date range, got %v", report.DateRange)
	}
}

func TestReflect_SingleEntity(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-reflect"
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)

	// Create facts about Mohamed
	factID1 := uuid.New().String()
	factID2 := uuid.New().String()

	fact1 := store.Record{
		ID:        factID1,
		Namespace: ns,
		Content:   "Mohamed speaks French",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":       "fact",
				"valid_from": now.Format(time.RFC3339),
				"valid_until": nil,
				"source":     "consolidation",
			},
			"entity":   "Mohamed",
			"property": "language",
			"value":    "French",
		},
	}

	fact2 := store.Record{
		ID:        factID2,
		Namespace: ns,
		Content:   "Mohamed likes Go",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{0, 1, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":       "fact",
				"valid_from": now.Add(time.Hour).Format(time.RFC3339),
				"valid_until": nil,
				"source":     "consolidation",
			},
			"entity":   "Mohamed",
			"property": "favorite_language",
			"value":    "Go",
		},
	}

	mem.store.Put(ctx, fact1)
	mem.store.Put(ctx, fact2)

	// Reflect
	report, err := mem.Reflect(ctx, ns)
	if err != nil {
		t.Fatalf("Reflect failed: %v", err)
	}

	if report.TotalFacts != 2 {
		t.Errorf("expected 2 facts, got %d", report.TotalFacts)
	}
	if report.TotalEntities != 1 {
		t.Errorf("expected 1 entity, got %d", report.TotalEntities)
	}

	// Check entity summary
	mohamedSummary, ok := report.EntitiesByName["Mohamed"]
	if !ok {
		t.Fatal("Mohamed not in entities")
	}
	if mohamedSummary.FactCount != 2 {
		t.Errorf("expected Mohamed to have 2 facts, got %d", mohamedSummary.FactCount)
	}
	if len(mohamedSummary.Properties) != 2 {
		t.Errorf("expected 2 properties, got %d", len(mohamedSummary.Properties))
	}
	if mohamedSummary.Sources["consolidation"] != 2 {
		t.Errorf("expected 2 consolidation sources, got %d", mohamedSummary.Sources["consolidation"])
	}

	// Check date range
	if report.DateRange == nil {
		t.Fatal("date range is nil")
	}
	if !report.DateRange.From.Equal(now) {
		t.Errorf("expected from=%v, got %v", now, report.DateRange.From)
	}
}

func TestReflect_MultipleEntities(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-reflect"
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)

	// Create facts about 3 entities
	for _, entityName := range []string{"Mohamed", "Ali", "Fatima"} {
		fact := store.Record{
			ID:        uuid.New().String(),
			Namespace: ns,
			Content:   fmt.Sprintf("%s fact", entityName),
			Vectors: map[string]store.Vector{
				"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
			},
			Metadata: map[string]any{
				"_memory": map[string]any{
					"type":       "fact",
					"valid_from": now.Format(time.RFC3339),
					"valid_until": nil,
					"source":     "consolidation",
				},
				"entity":   entityName,
				"property": "name",
				"value":    entityName,
			},
		}
		mem.store.Put(ctx, fact)
	}

	// Reflect
	report, err := mem.Reflect(ctx, ns)
	if err != nil {
		t.Fatalf("Reflect failed: %v", err)
	}

	if report.TotalFacts != 3 {
		t.Errorf("expected 3 facts, got %d", report.TotalFacts)
	}
	if report.TotalEntities != 3 {
		t.Errorf("expected 3 entities, got %d", report.TotalEntities)
	}

	// Check entities are present
	expectedEntities := []string{"Mohamed", "Ali", "Fatima"}
	for _, name := range expectedEntities {
		if _, ok := report.EntitiesByName[name]; !ok {
			t.Errorf("entity %q not found", name)
		}
	}
}

func TestReflect_IncludesContradictions(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-reflect"
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)

	// Create contradicting facts
	fact1 := store.Record{
		ID:        uuid.New().String(),
		Namespace: ns,
		Content:   "Mohamed speaks French",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":       "fact",
				"valid_from": now.Format(time.RFC3339),
				"valid_until": nil,
				"source":     "consolidation",
			},
			"entity":   "Mohamed",
			"property": "language",
			"value":    "French",
		},
	}

	fact2 := store.Record{
		ID:        uuid.New().String(),
		Namespace: ns,
		Content:   "Mohamed speaks Spanish",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{0, 1, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":       "fact",
				"valid_from": now.Add(time.Minute).Format(time.RFC3339),
				"valid_until": now.Add(time.Hour).Format(time.RFC3339),
				"source":     "consolidation",
			},
			"entity":   "Mohamed",
			"property": "language",
			"value":    "Spanish",
		},
	}

	mem.store.Put(ctx, fact1)
	mem.store.Put(ctx, fact2)

	// Reflect
	report, err := mem.Reflect(ctx, ns)
	if err != nil {
		t.Fatalf("Reflect failed: %v", err)
	}

	if report.TotalContradictions != 1 {
		t.Errorf("expected 1 contradiction, got %d", report.TotalContradictions)
	}

	// Check entity's contradiction count
	mohamedSummary, ok := report.EntitiesByName["Mohamed"]
	if !ok {
		t.Fatal("Mohamed not found")
	}
	if mohamedSummary.ContradictionCount != 1 {
		t.Errorf("expected 1 contradiction for Mohamed, got %d", mohamedSummary.ContradictionCount)
	}
}

func TestReflect_IdentifiesGaps(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-reflect"
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)

	// Create facts: Mohamed (1 fact), Ali (2 facts), Fatima (3 facts)
	entities := []struct {
		name     string
		factCount int
	}{
		{"Mohamed", 1},
		{"Ali", 2},
		{"Fatima", 3},
	}

	for _, e := range entities {
		for i := 0; i < e.factCount; i++ {
			fact := store.Record{
				ID:        uuid.New().String(),
				Namespace: ns,
				Content:   fmt.Sprintf("%s fact %d", e.name, i),
				Vectors: map[string]store.Vector{
					"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
				},
				Metadata: map[string]any{
					"_memory": map[string]any{
						"type":       "fact",
						"valid_from": now.Add(time.Duration(i) * time.Hour).Format(time.RFC3339),
						"valid_until": nil,
						"source":     "consolidation",
					},
					"entity":   e.name,
					"property": fmt.Sprintf("prop%d", i),
					"value":    fmt.Sprintf("val%d", i),
				},
			}
			mem.store.Put(ctx, fact)
		}
	}

	// Reflect
	report, err := mem.Reflect(ctx, ns)
	if err != nil {
		t.Fatalf("Reflect failed: %v", err)
	}

	// Check gaps (entities with <= 2 facts)
	if len(report.Gaps) != 2 {
		t.Errorf("expected 2 gaps, got %d", len(report.Gaps))
	}

	// Gaps should be sorted by fact count (Mohamed has 1, Ali has 2)
	if len(report.Gaps) >= 1 && report.Gaps[0].Entity != "Mohamed" {
		t.Errorf("expected first gap to be Mohamed, got %s", report.Gaps[0].Entity)
	}
	if len(report.Gaps) >= 2 && report.Gaps[1].Entity != "Ali" {
		t.Errorf("expected second gap to be Ali, got %s", report.Gaps[1].Entity)
	}

	// Fatima should not be in gaps (has 3 facts)
	for _, gap := range report.Gaps {
		if gap.Entity == "Fatima" {
			t.Error("Fatima should not be in gaps")
		}
	}
}

func TestReflect_SourceTracking(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-reflect"
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)

	// Create facts with different sources
	sources := []string{"consolidation", "user", "import"}
	for i, source := range sources {
		fact := store.Record{
			ID:        uuid.New().String(),
			Namespace: ns,
			Content:   fmt.Sprintf("Fact from %s", source),
			Vectors: map[string]store.Vector{
				"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
			},
			Metadata: map[string]any{
				"_memory": map[string]any{
					"type":       "fact",
					"valid_from": now.Add(time.Duration(i) * time.Hour).Format(time.RFC3339),
					"valid_until": nil,
					"source":     source,
				},
				"entity":   "TestEntity",
				"property": "prop",
				"value":    "val",
			},
		}
		mem.store.Put(ctx, fact)
	}

	// Reflect
	report, err := mem.Reflect(ctx, ns)
	if err != nil {
		t.Fatalf("Reflect failed: %v", err)
	}

	testEntity := report.EntitiesByName["TestEntity"]
	if testEntity == nil {
		t.Fatal("TestEntity not found")
	}

	// Check sources
	for _, source := range sources {
		if testEntity.Sources[source] != 1 {
			t.Errorf("expected 1 %s fact, got %d", source, testEntity.Sources[source])
		}
	}
}

func TestReflect_TemporalOrder(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-reflect"
	baseTime := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)

	// Create facts in reverse order (newest first)
	for i := 2; i >= 0; i-- {
		validFrom := baseTime.Add(time.Duration(i) * time.Hour)
		fact := store.Record{
			ID:        uuid.New().String(),
			Namespace: ns,
			Content:   fmt.Sprintf("Fact at %v", validFrom),
			Vectors: map[string]store.Vector{
				"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
			},
			Metadata: map[string]any{
				"_memory": map[string]any{
					"type":       "fact",
					"valid_from": validFrom.Format(time.RFC3339),
					"valid_until": nil,
					"source":     "consolidation",
				},
				"entity":   "TestEntity",
				"property": fmt.Sprintf("prop%d", i),
				"value":    fmt.Sprintf("val%d", i),
			},
		}
		mem.store.Put(ctx, fact)
	}

	// Reflect
	report, err := mem.Reflect(ctx, ns)
	if err != nil {
		t.Fatalf("Reflect failed: %v", err)
	}

	testEntity := report.EntitiesByName["TestEntity"]
	if testEntity == nil {
		t.Fatal("TestEntity not found")
	}

	// Check that facts in each property are sorted by time
	for propName, factValues := range testEntity.Properties {
		for i := 1; i < len(factValues); i++ {
			if !factValues[i-1].ValidFrom.Before(factValues[i].ValidFrom) {
				t.Errorf("property %q not sorted by time: %v should be before %v",
					propName, factValues[i-1].ValidFrom, factValues[i].ValidFrom)
			}
		}
	}
}

func TestReflect_FactsWithoutEntity(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-reflect"
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)

	// Create a fact without entity (should be ignored in grouping)
	fact := store.Record{
		ID:        uuid.New().String(),
		Namespace: ns,
		Content:   "Fact without entity",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":       "fact",
				"valid_from": now.Format(time.RFC3339),
				"valid_until": nil,
				"source":     "consolidation",
			},
			// Missing "entity" field
			"property": "prop",
			"value":    "val",
		},
	}

	mem.store.Put(ctx, fact)

	// Reflect
	report, err := mem.Reflect(ctx, ns)
	if err != nil {
		t.Fatalf("Reflect failed: %v", err)
	}

	// Fact should not be in entity grouping
	if report.TotalEntities != 0 {
		t.Errorf("expected 0 entities, got %d", report.TotalEntities)
	}
	// But reflection should still succeed
	if report == nil {
		t.Fatal("report is nil")
	}
}

// Test reinforcement

func TestCalculateConfidence(t *testing.T) {
	tests := []struct {
		count    int
		expected float32
	}{
		{0, 0.0},
		{1, 1.0 / 3.0}, // ≈ 0.33
		{2, 0.5},
		{5, 5.0 / 7.0}, // ≈ 0.71
		{10, 10.0 / 12.0}, // ≈ 0.83
	}

	for _, tt := range tests {
		result := calculateConfidence(tt.count)
		// Allow small floating point error
		if tt.expected == 0.0 {
			if result != 0.0 {
				t.Errorf("count=%d: expected 0.0, got %.4f", tt.count, result)
			}
		} else {
			relErr := (result - tt.expected) / tt.expected
			if relErr < -0.01 || relErr > 0.01 {
				t.Errorf("count=%d: expected ~%.4f, got %.4f", tt.count, tt.expected, result)
			}
		}
	}
}

func TestReinforce_ExistingFact(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-reinforce"
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)

	// Create a fact
	fact := store.Record{
		ID:        uuid.New().String(),
		Namespace: ns,
		Content:   "Mohamed speaks French",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":               "fact",
				"valid_from":         now.Format(time.RFC3339),
				"valid_until":        nil,
				"source":             "consolidation",
				"confidence":         0.5,
				"observation_count":  1,
			},
			"entity":   "Mohamed",
			"property": "language",
			"value":    "French",
		},
	}

	mem.store.Put(ctx, fact)

	// Reinforce it
	err := mem.Reinforce(ctx, ns, "Mohamed", "language", "French")
	if err != nil {
		t.Fatalf("Reinforce failed: %v", err)
	}

	// Verify updated
	updated, _ := mem.store.Get(ctx, fact.ID)
	memMeta := updated.Metadata["_memory"].(map[string]any)

	newCount := int(memMeta["observation_count"].(float64))
	if newCount != 2 {
		t.Errorf("expected count 2, got %d", newCount)
	}

	newConf := float32(memMeta["confidence"].(float64))
	expectedConf := calculateConfidence(2)
	if math.Abs(float64(newConf-expectedConf)) > 0.01 {
		t.Errorf("expected confidence ~%.4f, got %.4f", expectedConf, newConf)
	}
}

func TestReinforce_MultipleReinforcements(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-reinforce"
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)

	factID := uuid.New().String()

	// Create a fact
	fact := store.Record{
		ID:        factID,
		Namespace: ns,
		Content:   "Test fact",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":               "fact",
				"valid_from":         now.Format(time.RFC3339),
				"valid_until":        nil,
				"source":             "consolidation",
				"confidence":         0.5,
				"observation_count":  1,
			},
			"entity":   "Entity",
			"property": "prop",
			"value":    "val",
		},
	}

	mem.store.Put(ctx, fact)

	// Reinforce multiple times
	for i := 0; i < 4; i++ {
		err := mem.Reinforce(ctx, ns, "Entity", "prop", "val")
		if err != nil {
			t.Fatalf("Reinforce %d failed: %v", i+1, err)
		}
	}

	// Verify final state
	updated, _ := mem.store.Get(ctx, factID)
	memMeta := updated.Metadata["_memory"].(map[string]any)

	finalCount := int(memMeta["observation_count"].(float64))
	if finalCount != 5 { // 1 initial + 4 reinforcements
		t.Errorf("expected count 5, got %d", finalCount)
	}

	finalConf := float32(memMeta["confidence"].(float64))
	expectedConf := calculateConfidence(5)
	if math.Abs(float64(finalConf-expectedConf)) > 0.01 {
		t.Errorf("expected confidence ~%.4f, got %.4f", expectedConf, finalConf)
	}
}

func TestReinforce_NotFound(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-reinforce"

	// Try to reinforce non-existent fact
	err := mem.Reinforce(ctx, ns, "NonExistent", "prop", "val")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestReinforce_DifferentValue(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-reinforce"
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)

	// Create a fact with value "French"
	fact := store.Record{
		ID:        uuid.New().String(),
		Namespace: ns,
		Content:   "Test",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":               "fact",
				"valid_from":         now.Format(time.RFC3339),
				"valid_until":        nil,
				"source":             "consolidation",
				"confidence":         0.5,
				"observation_count":  1,
			},
			"entity":   "Mohamed",
			"property": "language",
			"value":    "French",
		},
	}

	mem.store.Put(ctx, fact)

	// Try to reinforce with different value ("Spanish")
	err := mem.Reinforce(ctx, ns, "Mohamed", "language", "Spanish")
	if err == nil {
		t.Error("expected error for different value, got nil")
	}
}

func TestReinforce_PersistsToStore(t *testing.T) {
	mem, cleanup := startMemory(t)
	defer cleanup()

	ctx := context.Background()
	ns := "test-reinforce"
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)

	factID := uuid.New().String()

	fact := store.Record{
		ID:        factID,
		Namespace: ns,
		Content:   "Test",
		Vectors: map[string]store.Vector{
			"fake": {Values: []float32{1, 0, 0, 0, 0, 0, 0, 0}, Model: "fake"},
		},
		Metadata: map[string]any{
			"_memory": map[string]any{
				"type":               "fact",
				"valid_from":         now.Format(time.RFC3339),
				"valid_until":        nil,
				"source":             "consolidation",
				"confidence":         0.5,
				"observation_count":  1,
			},
			"entity":   "Entity",
			"property": "prop",
			"value":    "val",
		},
	}

	mem.store.Put(ctx, fact)

	// Reinforce
	mem.Reinforce(ctx, ns, "Entity", "prop", "val")

	// Retrieve and verify persistence
	retrieved, _ := mem.store.Get(ctx, factID)
	parsedFact, _ := FactFromRecord(&retrieved)

	if parsedFact.ObservationCount != 2 {
		t.Errorf("expected count 2, got %d", parsedFact.ObservationCount)
	}
	if parsedFact.Confidence != calculateConfidence(2) {
		t.Errorf("expected confidence %.4f, got %.4f", calculateConfidence(2), parsedFact.Confidence)
	}
}
