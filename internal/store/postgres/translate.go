package postgres

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/alash3al/stash/internal/store"
)

// translatePredicate translates a store.Predicate to a SQL WHERE clause and args.
// Always includes "deleted_at IS NULL" to enforce soft-delete visibility.
func translatePredicate(p *store.Predicate, startParam int) (where string, params []any, err error) {
	if p == nil {
		return "deleted_at IS NULL", nil, nil
	}

	baseWhere, baseParams, err := translatePredicateRec(p, startParam)
	if err != nil {
		return "", nil, err
	}

	if baseWhere != "" {
		return fmt.Sprintf("(%s) AND deleted_at IS NULL", baseWhere), baseParams, nil
	}
	return "deleted_at IS NULL", nil, nil
}

// translatePredicateWithoutDeleted translates predicate without adding deleted_at IS NULL.
func translatePredicateWithoutDeleted(p *store.Predicate, startParam int) (where string, params []any, err error) {
	if p == nil {
		return "", nil, nil
	}
	return translatePredicateRec(p, startParam)
}

func translatePredicateRec(p *store.Predicate, paramStart int) (where string, params []any, err error) {
	// Handle composite predicates first
	if len(p.And) > 0 {
		return translateComposite(p.And, "AND", paramStart)
	}
	if len(p.Or) > 0 {
		return translateComposite(p.Or, "OR", paramStart)
	}
	if p.Not != nil {
		innerWhere, innerParams, err := translatePredicateRec(p.Not, paramStart)
		if err != nil {
			return "", nil, err
		}
		if innerWhere == "" {
			return "", nil, nil
		}
		return fmt.Sprintf("NOT (%s)", innerWhere), innerParams, nil
	}

	// Leaf predicate
	return translateLeaf(p, paramStart)
}

func translateComposite(preds []store.Predicate, op string, paramStart int) (where string, params []any, err error) {
	var parts []string
	var allParams []any
	currentParam := paramStart

	for i := range preds {
		part, partParams, err := translatePredicateRec(&preds[i], currentParam)
		if err != nil {
			return "", nil, err
		}
		if part == "" {
			continue
		}
		parts = append(parts, part)
		allParams = append(allParams, partParams...)
		currentParam += len(partParams)
	}

	if len(parts) == 0 {
		return "", nil, nil
	}
	if len(parts) == 1 {
		return parts[0], allParams, nil
	}
	return "(" + strings.Join(parts, " "+op+" ") + ")", allParams, nil
}

