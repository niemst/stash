package reasoner

import (
	"context"
	"testing"
)

func TestFakeReasonStructured(t *testing.T) {
	fake := NewFake("fake", "fake")
	texts := []string{"Alice is an engineer", "Alice works at TechCorp"}

	sf, err := fake.ReasonStructured(context.Background(), texts)
	if err != nil {
		t.Fatalf("ReasonStructured failed: %v", err)
	}

	if sf == nil {
		t.Fatal("ReasonStructured returned nil")
	}

	if sf.Entity != "test_entity" {
		t.Errorf("expected Entity=test_entity, got %q", sf.Entity)
	}

	if sf.Property != "test_property" {
		t.Errorf("expected Property=test_property, got %q", sf.Property)
	}

	if sf.Value == "" {
		t.Error("expected non-empty Value")
	}

	if sf.Summary == "" {
		t.Error("expected non-empty Summary")
	}
}

func TestFakeReasonStructuredDeterminism(t *testing.T) {
	fake := NewFake("fake", "fake")
	texts := []string{"Alice is an engineer", "Alice works at TechCorp"}

	sf1, _ := fake.ReasonStructured(context.Background(), texts)
	sf2, _ := fake.ReasonStructured(context.Background(), texts)

	if sf1.Value != sf2.Value {
		t.Errorf("ReasonStructured not deterministic: %q != %q", sf1.Value, sf2.Value)
	}

	if sf1.Summary != sf2.Summary {
		t.Errorf("ReasonStructured not deterministic: %q != %q", sf1.Summary, sf2.Summary)
	}
}

func TestFakeReasonStructuredEmptyTexts(t *testing.T) {
	fake := NewFake("fake", "fake")

	_, err := fake.ReasonStructured(context.Background(), []string{})
	if err == nil {
		t.Error("expected error for empty texts")
	}
}

func TestFakeReasonStructuredVsFake(t *testing.T) {
	fake := NewFake("fake", "fake")
	texts := []string{"Alice is an engineer"}

	sf, _ := fake.ReasonStructured(context.Background(), texts)
	text, _ := fake.Reason(context.Background(), texts)

	// Summary should match the Reason output
	if sf.Summary != text {
		t.Errorf("Summary mismatch: %q != %q", sf.Summary, text)
	}
}
