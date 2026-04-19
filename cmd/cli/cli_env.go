package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/urfave/cli/v3"
)

func EnvCmd(ctx context.Context, cmd *cli.Command) error {
	// Get the shared bootstrap context from root command metadata
	rootCmd := cmd.Root()
	if rootCmd == nil {
		return fmt.Errorf("root command not found")
	}
	bootstrapCtx, ok := rootCmd.Metadata["bootstrapCtx"].(*bootstrap.Context)
	if !ok {
		return fmt.Errorf("bootstrap context not found in metadata")
	}

	// Use same logic as bootstrap to determine config file
	filename := os.Getenv("STASHCONFIG")

	// Output config details using a table
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleLight)
	t.AppendHeader(table.Row{"Configuration Key", "Value"})
	t.AppendRows([]table.Row{
		{"ConfigFile", filename},
		{"STASHCONFIG Env", os.Getenv("STASHCONFIG")},
		{"Store Driver", bootstrapCtx.Config.StoreDriver},
		{"Store DSN", MaskDSN(bootstrapCtx.Config.StoreDSN)},
		{"Vector Dimension", bootstrapCtx.Config.VectorDim},
		{"Max Result Size", bootstrapCtx.Config.MaxResultSize},
		{"Embedder Driver", bootstrapCtx.Config.EmbedderDriver},
		{"OpenAI API Key", MaskAPIKey(bootstrapCtx.Config.OpenAIAPIKey)},
		{"OpenAI Base URL", bootstrapCtx.Config.OpenAIBaseURL},
		{"Embedding Model", bootstrapCtx.Config.EmbeddingModel},
		{"Frame TTL", bootstrapCtx.Config.FrameTTL},
		{"HTTP Addr", bootstrapCtx.Config.HTTPAddr},
		{"Log Level", bootstrapCtx.Config.LogLevel},
		{"Log Format", bootstrapCtx.Config.LogFormat},
	})
	fmt.Println("=== Stash Configuration ===")
	t.Render()
	fmt.Println()

	// Log bootstrap status using the shared logger
	bootstrapCtx.Logger.Info("Bootstrap successful",
		"StoreInitialized", bootstrapCtx.Store != nil,
		"EmbedderInitialized", bootstrapCtx.Embedder != nil,
		"MemoryInitialized", bootstrapCtx.Memory != nil,
	)

	return nil
}

func MaskDSN(dsn string) string {
	if len(dsn) > 50 {
		return dsn[:20] + "..." + dsn[len(dsn)-20:]
	}
	return dsn
}

func MaskAPIKey(key string) string {
	if len(key) < 8 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-4:]
}
