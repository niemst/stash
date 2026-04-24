package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/urfave/cli/v3"
)

func contextShowCmd(ctx context.Context, cmd *cli.Command) error {
	bc, ok := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
	if !ok {
		return fmt.Errorf("bootstrap context not available")
	}

	namespace := cmd.String("namespace")

	wm, err := bc.Memory.WorkingMemory(ctx, namespace, "")
	if err != nil {
		return err
	}

	output := map[string]interface{}{
		"id":         wm.ID,
		"focus":      wm.Focus,
		"event_ids":  wm.EventIDs,
		"created_at": wm.CreatedAt,
		"updated_at": wm.UpdatedAt,
		"expires_at": wm.ExpiresAt,
		"expires_in": time.Until(wm.ExpiresAt).Round(time.Second).String(),
	}

	jsonOutput, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	fmt.Println(string(jsonOutput))
	return nil
}

func contextUpdateCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("focus argument is required")
	}

	focus := args.First()
	namespace := cmd.String("namespace")

	bc, ok := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
	if !ok {
		return fmt.Errorf("bootstrap context not available")
	}

	wm, err := bc.Memory.WorkingMemory(ctx, namespace, focus)
	if err != nil {
		return err
	}

	output := map[string]interface{}{
		"success": true,
		"focus":   wm.Focus,
		"id":      wm.ID,
	}

	jsonOutput, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	fmt.Println(string(jsonOutput))
	return nil
}
