package postgres

import (
	"context"
	"testing"

	"github.com/alash3al/stash/internal/store/storetest"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestPostgresStore(t *testing.T) {
	ctx := context.Background()

	// Start Postgres with pgvector
	container, err := postgres.Run(ctx,
		"pgvector/pgvector:pg16",
		postgres.WithDatabase("stash_test"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2)),
	)
	if err != nil {
		t.Fatalf("failed to start container: %v", err)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	}()

	connStr, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	// Create store
	cfg := Config{
		DSN:             connStr,
		VectorDim:       3, // Match test vectors
		IndexedMetadata: []string{"category", "source"},
		MaxResultSize:   1000,
	}

	s, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	// Run the shared test suite
	storetest.RunSuite(t, s)
}