func translateLeaf(p *store.Predicate, paramNum int) (where string, params []any, err error) {
	// Validate operator
	switch p.Op {
	case store.OpEq, store.OpNe, store.OpGt, store.OpGte, store.OpLt, store.OpLte,
		store.OpIn, store.OpNotIn, store.OpExists, store.OpContains, store.OpPrefix:
		// valid
	default:
		return "", nil, store.ErrInvalidQuery
	}

	// Validate value is not nil for operators that require it
	if p.Value == nil {
		switch p.Op {
		case store.OpExists:
			// OpExists uses bool value, nil is invalid
			return "", nil, store.ErrInvalidQuery
		case store.OpIn, store.OpNotIn:
			// Empty slice is allowed, nil is invalid
			return "", nil, store.ErrInvalidQuery
		}
		// For other operators, nil might be valid (e.g., field = NULL)
		// Let database handle NULL comparison semantics
	}

	// Map field to SQL expression
	expr, err := fieldToExpr(p.Field)
	if err != nil {
		return "", nil, err
	}

	// Determine if this is a metadata field (uses ->> which returns text)
	isMetadata := strings.HasPrefix(p.Field, "metadata.")

	// Generate SQL based on operator
	switch p.Op {
	case store.OpEq:
		if isMetadata {
			return fmt.Sprintf("(%s)::text = $%d::text", expr, paramNum), []any{fmt.Sprintf("%v", p.Value)}, nil
		}
		return fmt.Sprintf("%s = $%d", expr, paramNum), []any{p.Value}, nil
	case store.OpNe:
		if isMetadata {
			return fmt.Sprintf("(%s)::text != $%d::text", expr, paramNum), []any{fmt.Sprintf("%v", p.Value)}, nil
		}
		return fmt.Sprintf("%s != $%d", expr, paramNum), []any{p.Value}, nil
	case store.OpGt:
		if isMetadata {
			return fmt.Sprintf("(%s)::numeric > $%d::numeric", expr, paramNum), []any{p.Value}, nil
		}
		return fmt.Sprintf("%s > $%d", expr, paramNum), []any{p.Value}, nil
	case store.OpGte:
		if isMetadata {
			return fmt.Sprintf("(%s)::numeric >= $%d::numeric", expr, paramNum), []any{p.Value}, nil
		}
		return fmt.Sprintf("%s >= $%d", expr, paramNum), []any{p.Value}, nil
	case store.OpLt:
		if isMetadata {
			return fmt.Sprintf("(%s)::numeric < $%d::numeric", expr, paramNum), []any{p.Value}, nil
		}
		return fmt.Sprintf("%s < $%d", expr, paramNum), []any{p.Value}, nil
	case store.OpLte:
		if isMetadata {
			return fmt.Sprintf("(%s)::numeric <= $%d::numeric", expr, paramNum), []any{p.Value}, nil
		}
		return fmt.Sprintf("%s <= $%d", expr, paramNum), []any{p.Value}, nil
	case store.OpIn:
		slice, err := toAnySlice(p.Value)
		if err != nil {
			return "", nil, store.ErrInvalidQuery
		}
		if len(slice) == 0 {
			return "FALSE", nil, nil
		}
		placeholders := make([]string, len(slice))
		for i := range slice {
			placeholders[i] = fmt.Sprintf("$%d", paramNum+i)
		}
		if isMetadata {
			params := make([]any, len(slice))
			for i, v := range slice {
				params[i] = fmt.Sprintf("%v", v)
			}
			return fmt.Sprintf("(%s)::text IN (%s)", expr, strings.Join(placeholders, ", ")), params, nil
		}
		return fmt.Sprintf("%s IN (%s)", expr, strings.Join(placeholders, ", ")), slice, nil
	case store.OpNotIn:
		slice, err := toAnySlice(p.Value)
		if err != nil {
			return "", nil, store.ErrInvalidQuery
		}
		if len(slice) == 0 {
			return "TRUE", nil, nil // NOT IN empty set is always true
		}
		placeholders := make([]string, len(slice))
		for i := range slice {
			placeholders[i] = fmt.Sprintf("$%d", paramNum+i)
		}
		if isMetadata {
			params := make([]any, len(slice))
			for i, v := range slice {
				params[i] = fmt.Sprintf("%v", v)
			}
			return fmt.Sprintf("(%s)::text NOT IN (%s)", expr, strings.Join(placeholders, ", ")), params, nil
		}
		return fmt.Sprintf("%s NOT IN (%s)", expr, strings.Join(placeholders, ", ")), slice, nil
	case store.OpExists:
		exists, ok := p.Value.(bool)
		if !ok {
			return "", nil, store.ErrInvalidQuery
		}
		if exists {
			return fmt.Sprintf("%s IS NOT NULL", expr), nil, nil
		}
		return fmt.Sprintf("%s IS NULL", expr), nil, nil
	case store.OpContains:
		return fmt.Sprintf("$%d = ANY(%s)", paramNum, expr), []any{p.Value}, nil
	case store.OpPrefix:
		// Escape LIKE metacharacters to prevent wildcard matching
		value, ok := p.Value.(string)
		if !ok {
			return "", nil, store.ErrInvalidQuery
		}
		escaped := strings.ReplaceAll(strings.ReplaceAll(value, "\\", "\\\\"), "%", "\\%")
		escaped = strings.ReplaceAll(escaped, "_", "\\_")
		return fmt.Sprintf("%s LIKE ($%d || '%%') ESCAPE '\\'", expr, paramNum), []any{escaped}, nil
	}

	return "", nil, store.ErrInvalidQuery
}

// toAnySlice converts any slice to []any.
func toAnySlice(v any) ([]any, error) {
	if v == nil {
		return nil, nil
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil, fmt.Errorf("not a slice")
	}

	result := make([]any, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		result[i] = rv.Index(i).Interface()
	}
	return result, nil
}

// fieldToExpr converts a dotted field path to a SQL expression.
func fieldToExpr(field string) (string, error) {
	switch field {
	case "id":
		return "id", nil
	case "content":
		return "content", nil
	case "created_at":
		return "created_at", nil
	case "updated_at":
		return "updated_at", nil
	case "deleted_at":
		return "deleted_at", nil
	default:
		// Assume metadata path
		if strings.HasPrefix(field, "metadata.") {
			path := strings.TrimPrefix(field, "metadata.")
			parts := strings.Split(path, ".")
			if len(parts) == 0 {
				return "", store.ErrInvalidQuery
			}
			// Build JSONB traversal: metadata->'a'->'b'->>'c'
			expr := "metadata"
			for i := 0; i < len(parts)-1; i++ {
				expr += fmt.Sprintf("->'%s'", parts[i])
			}
			expr += fmt.Sprintf("->>'%s'", parts[len(parts)-1])
			return expr, nil
		}
		return "", store.ErrInvalidQuery
	}
}

