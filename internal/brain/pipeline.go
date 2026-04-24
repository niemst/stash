package brain

import (
	"context"
	"time"
)

// Run starts the background pipeline.
// Consolidates memories periodically.
// Blocks until ctx is cancelled.
func (b *Brain) Run(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	pending := make(map[string]time.Time) // namespace -> first seen
	debounce := time.NewTicker(5 * time.Second)
	defer debounce.Stop()

	for {
		select {
		case ns := <-b.pipelineCh:
			// Track namespace for debouncing
			if _, exists := pending[ns]; !exists {
				pending[ns] = time.Now()
			}
		case <-debounce.C:
			// Process namespaces that have been idle for 10s
			now := time.Now()
			for ns, since := range pending {
				if now.Sub(since) >= 10*time.Second {
					b.consolidate(ctx, ns)
					delete(pending, ns)
				}
			}
		case <-ticker.C:
			// Periodic cleanup
			b.purgeExpired(ctx)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// consolidate processes recent events into facts and extracts relationships.
func (b *Brain) consolidate(ctx context.Context, namespace string) {
	// Check context before starting expensive work
	select {
	case <-ctx.Done():
		return
	default:
	}

	// Query recent events (last 7 days)
	sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour)
	records, err := b.queryRecentEventRecords(ctx, namespace, sevenDaysAgo)
	if err != nil || len(records) == 0 {
		return
	}

	// Cluster by similarity
	clusters := b.clusterRecordsBySimilarity(records, 0.85)

	for _, cluster := range clusters {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return
		default:
		}

		if len(cluster) < 2 {
			continue
		}

		// Extract event contents
		var texts []string
		var eventIDs []string
		for _, r := range cluster {
			texts = append(texts, r.Content)
			eventIDs = append(eventIDs, r.ID)
		}

		// Use LLM to synthesize fact
		structured, err := b.reasoner.ReasonStructured(ctx, texts)
		if err != nil {
			continue
		}

		if structured.Summary == "" {
			continue
		}

		// Store the fact
		factType := FactTypeState
		if structured.Entity != "" && structured.Property != "" {
			factType = FactTypeAtemporal
		}

		err = b.storeFact(ctx, namespace, structured.Summary, factType, len(cluster), "consolidation", eventIDs)
		if err != nil {
			continue
		}
	}

	// Extract relationships from facts
	b.extractRelationships(ctx, namespace)
}

// extractRelationships uses the LLM to find relationships in facts.
func (b *Brain) extractRelationships(ctx context.Context, namespace string) {
	// Check context before starting
	select {
	case <-ctx.Done():
		return
	default:
	}

	facts, err := b.queryFacts(ctx, namespace)
	if err != nil {
		return
	}

	for _, fact := range facts {
		// Check context cancellation in loop
		select {
		case <-ctx.Done():
			return
		default:
		}
		// Skip facts that already have relationships extracted
		if fact.Source == "relationship" {
			continue
		}

		relationships, err := b.reasoner.ReasonRelationships(ctx, fact.Content)
		if err != nil {
			continue
		}

		for _, rel := range relationships {
			b.storeRelationship(ctx, namespace, rel.FromEntity, rel.RelationType, rel.ToEntity, "consolidation", rel.Confidence)
		}
	}
}

// purgeExpired removes expired memories.
func (b *Brain) purgeExpired(ctx context.Context) {
	// Query all events with expires_at < now
	// This is a simplified placeholder - full implementation would query by expiration
	// For now, this is a no-op as we don't set expiration in Remember()
}
