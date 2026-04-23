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
				Name:  "events",
				Usage: "Manage events",
				Commands: []*cli.Command{
					{
						Name:    "create",
						Aliases: []string{"remember"},
						Usage:   "Store an event in memory",
						Action:  rememberCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "namespace",
								Usage: "Namespace for the event",
							},
							&cli.StringFlag{
								Name:  "metadata",
								Usage: "JSON metadata for the event",
							},
						},
					},
					{
						Name:    "search",
						Aliases: []string{"recall"},
						Usage:   "Search for relevant events",
						Action:  recallCmd,
						Flags: []cli.Flag{
							&cli.StringSliceFlag{
								Name:  "namespace",
								Usage: "Namespaces to search (comma-separated or repeated)",
							},
							&cli.IntFlag{
								Name:  "limit",
								Usage: "Maximum number of results",
								Value: 10,
							},
							&cli.StringFlag{
								Name:  "where",
								Usage: "Metadata filter in format: field=value,field>=value,... (supports =, !=, <, >, <=, >=)",
							},
						},
					},
					{
						Name:   "list",
						Usage:  "List recent events",
						Action: listCmd,
						Flags: []cli.Flag{
							&cli.StringSliceFlag{
								Name:  "namespace",
								Usage: "Namespaces to list (comma-separated or repeated)",
							},
							&cli.IntFlag{
								Name:  "limit",
								Usage: "Maximum number of results",
								Value: 20,
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
				},
			},
			{
				Name:  "context",
				Usage: "Manage working memory context",
				Commands: []*cli.Command{
					{
						Name:   "show",
						Usage:  "View current working memory context",
						Action: contextShowCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "namespace",
								Usage: "Namespace for the context",
							},
						},
					},
					{
						Name:   "update",
						Usage:  "Update the focus of working memory",
						Action: contextUpdateCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "namespace",
								Usage: "Namespace for the context",
							},
						},
					},
				},
			},
			{
				Name:  "facts",
				Usage: "Manage facts and cognitive processes",
				Commands: []*cli.Command{
					{
						Name:   "consolidate",
						Usage:  "Synthesize recent events into facts",
						Action: consolidateCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "namespace",
								Usage: "Namespace to consolidate (optional)",
							},
							&cli.StringFlag{
								Name:  "window",
								Usage: "Time window for recent events (e.g., 1h, 30m)",
								Value: "1h",
							},
							&cli.IntFlag{
								Name:  "limit",
								Usage: "Maximum number of events to process",
								Value: 100,
							},
						},
					},
					{
						Name:   "contradictions",
						Usage:  "Find contradictions in facts",
						Action: contradictionsCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "namespace",
								Usage: "Namespace to check (optional)",
							},
						},
					},
					{
						Name:   "reflect",
						Usage:  "Introspect memory state and generate report",
						Action: reflectCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "namespace",
								Usage: "Namespace to reflect on (optional)",
							},
						},
					},
					{
						Name:   "reinforce",
						Usage:  "Strengthen a fact by increasing observation count",
						Action: reinforceCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "entity",
								Usage:    "Entity identifier (required)",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "property",
								Usage:    "Property name (required)",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "value",
								Usage:    "Property value (required)",
								Required: true,
							},
							&cli.IntFlag{
								Name:  "count",
								Usage: "Number of times to reinforce",
								Value: 1,
							},
						},
					},
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
