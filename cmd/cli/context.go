package main

import (
	"context"
	"fmt"
	"time"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/urfave/cli/v3"
)

func contextCmd(ctx context.Context, cmd *cli.Command) error {
	updateFocus := cmd.String("update")

	bc, ok := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
	if !ok {
		return fmt.Errorf("bootstrap context not available")
	}

	wm, err := bc.Memory.WorkingMemory(ctx, updateFocus)
	if err != nil {
		return err
	}

	if updateFocus != "" {
		fmt.Println("Context updated")
		return nil
	}

	// Display current working memory
	if wm.Focus == "" {
		fmt.Println("Focus: (empty)")
	} else {
		fmt.Printf("Focus: %s\n", wm.Focus)
	}
	fmt.Printf("ID: %s\n", wm.ID)
	fmt.Printf("Created: %s\n", wm.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated: %s\n", wm.UpdatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Expires: %s (in %s)\n",
		wm.ExpiresAt.Format("2006-01-02 15:04:05"),
		time.Until(wm.ExpiresAt).Round(time.Second))

	if len(wm.EventIDs) == 0 {
		fmt.Println("Events: none")
	} else {
		fmt.Printf("Events (%d):\n", len(wm.EventIDs))
		// Show first 5 event IDs
		maxShow := 5
		if len(wm.EventIDs) < maxShow {
			maxShow = len(wm.EventIDs)
		}
		for i := 0; i < maxShow; i++ {
			fmt.Printf("  • %s\n", wm.EventIDs[i])
		}
		if len(wm.EventIDs) > maxShow {
			fmt.Printf("  … and %d more\n", len(wm.EventIDs)-maxShow)
		}
	}

	return nil
}
