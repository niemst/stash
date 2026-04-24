package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alash3al/stash/internal/store"
)

// parseFilterDSL parses a simple filter DSL and returns a store.Predicate.
// Format: "field=value,field>=value,field<value"
// Supported operators: =, !=, <, >, <=, >=
// Multiple filters are AND-ed together.
func parseFilterDSL(dsl string) (*store.Predicate, error) {
	if dsl == "" {
		return nil, nil
	}

	parts := strings.Split(dsl, ",")
	if len(parts) == 0 {
		return nil, nil
	}

	predicates := make([]store.Predicate, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		pred, err := parseFilterExpression(part)
		if err != nil {
			return nil, err
		}
		predicates = append(predicates, pred)
	}

	if len(predicates) == 0 {
		return nil, nil
	}

	if len(predicates) == 1 {
		return &predicates[0], nil
	}

	return &store.Predicate{And: predicates}, nil
}

func parseFilterExpression(expr string) (store.Predicate, error) {
	operators := []struct {
		op   string
		storeOp store.Op
	}{
		{"<=", store.OpLte},
		{">=", store.OpGte},
		{"!=", store.OpNe},
		{"<", store.OpLt},
		{">", store.OpGt},
		{"=", store.OpEq},
	}

	var field, value string
	var op store.Op

	found := false
	for _, o := range operators {
		idx := strings.Index(expr, o.op)
		if idx > 0 {
			field = strings.TrimSpace(expr[:idx])
			value = strings.TrimSpace(expr[idx+len(o.op):])
			op = o.storeOp
			found = true
			break
		}
	}

	if !found {
		return store.Predicate{}, fmt.Errorf("invalid filter expression %q: must contain operator (=, !=, <, >, <=, >=)", expr)
	}

	if field == "" {
		return store.Predicate{}, fmt.Errorf("invalid filter expression %q: missing field name", expr)
	}

	if value == "" {
		return store.Predicate{}, fmt.Errorf("invalid filter expression %q: missing value", expr)
	}

	metadataField := "metadata." + field

	var finalValue any = value
	if op == store.OpGt || op == store.OpGte || op == store.OpLt || op == store.OpLte {
		if num, err := strconv.ParseFloat(value, 64); err == nil {
			finalValue = num
		}
	}

	return store.Predicate{
		Field: metadataField,
		Op:    op,
		Value: finalValue,
	}, nil
}
