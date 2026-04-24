package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/urfave/cli/v3"
)

func recallCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("query argument is required")
	}

	query := args.First()
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("query cannot be empty")
	}

	namespaces := cmd.StringSlice("namespace")
	limit := cmd.Int("limit")
	if limit <= 0 {
		limit = 10
	}

	whereStr := cmd.String("where")
	filter, err := parseFilterDSL(whereStr)
	if err != nil {
		return fmt.Errorf("invalid --where flag: %w", err)
	}

	bc, ok := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
	if !ok {
		return fmt.Errorf("bootstrap context not available")
	}

	var events interface{}
	if filter != nil {
		events, err = bc.Memory.RecallWhere(ctx, namespaces, query, filter, limit)
	} else {
		events, err = bc.Memory.Recall(ctx, namespaces, query, limit)
	}
	if err != nil {
		return err
	}

	jsonOutput, err := json.Marshal(events)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	fmt.Println(string(jsonOutput))
	return nil
}
