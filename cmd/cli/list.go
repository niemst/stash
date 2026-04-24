package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/alash3al/stash/internal/store"
	"github.com/urfave/cli/v3"
)

func listCmd(ctx context.Context, cmd *cli.Command) error {
	bc, ok := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
	if !ok {
		return fmt.Errorf("bootstrap context not available")
	}

	namespaces := cmd.StringSlice("namespace")
	limit := cmd.Int("limit")
	if limit <= 0 {
		limit = 20
	}

	records, err := bc.Store.List(ctx, store.Filter{
		Namespaces: namespaces,
		Limit:      limit,
		Where: &store.Predicate{
			Field: "metadata._memory.type",
			Op:    store.OpEq,
			Value: "event",
		},
	})
	if err != nil {
		return err
	}

	type eventItem struct {
		ID        string         `json:"id"`
		Namespace string         `json:"namespace"`
		Content   string         `json:"content"`
		Metadata  map[string]any `json:"metadata,omitempty"`
		Timestamp time.Time      `json:"timestamp"`
	}

	events := make([]eventItem, 0, len(records))
	for _, record := range records {
		var timestamp time.Time
		if memMeta, ok := record.Metadata["_memory"].(map[string]any); ok {
			if tsStr, ok := memMeta["timestamp"].(string); ok {
				if ts, err := time.Parse(time.RFC3339, tsStr); err == nil {
					timestamp = ts
				}
			}
		}
		if timestamp.IsZero() {
			timestamp = record.CreatedAt
		}

		userMetadata := make(map[string]any)
		for k, v := range record.Metadata {
			if k != "_memory" {
				userMetadata[k] = v
			}
		}

		events = append(events, eventItem{
			ID:        record.ID,
			Namespace: record.Namespace,
			Content:   record.Content,
			Metadata:  userMetadata,
			Timestamp: timestamp,
		})
	}

	jsonOutput, err := json.Marshal(map[string]interface{}{"events": events})
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	fmt.Println(string(jsonOutput))
	return nil
}