// translateOrder translates store.Order to SQL ORDER BY.
func translateOrder(orders []store.Order) (string, error) {
	if len(orders) == 0 {
		return "", nil
	}

	var clauses []string
	for _, o := range orders {
		expr, err := fieldToExpr(o.Field)
		if err != nil {
			return "", err
		}
		if o.Desc {
			clauses = append(clauses, expr+" DESC")
		} else {
			clauses = append(clauses, expr+" ASC")
		}
	}
	return "ORDER BY " + strings.Join(clauses, ", "), nil
}

// buildLimitOffset builds SQL LIMIT/OFFSET clause with safe bounds.
func (s *Store) buildLimitOffset(limit, offset int) string {
	if limit <= 0 {
		limit = s.maxResultSize()
	} else if limit > s.maxResultSize() {
		limit = s.maxResultSize()
	}

	if offset < 0 {
		offset = 0
	}

	if offset == 0 {
		return fmt.Sprintf("LIMIT %d", limit)
	}
	return fmt.Sprintf("LIMIT %d OFFSET %d", limit, offset)
}

// validateQuery validates a store.Query for search.
func (s *Store) validateQuery(q store.Query) error {
	if q.TopK <= 0 && (q.Vector != nil || q.Text != "") {
		return store.ErrInvalidQuery
	}
	if q.TopK > 0 && q.Vector == nil && q.Text == "" {
		return store.ErrInvalidQuery
	}
	if q.Vector != nil && len(q.Vector) != s.cfg.VectorDim {
		return fmt.Errorf("%w: query vector dimension %d != store dimension %d",
			store.ErrInvalidVector, len(q.Vector), s.cfg.VectorDim)
	}
	if q.Vector != nil && q.VectorName == "" {
		return store.ErrInvalidQuery
	}
	// VectorName is only meaningful with Vector for vector search
	if q.Text != "" && q.VectorName != "" && q.Vector == nil {
		return store.ErrInvalidQuery
	}
	return nil
}

// queryToSQL builds SQL for a search query.
func (s *Store) queryToSQL(q store.Query, paramStart int) (string, []any, error) {
	if err := s.validateQuery(q); err != nil {
		return "", nil, err
	}

	var whereParts []string
	var params []any
	currentParam := paramStart

	// Add filter predicate if present
	if q.Filter != nil {
		filterWhere, filterParams, err := translatePredicate(q.Filter, currentParam)
		if err != nil {
			return "", nil, err
		}
		if filterWhere != "" {
			whereParts = append(whereParts, filterWhere)
			params = append(params, filterParams...)
			currentParam += len(filterParams)
		}
	} else {
		whereParts = append(whereParts, "deleted_at IS NULL")
	}

	// Add text search if present
	if q.Text != "" {
		whereParts = append(whereParts, fmt.Sprintf("to_tsvector('english', content) @@ plainto_tsquery('english', $%d)", currentParam))
		params = append(params, q.Text)
		currentParam++
	}

	// Build WHERE clause
	whereClause := ""
	if len(whereParts) > 0 {
		whereClause = "WHERE " + strings.Join(whereParts, " AND ")
	}

	// Build ORDER BY
	orderBy := ""
	if q.Vector != nil {
		// Vector search: order by distance
		orderBy = fmt.Sprintf("ORDER BY v.vector <=> $%d", currentParam)
		params = append(params, q.Vector)
		currentParam++
	} else if q.Text != "" {
		// Text search: order by rank
		orderBy = fmt.Sprintf("ORDER BY ts_rank(to_tsvector('english', content), plainto_tsquery('english', $%d)) DESC", currentParam-1)
		// currentParam-1 because q.Text was already added as parameter
	}

	return fmt.Sprintf(`
		SELECT r.id, r.content, r.metadata, r.created_at, r.updated_at
		FROM records r
		%s
		%s
		%s
	`, whereClause, orderBy, s.buildLimitOffset(q.TopK, 0)), params, nil
}
