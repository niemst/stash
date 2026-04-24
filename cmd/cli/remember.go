package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/urfave/cli/v3"
)

func rememberCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("content argument is required")
	}

	content := args.First()
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("content cannot be empty")
	}

	namespace := cmd.String("namespace")

	var metadata map[string]any
	if metadataFlag := cmd.String("metadata"); metadataFlag != "" {
		if err := json.Unmarshal([]byte(metadataFlag), &metadata); err != nil {
			return fmt.Errorf("invalid metadata JSON: %w", err)
		}
	}

	bc, ok := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
	if !ok {
		return fmt.Errorf("bootstrap context not available")
	}

	eventID, err := bc.Memory.Remember(ctx, namespace, content, metadata)
	if err != nil {
		return err
	}

	output := map[string]string{
		"id":      eventID,
		"message": "Event remembered successfully",
	}

	jsonOutput, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	fmt.Println(string(jsonOutput))
	return nil
}
