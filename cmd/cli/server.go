package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/urfave/cli/v3"
)

// Request/Response types for HTTP API
type AddFactRequest struct {
	Content    string            `json:"content"`
	Confidence float64           `json:"confidence"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type EventResponse struct {
	ID        string            `json:"id"`
	Content   string            `json:"content"`
	Namespace string            `json:"namespace"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt string            `json:"created_at"`
}

type FactResponse struct {
	ID               string  `json:"id"`
	Content          string  `json:"content"`
	Type             string  `json:"type"`
	Confidence       float32 `json:"confidence"`
	ObservationCount int     `json:"observation_count"`
	Source           string  `json:"source"`
	Score            float32 `json:"score,omitempty"` // For ranked results
	ValidFrom        string  `json:"valid_from"`
}

type RecallFactsResponse struct {
	Query     string         `json:"query"`
	Namespace string         `json:"namespace"`
	Ranked    bool           `json:"ranked"`
	Limit     int            `json:"limit"`
	Facts     []FactResponse `json:"facts"`
}

func serverCmd(ctx context.Context, cmd *cli.Command) error {
	port := cmd.String("port")
	host := cmd.String("host")

	bc, ok := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
	if !ok {
		return fmt.Errorf("bootstrap context not available")
	}

	e := echo.New()

	// Middleware
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	// Routes - Core agent operations only
	e.POST("/api/v1/facts", addFactHandler(bc))
	e.GET("/api/v1/facts", recallFactsHandler(bc))

	// Health check
	e.GET("/health", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	fmt.Printf("Starting server on %s:%s\n", host, port)
	return e.Start(host + ":" + port)
}

func addFactHandler(bc *bootstrap.Context) echo.HandlerFunc {
	return func(c *echo.Context) error {
		var req AddFactRequest
		// Use c.Bind for Echo v4
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
		}

		if strings.TrimSpace(req.Content) == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "content is required"})
		}

		namespace := c.QueryParam("namespace")
		metadata := make(map[string]any)
		if req.Metadata != nil {
			for k, v := range req.Metadata {
				metadata[k] = v
			}
		}

		// Remember event using memory interface
		eventID, err := bc.Memory.Remember(c.Request().Context(), namespace, req.Content, metadata)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		response := map[string]string{
			"id":      eventID,
			"message": "Event remembered successfully",
		}

		return c.JSON(http.StatusCreated, response)
	}
}

func recallFactsHandler(bc *bootstrap.Context) echo.HandlerFunc {
	return func(c *echo.Context) error {
		query := c.QueryParam("query")
		if strings.TrimSpace(query) == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "query is required"})
		}

		namespace := c.QueryParam("namespace")
		limit := 10
		if l := c.QueryParam("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}

		ranked := c.QueryParam("ranked") == "true"

		var facts interface{}
		var err error

		if ranked {
			// Use confidence-ranked retrieval
			factsList, err := bc.Memory.RecallFactsRanked(c.Request().Context(), namespace, query, limit)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}

			output := make([]FactResponse, len(factsList))
			for i, fact := range factsList {
				output[i] = FactResponse{
					ID:               fact.ID,
					Content:          fact.Content,
					Type:             fact.Type,
					Confidence:       fact.Confidence,
					ObservationCount: fact.ObservationCount,
					Source:           fact.Source,
					Score:            fact.Score,
					ValidFrom:        fact.ValidFrom.Format("2006-01-02T15:04:05Z"),
				}
			}
			facts = output
		} else {
			// Query by type
			factsList, err := bc.Memory.QueryFactsByType(c.Request().Context(), namespace, "state")
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}

			output := make([]FactResponse, len(factsList))
			for i, fact := range factsList {
				output[i] = FactResponse{
					ID:               fact.ID,
					Content:          fact.Content,
					Type:             fact.Type,
					Confidence:       fact.Confidence,
					ObservationCount: fact.ObservationCount,
					Source:           fact.Source,
					ValidFrom:        fact.ValidFrom.Format("2006-01-02T15:04:05Z"),
				}
			}
			facts = output
		}

		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		response := RecallFactsResponse{
			Query:     query,
			Namespace: namespace,
			Ranked:    ranked,
			Limit:     limit,
			Facts:     facts.([]FactResponse),
		}

		return c.JSON(http.StatusOK, response)
	}
}
