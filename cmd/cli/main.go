package main

import (
	"context"
	"log"
	"os"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "stash",
		Usage: "Stash - Memory layer for AI applications",
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			bc, err := bootstrap.New(ctx)
			if err != nil {
				return ctx, err
			}
			cmd.Metadata["bootstrapCtx"] = bc
			return ctx, nil
		},
		After: func(ctx context.Context, cmd *cli.Command) error {
			if bc, ok := cmd.Metadata["bootstrapCtx"].(*bootstrap.Context); ok {
				return bc.Close()
			}
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:   "env",
				Usage:  "Show environment variables and configuration",
				Action: EnvCmd,
			},
			{
				Name:   "remember",
				Usage:  "Store an event in memory",
				Action: rememberCmd,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "metadata",
						Usage: "JSON metadata for the event",
					},
				},
			},
			{
				Name:   "recall",
				Usage:  "Search for relevant events",
				Action: recallCmd,
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:  "limit",
						Usage: "Maximum number of results",
						Value: 10,
					},
					&cli.BoolFlag{
						Name:  "json",
						Usage: "Output results as JSON",
					},
				},
			},
			{
				Name:   "context",
				Usage:  "View or update working memory context",
				Action: contextCmd,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "update",
						Usage: "Update the focus of working memory (empty string clears focus)",
					},
				},
			},
			{
				Name:   "delete",
				Usage:  "Soft-delete an event by ID",
				Action: deleteCmd,
			},
			{
				Name:   "purge",
				Usage:  "Hard-delete an event by ID",
				Action: purgeCmd,
			},
			{
				Name:   "list",
				Usage:  "List recent events",
				Action: listCmd,
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:  "limit",
						Usage: "Maximum number of results",
						Value: 20,
					},
					&cli.BoolFlag{
						Name:  "json",
						Usage: "Output results as JSON",
					},
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
