package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/alash3al/stash/internal/config"
	"github.com/alash3al/stash/internal/embedder"
	"github.com/alash3al/stash/internal/memory"
	"github.com/alash3al/stash/internal/reasoner"
	"github.com/alash3al/stash/internal/store"
	"github.com/alash3al/stash/internal/store/postgres"
)

type Context struct {
	Config   *config.Config
	Store    store.Store
	Embedder embedder.Embedder
	Reasoner reasoner.Reasoner
	Memory   *memory.Memory
	Logger   *slog.Logger
}

func MustNew(ctx context.Context) *Context {
	bootstrapCtx, err := New(ctx)
	if err != nil {
		panic(fmt.Sprintf("bootstrap failed: %v", err))
	}
	return bootstrapCtx
}

func New(ctx context.Context) (*Context, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	var h slog.Handler
	opts := &slog.HandlerOptions{}

lvl := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return nil, fmt.Errorf("unknown log level: %q", cfg.LogLevel)
	}
	opts.Level = lvl

	if cfg.LogFormat == "json" {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	logger := slog.New(h)

	str, err := buildStore(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("build store: %w", err)
	}

	emb, err := buildEmbedder(cfg)
	if err != nil {
		str.Close()
		return nil, fmt.Errorf("build embedder: %w", err)
	}

	if emb.Dims() != cfg.VectorDim {
		str.Close()
		return nil, fmt.Errorf("vector dimension mismatch: embedder returns %d, config expects %d", emb.Dims(), cfg.VectorDim)
	}

	reas, err := buildReasoner(cfg)
	if err != nil {
		str.Close()
		return nil, fmt.Errorf("build reasoner: %w", err)
	}

	mem, err := memory.New(str, emb, reas)
	if err != nil {
		str.Close()
		return nil, fmt.Errorf("build memory: %w", err)
	}

	return &Context{
		Config:   cfg,
		Store:    str,
		Embedder: emb,
		Reasoner: reas,
		Memory:   mem,
		Logger:   logger,
	}, nil
}

func (c *Context) Close() error {
	var errs []string
	if c.Memory != nil {
		if err := c.Memory.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("memory: %v", err))
		}
	}
	if c.Store != nil {
		if err := c.Store.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("store: %v", err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func loadConfig() (*config.Config, error) {
	filename := os.Getenv("STASHCONFIG")
	if filename == "" {
		filename = ".env"
	}
	return config.NewFromFile(filename)
}

func buildStore(ctx context.Context, cfg *config.Config) (store.Store, error) {
	pgCfg := postgres.Config{
		DSN:             cfg.StoreDSN,
		VectorDim:       cfg.VectorDim,
		IndexedMetadata: []string{}, // TODO: make configurable
		MaxResultSize:   cfg.MaxResultSize,
	}
	return postgres.New(pgCfg)
}

func buildEmbedder(cfg *config.Config) (embedder.Embedder, error) {
	return embedder.NewOpenAI(
		cfg.OpenAIBaseURL,
		cfg.OpenAIAPIKey,
		cfg.EmbeddingModel,
		cfg.VectorDim,
	)
}

func buildReasoner(cfg *config.Config) (reasoner.Reasoner, error) {
	return reasoner.NewOpenAI(
		cfg.OpenAIBaseURL,
		cfg.OpenAIAPIKey,
		"openai", // driver name (for logging)
		cfg.ReasonerModel,
	)
}