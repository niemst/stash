package reasoner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// OpenAI uses the OpenAI-compatible SDK for reasoning tasks.
// Works with any OpenAI-compatible endpoint: api.openai.com, OpenRouter, local endpoints, etc.
// The model string is passed as-is to the API — no stripping or transformation.
type OpenAI struct {
	client openai.Client
	model  string
	driver string
}

// NewOpenAI creates an OpenAI reasoner.
// baseURL: the API endpoint (e.g. "https://api.openai.com/v1")
// apiKey: the API key for the endpoint
// driver: the driver name (e.g. "openai")
// model: required — the model string for this endpoint (e.g. "gpt-4o-mini")
// Returns error if apiKey or model is empty.
func NewOpenAI(baseURL, apiKey, driver, model string) (*OpenAI, error) {
	if apiKey == "" {
		return nil, errors.New("reasoner: apiKey is required")
	}
	if model == "" {
		return nil, errors.New("reasoner: model is required")
	}
	if driver == "" {
		return nil, errors.New("reasoner: driver is required")
	}

	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
	)

	return &OpenAI{
		client: client,
		model:  model,
		driver: driver,
	}, nil
}

// Model returns the model string as passed at construction.
func (o *OpenAI) Model() string {
	return o.model
}

// Driver returns the driver name as passed at construction.
func (o *OpenAI) Driver() string {
	return o.driver
}

// Reason synthesizes structured reasoning over the given texts using the OpenAI API.
// Combines all texts into a consolidation prompt and returns the LLM's response.
func (o *OpenAI) Reason(ctx context.Context, texts []string) (string, error) {
	if len(texts) == 0 {
		return "", errors.New("reasoner: texts must not be empty")
	}

	// Build prompt: ask LLM to synthesize events into a fact
	eventsList := strings.Join(texts, "\n- ")
	prompt := fmt.Sprintf(`You are a memory synthesis engine. Given raw observations (events), distill them into a single durable fact.

Events:
- %s

Output a single, declarative fact statement (1–2 sentences). Focus on what is true, not when or how often.
Example: "Mohamed prefers Go for systems programming" (not "Mohamed mentioned Go three times").

Fact:`, eventsList)

	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: o.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
	})
	if err != nil {
		return "", fmt.Errorf("chat.completions call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("reasoner: no response from LLM")
	}

	// Extract synthesized fact from response
	fact := resp.Choices[0].Message.Content
	fact = strings.TrimSpace(fact)

	return fact, nil
}

// ReasonStructured synthesizes structured reasoning and extracts entity, property, value.
// Asks the LLM to output in a parseable format: "Entity: X | Property: Y | Value: Z | Summary: ..."
func (o *OpenAI) ReasonStructured(ctx context.Context, texts []string) (*StructuredFact, error) {
	if len(texts) == 0 {
		return nil, errors.New("reasoner: texts must not be empty")
	}

	// Build prompt: ask LLM for structured output
	eventsList := strings.Join(texts, "\n- ")
	prompt := fmt.Sprintf(`You are a memory synthesis engine. Given raw observations (events), extract the key fact.

Events:
- %s

Extract and output in this exact format (each field on its own line):
Entity: [subject name or identifier]
Property: [attribute or predicate]
Value: [the fact value]
Summary: [1-2 sentence natural language fact statement]

Example:
Entity: Mohamed
Property: preferred_language
Value: Go
Summary: Mohamed prefers Go for systems programming.

Now extract:`, eventsList)

	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: o.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("chat.completions call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, errors.New("reasoner: no response from LLM")
	}

	output := strings.TrimSpace(resp.Choices[0].Message.Content)

	// Parse structured output
	sf := &StructuredFact{}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Entity:") {
			sf.Entity = strings.TrimSpace(strings.TrimPrefix(line, "Entity:"))
		} else if strings.HasPrefix(line, "Property:") {
			sf.Property = strings.TrimSpace(strings.TrimPrefix(line, "Property:"))
		} else if strings.HasPrefix(line, "Value:") {
			sf.Value = strings.TrimSpace(strings.TrimPrefix(line, "Value:"))
		} else if strings.HasPrefix(line, "Summary:") {
			sf.Summary = strings.TrimSpace(strings.TrimPrefix(line, "Summary:"))
		}
	}

	// If summary is empty, use full output
	if sf.Summary == "" {
		sf.Summary = output
	}

	return sf, nil
}

// ReasonRelationships extracts relationships between entities from a fact.
// Parses multi-line output with format: "From: X | RelationType: Y | To: Z | Confidence: N"
func (o *OpenAI) ReasonRelationships(ctx context.Context, factContent string) ([]*StructuredRelationship, error) {
	if factContent == "" {
		return nil, errors.New("reasoner: factContent must not be empty")
	}

	prompt := fmt.Sprintf(`You are extracting semantic relationships from a fact for a knowledge graph.

Fact: %s

Identify all entities and relationships in this fact. For each relationship:
- From: the subject entity name (exact as mentioned in fact)
- RelationType: a simple, lowercase relationship type (e.g., works_at, located_in, manages, knows, created_by)
- To: the object entity name (exact as mentioned in fact)
- Confidence: 0.7-1.0 (how confident this relationship is valid)

Format each relationship on its own line exactly like this:
From: Subject | RelationType: type_name | To: Object | Confidence: 0.85

Output only the relationships, one per line. If no relationships found, output nothing.

Example fact: "Alice is an engineer at TechCorp in Paris"
From: Alice | RelationType: works_at | To: TechCorp | Confidence: 0.9
From: TechCorp | RelationType: located_in | To: Paris | Confidence: 0.85
From: Alice | RelationType: role_at | To: engineer | Confidence: 0.8

Now extract relationships from the given fact:`, factContent)

	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: o.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("chat.completions call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, errors.New("reasoner: no response from LLM")
	}

	output := strings.TrimSpace(resp.Choices[0].Message.Content)
	return parseRelationships(output), nil
}

// parseRelationships parses multi-line relationship output.
// Each line should be: "From: X | RelationType: Y | To: Z | Confidence: N"
func parseRelationships(output string) []*StructuredRelationship {
	var relationships []*StructuredRelationship
	if output == "" {
		return relationships
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		rel := parseRelationshipLine(line)
		if rel != nil && rel.FromEntity != "" && rel.RelationType != "" && rel.ToEntity != "" {
			if rel.Confidence == 0 {
				rel.Confidence = 0.7 // default confidence if not specified
			}
			relationships = append(relationships, rel)
		}
	}

	return relationships
}

// parseRelationshipLine parses a single relationship line.
func parseRelationshipLine(line string) *StructuredRelationship {
	// Expected format: "From: X | RelationType: Y | To: Z | Confidence: N"
	parts := strings.Split(line, "|")
	if len(parts) < 3 {
		return nil
	}

	rel := &StructuredRelationship{}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "From:") {
			rel.FromEntity = strings.TrimSpace(strings.TrimPrefix(part, "From:"))
		} else if strings.HasPrefix(part, "RelationType:") {
			rel.RelationType = strings.TrimSpace(strings.TrimPrefix(part, "RelationType:"))
		} else if strings.HasPrefix(part, "To:") {
			rel.ToEntity = strings.TrimSpace(strings.TrimPrefix(part, "To:"))
		} else if strings.HasPrefix(part, "Confidence:") {
			confStr := strings.TrimSpace(strings.TrimPrefix(part, "Confidence:"))
			var conf float32
			fmt.Sscanf(confStr, "%f", &conf)
			if conf > 0 && conf <= 1.0 {
				rel.Confidence = conf
			}
		}
	}

	return rel
}
