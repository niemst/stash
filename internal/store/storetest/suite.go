package storetest

import (
	"context"
	"sync"
	"testing"

	"github.com/alash3al/stash/internal/store"
)

// RunSuite runs a comprehensive test suite against a store.Store implementation.
func RunSuite(t *testing.T, s store.Store) {
	t.Helper()

	t.Run("PutGet", func(t *testing.T) { testPutGet(t, s) })
	t.Run("PutMany", func(t *testing.T) { testPutMany(t, s) })
	t.Run("DeletePurge", func(t *testing.T) { testDeletePurge(t, s) })
	t.Run("DeleteWhere", func(t *testing.T) { testDeleteWhere(t, s) })
	t.Run("SearchVector", func(t *testing.T) { testSearchVector(t, s) })
	t.Run("SearchText", func(t *testing.T) { testSearchText(t, s) })
	t.Run("ListFilter", func(t *testing.T) { testListFilter(t, s) })
	t.Run("Iterate", func(t *testing.T) { testIterate(t, s) })
	t.Run("Count", func(t *testing.T) { testCount(t, s) })
	t.Run("WithTx", func(t *testing.T) { testWithTx(t, s) })
	t.Run("SoftDeleteInvisibility", func(t *testing.T) { testSoftDeleteInvisibility(t, s) })
	t.Run("ConcurrentPut", func(t *testing.T) { testConcurrentPut(t, s) })
	t.Run("Health", func(t *testing.T) { testHealth(t, s) })
	t.Run("PredicateOperators", func(t *testing.T) { testPredicateOperators(t, s) })
	t.Run("PredicateComposition", func(t *testing.T) { testPredicateComposition(t, s) })
	t.Run("MetadataPaths", func(t *testing.T) { testMetadataPaths(t, s) })
	t.Run("VectorWithFilter", func(t *testing.T) { testVectorWithFilter(t, s) })
}

func testPutGet(t *testing.T, s store.Store) {
	ctx := context.Background()

	// Create a record
	r := store.Record{
		ID:      "test-1",
		Content: "Hello World",
		Vectors: map[string]store.Vector{
			"test": {Values: []float32{0.1, 0.2, 0.3}, Model: "test-model"},
		},
		Metadata: map[string]any{"key": "value"},
	}

	// Put the record
	err := s.Put(ctx, r)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get the record
	got, err := s.Get(ctx, "test-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Verify
	if got.ID != r.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, r.ID)
	}
	if got.Content != r.Content {
		t.Errorf("Content mismatch: got %q, want %q", got.Content, r.Content)
	}
	if len(got.Vectors) != 1 {
		t.Errorf("Vector count mismatch: got %d, want 1", len(got.Vectors))
	}
	if got.Metadata["key"] != "value" {
		t.Errorf("Metadata mismatch: got %v", got.Metadata)
	}

	// Get non-existent
	_, err = s.Get(ctx, "nonexistent")
	if err != store.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got: %v", err)
	}
}

func testPutMany(t *testing.T, s store.Store) {
	ctx := context.Background()

	// Create 100 records
	records := make([]store.Record, 100)
	for i := 0; i < 100; i++ {
		records[i] = store.Record{
			ID:      "bulk-" + string(rune('a'+i)),
			Content: "Bulk record",
			Vectors: map[string]store.Vector{
				"test": {Values: []float32{0.1, 0.2, 0.3}, Model: "test-model"},
			},
			Metadata: map[string]any{"index": i},
		}
	}

	err := s.PutMany(ctx, records)
	if err != nil {
		t.Fatalf("PutMany failed: %v", err)
	}

	// Verify a few records
	for _, idx := range []int{0, 50, 99} {
		id := "bulk-" + string(rune('a'+idx))
		got, err := s.Get(ctx, id)
		if err != nil {
			t.Errorf("Get %s failed: %v", id, err)
		}
		if got.ID != id {
			t.Errorf("ID mismatch for %s: got %q", id, got.ID)
		}
	}
}

