package reasoner

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"strings"
)

// Fake returns deterministic reasoning results for testing.
// Same input always produces the same output.
// No external calls. No API key required.
// NOT suitable for semantic correctness testing — only plumbing tests.
type Fake struct {
	model  string
	driver string
}

// NewFake creates a Fake reasoner.
// driver: the driver name (e.g. "fake")
// model: the model name (e.g. "fake" or any string for testing)
func NewFake(driver, model string) *Fake {
	if driver == "" {
		driver = "fake"
	}
	if model == "" {
		model = "fake"
	}
	return &Fake{
		model:  model,
		driver: driver,
	}
}

// Model returns the model string as passed at construction.
func (f *Fake) Model() string {
	return f.model
}

// Driver returns the driver name as passed at construction.
func (f *Fake) Driver() string {
	return f.driver
}

// Reason returns a deterministic synthetic fact based on input texts.
// Same texts always produce the same output.
// Format: "Synthesized fact from <N> texts: <hash-based summary>"
func (f *Fake) Reason(_ context.Context, texts []string) (string, error) {
	if len(texts) == 0 {
		return "", errors.New("reasoner: texts must not be empty")
	}

	// Combine all texts for hashing
	combined := strings.Join(texts, "\n")

	// Compute deterministic hash-based summary
	hash := md5.Sum([]byte(combined))
	hashStr := fmt.Sprintf("%x", hash)[:8] // First 8 hex chars

	// Return consistent synthesized fact
	return fmt.Sprintf("Synthesized fact from %d texts: [%s]", len(texts), hashStr), nil
}

// ReasonStructured returns a deterministic structured fact.
// For testing only. Entity, Property, Value are fixed dummy values.
func (f *Fake) ReasonStructured(_ context.Context, texts []string) (*StructuredFact, error) {
	if len(texts) == 0 {
		return nil, errors.New("reasoner: texts must not be empty")
	}

	// Combine all texts for hashing
	combined := strings.Join(texts, "\n")

	// Compute deterministic hash-based summary
	hash := md5.Sum([]byte(combined))
	hashStr := fmt.Sprintf("%x", hash)[:8]

	// Return consistent structured fact (dummy values for testing)
	return &StructuredFact{
		Entity:   "test_entity",
		Property: "test_property",
		Value:    hashStr,
		Summary:  fmt.Sprintf("Synthesized fact from %d texts: [%s]", len(texts), hashStr),
	}, nil
}
