package main

import (
	"context"
	"fmt"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/urfave/cli/v3"
)

func purgeCmd(ctx context.Context, cmd *cli.Command) error {
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

	if err := bc.Store.Purge(ctx, eventID); err != nil {
		return err
	}

	fmt.Printf("Event purged (hard delete): %s\n", eventID)
	return nil
}