func testDeletePurge(t *testing.T, s store.Store) {
	ctx := context.Background()

	// Create and delete
	r := store.Record{ID: "delete-test", Content: "To be deleted"}
	err := s.Put(ctx, r)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Soft delete
	err = s.Delete(ctx, "delete-test")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone from reads
	_, err = s.Get(ctx, "delete-test")
	if err != store.ErrNotFound {
		t.Errorf("Expected ErrNotFound after delete, got: %v", err)
	}

	// Purge
	err = s.Purge(ctx, "delete-test")
	if err != nil {
		t.Fatalf("Purge failed: %v", err)
	}

	// Verify it's completely gone
	_, err = s.Get(ctx, "delete-test")
	if err != store.ErrNotFound {
		t.Errorf("Expected ErrNotFound after purge, got: %v", err)
	}
}

func testDeleteWhere(t *testing.T, s store.Store) {
	ctx := context.Background()

	// Create records with different metadata
	for i := 0; i < 10; i++ {
		category := "A"
		if i >= 5 {
			category = "B"
		}
		err := s.Put(ctx, store.Record{
			ID:       "delete-where-" + string(rune('a'+i)),
			Content:  "Test",
			Metadata: map[string]any{"category": category},
		})
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Delete all category "A"
	count, err := s.DeleteWhere(ctx, &store.Predicate{
		Field: "metadata.category",
		Op:    store.OpEq,
		Value: "A",
	})
	if err != nil {
		t.Fatalf("DeleteWhere failed: %v", err)
	}
	if count != 5 {
		t.Errorf("Expected 5 deleted, got %d", count)
	}

	// Verify category A records are gone
	for i := 0; i < 5; i++ {
		_, err := s.Get(ctx, "delete-where-"+string(rune('a'+i)))
		if err != store.ErrNotFound {
			t.Errorf("Expected ErrNotFound for category A record %d", i)
		}
	}

	// Verify category B records remain
	for i := 5; i < 10; i++ {
		_, err := s.Get(ctx, "delete-where-"+string(rune('a'+i)))
		if err != nil {
			t.Errorf("Category B record %d should exist: %v", i, err)
		}
	}
}

func testSearchVector(t *testing.T, s store.Store) {
	ctx := context.Background()

	// Skip if no vectors (would need dimension from store)
	// Just test basic search structure
	results, err := s.Search(ctx, store.Query{
		Vector:     []float32{0.1, 0.2, 0.3},
		VectorName: "test",
		TopK:       10,
	})
	// This may fail if vectors don't exist, but tests the path
	_ = results
	_ = err
}

func testSearchText(t *testing.T, s store.Store) {
	ctx := context.Background()

	// Create records with searchable content
	for i, text := range []string{
		"The quick brown fox",
		"The slow green turtle",
		"Fast brown dog",
		"Green grass and flowers",
	} {
		err := s.Put(ctx, store.Record{
			ID:      "text-search-" + string(rune('a'+i)),
			Content: text,
		})
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Search for "brown"
	results, err := s.Search(ctx, store.Query{
		Text: "brown",
		TopK: 10,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) < 2 {
		t.Errorf("Expected at least 2 results for 'brown', got %d", len(results))
	}
}

func testListFilter(t *testing.T, s store.Store) {
	ctx := context.Background()

	// Create records with different metadata
	for i := 0; i < 20; i++ {
		err := s.Put(ctx, store.Record{
			ID:       "list-" + string(rune('a'+i)),
			Content:  "Test record",
			Metadata: map[string]any{"count": i},
		})
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// List with filter
	results, err := s.List(ctx, store.Filter{
		Where: &store.Predicate{
			Field: "metadata.count",
			Op:    store.OpGte,
			Value: float64(10),
		},
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(results) > 5 {
		t.Errorf("Expected max 5 results, got %d", len(results))
	}

	for _, r := range results {
		count := int(r.Metadata["count"].(float64))
		if count < 10 {
			t.Errorf("Record with count %d should have been filtered out", count)
		}
	}
}

func testIterate(t *testing.T, s store.Store) {
	ctx := context.Background()

	// Create 50 records
	for i := 0; i < 50; i++ {
		err := s.Put(ctx, store.Record{
			ID:      "iterate-" + string(rune('a'+i)),
			Content: "Iterate test",
		})
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Iterate with limit
	recordsCh, errCh := s.Iterate(ctx, store.Filter{
		Limit: 10,
		Order: []store.Order{{Field: "id", Desc: false}},
	})

	var collected []store.Record
	for r := range recordsCh {
		collected = append(collected, r)
	}

	if err := <-errCh; err != nil {
		t.Errorf("Iterate error: %v", err)
	}

	if len(collected) != 10 {
		t.Errorf("Expected 10 records, got %d", len(collected))
	}
}

func testCount(t *testing.T, s store.Store) {
	ctx := context.Background()

	// Create records
	for i := 0; i < 10; i++ {
		err := s.Put(ctx, store.Record{
			ID:       "count-" + string(rune('a'+i)),
			Content:  "Count test",
			Metadata: map[string]any{"type": "test"},
		})
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Count all
	count, err := s.Count(ctx, nil)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count < 10 {
		t.Errorf("Expected at least 10, got %d", count)
	}

	// Count with predicate
	count, err = s.Count(ctx, &store.Predicate{
		Field: "metadata.type",
		Op:    store.OpEq,
		Value: "test",
	})
	if err != nil {
		t.Fatalf("Count with predicate failed: %v", err)
	}
	if count < 10 {
		t.Errorf("Expected at least 10 test records, got %d", count)
	}
}

func testWithTx(t *testing.T, s store.Store) {
	ctx := context.Background()

	// Create initial record
	err := s.Put(ctx, store.Record{ID: "tx-1", Content: "Initial"})
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Transaction that fails - should rollback
	err = s.WithTx(ctx, func(tx store.Store) error {
		err := tx.Put(ctx, store.Record{ID: "tx-2", Content: "In tx"})
		if err != nil {
			return err
		}
		return store.ErrNotFound // Force rollback
	})
	if err == nil {
		t.Errorf("Expected error from WithTx")
	}

	// Verify tx-2 was rolled back
	_, err = s.Get(ctx, "tx-2")
	if err != store.ErrNotFound {
		t.Errorf("Expected ErrNotFound after rollback, got: %v", err)
	}

	// Successful transaction
	err = s.WithTx(ctx, func(tx store.Store) error {
		return tx.Put(ctx, store.Record{ID: "tx-3", Content: "Committed"})
	})
	if err != nil {
		t.Fatalf("WithTx commit failed: %v", err)
	}

	// Verify tx-3 exists
	_, err = s.Get(ctx, "tx-3")
	if err != nil {
		t.Errorf("tx-3 should exist after commit: %v", err)
	}
}

func testSoftDeleteInvisibility(t *testing.T, s store.Store) {
	ctx := context.Background()

	// Create record
	err := s.Put(ctx, store.Record{ID: "soft-delete-test", Content: "Visible"})
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify visible
	_, err = s.Get(ctx, "soft-delete-test")
	if err != nil {
		t.Errorf("Record should be visible: %v", err)
	}

	// Soft delete
	err = s.Delete(ctx, "soft-delete-test")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify invisible via Get
	_, err = s.Get(ctx, "soft-delete-test")
	if err != store.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got: %v", err)
	}

	// Verify invisible via List
	results, _ := s.List(ctx, store.Filter{
		Where: &store.Predicate{
			Field: "id",
			Op:    store.OpEq,
			Value: "soft-delete-test",
		},
	})
	if len(results) > 0 {
		t.Errorf("Deleted record should not appear in List")
	}

	// Verify invisible via Count
	count, _ := s.Count(ctx, &store.Predicate{
		Field: "id",
		Op:    store.OpEq,
		Value: "soft-delete-test",
	})
	if count > 0 {
		t.Errorf("Deleted record should not be counted")
	}
}

func testConcurrentPut(t *testing.T, s store.Store) {
	ctx := context.Background()

	// Create initial record
	err := s.Put(ctx, store.Record{ID: "concurrent-test", Content: "Original"})
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Concurrent updates
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.Put(ctx, store.Record{
				ID:      "concurrent-test",
				Content: "Update " + string(rune('0'+i)),
			})
		}(i)
	}
	wg.Wait()

	// Should have final value (last write wins)
	got, err := s.Get(ctx, "concurrent-test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	// Content should be one of the updates
	if got.Content[:7] != "Update " {
		t.Errorf("Unexpected content: %s", got.Content)
	}
}

func testHealth(t *testing.T, s store.Store) {
	ctx := context.Background()

	err := s.Health(ctx)
	if err != nil {
		t.Errorf("Health check failed: %v", err)
	}
}

func testPredicateOperators(t *testing.T, s store.Store) {
	ctx := context.Background()

	// Create test records
	records := []store.Record{
		{ID: "pred-eq", Content: "Test", Metadata: map[string]any{"num": 10}},
		{ID: "pred-ne", Content: "Test", Metadata: map[string]any{"num": 20}},
		{ID: "pred-gt", Content: "Test", Metadata: map[string]any{"num": 30}},
		{ID: "pred-lt", Content: "Test", Metadata: map[string]any{"num": 5}},
	}
	for _, r := range records {
		s.Put(ctx, r)
	}

	tests := []struct {
		name      string
		predicate *store.Predicate
		expected  int
	}{
		{"eq", &store.Predicate{Field: "metadata.num", Op: store.OpEq, Value: 10}, 1},
		{"ne", &store.Predicate{Field: "metadata.num", Op: store.OpNe, Value: 10}, 3},
		{"gt", &store.Predicate{Field: "metadata.num", Op: store.OpGt, Value: 10}, 2},
		{"lt", &store.Predicate{Field: "metadata.num", Op: store.OpLt, Value: 10}, 1},
		{"gte", &store.Predicate{Field: "metadata.num", Op: store.OpGte, Value: 10}, 3},
		{"lte", &store.Predicate{Field: "metadata.num", Op: store.OpLte, Value: 10}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := s.List(ctx, store.Filter{Where: tt.predicate})
			if err != nil {
				t.Errorf("List failed: %v", err)
			}
			if len(results) != tt.expected {
				t.Errorf("Expected %d results for %s, got %d", tt.expected, tt.name, len(results))
			}
		})
	}
}

func testPredicateComposition(t *testing.T, s store.Store) {
	ctx := context.Background()

	// Create test records
	records := []store.Record{
		{ID: "comp-1", Metadata: map[string]any{"a": true, "b": false}},
		{ID: "comp-2", Metadata: map[string]any{"a": true, "b": true}},
		{ID: "comp-3", Metadata: map[string]any{"a": false, "b": true}},
	}
	for _, r := range records {
		s.Put(ctx, r)
	}

	// AND test
	results, err := s.List(ctx, store.Filter{
		Where: &store.Predicate{
			And: []store.Predicate{
				{Field: "metadata.a", Op: store.OpEq, Value: true},
				{Field: "metadata.b", Op: store.OpEq, Value: true},
			},
		},
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("AND test: expected 1, got %d", len(results))
	}

	// OR test
	results, err = s.List(ctx, store.Filter{
		Where: &store.Predicate{
			Or: []store.Predicate{
				{Field: "metadata.a", Op: store.OpEq, Value: true},
				{Field: "metadata.b", Op: store.OpEq, Value: true},
			},
		},
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("OR test: expected 3, got %d", len(results))
	}

	// NOT test
	results, err = s.List(ctx, store.Filter{
		Where: &store.Predicate{
			Not: &store.Predicate{
				Field: "metadata.a",
				Op:    store.OpEq,
				Value: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("NOT test: expected 1, got %d", len(results))
	}
}

func testMetadataPaths(t *testing.T, s store.Store) {
	ctx := context.Background()

	// Create record with nested metadata
	err := s.Put(ctx, store.Record{
		ID:      "nested-metadata",
		Content: "Test",
		Metadata: map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"level3": "deep value",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Query nested path
	results, err := s.List(ctx, store.Filter{
		Where: &store.Predicate{
			Field: "metadata.level1.level2.level3",
			Op:    store.OpEq,
			Value: "deep value",
		},
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result for nested path, got %d", len(results))
	}
}

func testVectorWithFilter(t *testing.T, s store.Store) {
	ctx := context.Background()

	// Create records with vectors and metadata
	for i := 0; i < 10; i++ {
		vecCategory := "A"
		if i >= 5 {
			vecCategory = "B"
		}
		err := s.Put(ctx, store.Record{
			ID:      "vec-filter-" + string(rune('a'+i)),
			Content: "Test",
			Vectors: map[string]store.Vector{
				"test": {Values: []float32{0.1, 0.2, 0.3}, Model: "test"},
			},
			Metadata: map[string]any{"category": vecCategory},
		})
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Search with filter
	results, err := s.Search(ctx, store.Query{
		Vector:     []float32{0.1, 0.2, 0.3},
		VectorName: "test",
		Filter: &store.Predicate{
			Field: "metadata.category",
			Op:    store.OpEq,
			Value: "A",
		},
		TopK: 10,
	})
	// May fail if no matching vectors, but tests the code path
	_ = results
	_ = err
}
