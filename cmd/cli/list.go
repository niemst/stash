package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/alash3al/stash/internal/store"
	"github.com/urfave/cli/v3"
)

func listCmd(ctx context.Context, cmd *cli.Command) error {
	bc, ok := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
	if !ok {
		return fmt.Errorf("bootstrap context not available")
	}

	limit := cmd.Int("limit")
	if limit <= 0 {
		limit = 20
	}

	records, err := bc.Store.List(ctx, store.Filter{
		Limit: limit,
		Where: &store.Predicate{
			Field: "metadata._memory.type",
			Op:    store.OpEq,
			Value: "event",
		},
	})
	if err != nil {
		return err
	}

	if cmd.Bool("json") {
		output, err := json.MarshalIndent(records, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal records to JSON: %w", err)
		}
		fmt.Println(string(output))
		return nil
	}

	if len(records) == 0 {
		fmt.Println("No events found.")
		return nil
	}

	for _, record := range records {
		content := record.Content
		if len(content) > 60 {
			content = content[:57] + "..."
		}

		fmt.Printf("• %s\n", content)
		fmt.Printf("  ID: %s | Created: %s\n", record.ID, record.CreatedAt.Format("2006-01-02 15:04:05"))

		if len(record.Metadata) > 0 {
			// Filter out _memory metadata for display
			displayMeta := make(map[string]any)
			for k, v := range record.Metadata {
				if !strings.HasPrefix(k, "_memory") {
					displayMeta[k] = v
				}
			}
			if len(displayMeta) > 0 {
				metadataStr, _ := json.Marshal(displayMeta)
				fmt.Printf("  Metadata: %s\n", metadataStr)
			}
		}
		fmt.Println()
	}

	return nil
}