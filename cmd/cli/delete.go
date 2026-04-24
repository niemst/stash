package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/urfave/cli/v3"
)

func deleteCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("event ID argument is required")
	}

	eventID := args.First()
	if eventID == "" {
		return fmt.Errorf("event ID cannot be empty")
	}

	bc, ok := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
	if !ok {
		return fmt.Errorf("bootstrap context not available")
	}

	if err := bc.Store.Delete(ctx, eventID); err != nil {
		return err
	}

	output := map[string]interface{}{
		"success": true,
		"deleted": 1,
		"id":      eventID,
	}

	jsonOutput, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	fmt.Println(string(jsonOutput))
	return nil
}
