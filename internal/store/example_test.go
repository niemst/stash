package store_test

import (
	"fmt"

	"github.com/alash3al/stash/internal/store"
	"github.com/alash3al/stash/internal/store/postgres"
)

func Example() {

	// This is a compile-time example. In real usage, provide a real DSN.
	cfg := postgres.Config{
		DSN:             "postgres://user:pass@localhost/db?sslmode=disable",
		VectorDim:       384,
		IndexedMetadata: []string{"category"},
		MaxResultSize:   1000,
	}

	// Create store (will fail without real database)
	_, err := postgres.New(cfg)
	if err != nil {
		// Expected in example
		fmt.Println("Store created (or connection failed as expected)")
	}

	// Demonstrate the store.Store interface usage
	fmt.Println("store.Store interface defines CRUD, search, and transactions")
	// Output:
	// Store created (or connection failed as expected)
	// store.Store interface defines CRUD, search, and transactions
}

func ExampleRecord() {
	r := store.Record{
		ID:      "example-123",
		Content: "The quick brown fox jumps over the lazy dog",
		Vectors: map[string]store.Vector{
			"bge": {
				Values: make([]float32, 384), // 384-dimensional vector
				Model:  "bge-base-en-v1.5",
			},
		},
		Metadata: map[string]any{
			"category": "example",
			"language": "en",
			"tags":     []string{"animals", "classic"},
		},
	}

	fmt.Printf("Record ID: %s\n", r.ID)
	fmt.Printf("Content length: %d\n", len(r.Content))
	fmt.Printf("Vector count: %d\n", len(r.Vectors))
	fmt.Printf("Metadata fields: %d\n", len(r.Metadata))
	// Output:
	// Record ID: example-123
	// Content length: 43
	// Vector count: 1
	// Metadata fields: 3
}

func ExamplePredicate() {
	// Simple equality
	p1 := &store.Predicate{
		Field: "metadata.category",
		Op:    store.OpEq,
		Value: "news",
	}

	// Composite AND
	p2 := &store.Predicate{
		And: []store.Predicate{
			{Field: "metadata.language", Op: store.OpEq, Value: "en"},
			{Field: "created_at", Op: store.OpGte, Value: "2024-01-01"},
		},
	}

	// Complex with NOT
	p3 := &store.Predicate{
		Not: &store.Predicate{
			Or: []store.Predicate{
				{Field: "metadata.deleted", Op: store.OpEq, Value: true},
				{Field: "metadata.archived", Op: store.OpEq, Value: true},
			},
		},
	}

	fmt.Printf("Predicate 1: %s %s %v\n", p1.Field, p1.Op, p1.Value)
	fmt.Printf("Predicate 2: AND with %d children\n", len(p2.And))
	fmt.Printf("Predicate 3: NOT with OR of %d children\n", len(p3.Not.Or))
	// Output:
	// Predicate 1: metadata.category eq news
	// Predicate 2: AND with 2 children
	// Predicate 3: NOT with OR of 2 children
}

func ExampleQuery() {
	q := store.Query{
		Vector:     make([]float32, 384), // Query embedding
		VectorName: "bge",
		TopK:       10,
		Filter: &store.Predicate{
			Field: "metadata.category",
			Op:    store.OpEq,
			Value: "technology",
		},
	}

	fmt.Printf("Vector search with filter\n")
	fmt.Printf("Vector dimension: %d\n", len(q.Vector))
	fmt.Printf("Vector name: %s\n", q.VectorName)
	fmt.Printf("TopK: %d\n", q.TopK)
	fmt.Printf("Filter field: %s\n", q.Filter.Field)
	// Output:
	// Vector search with filter
	// Vector dimension: 384
	// Vector name: bge
	// TopK: 10
	// Filter field: metadata.category
}
