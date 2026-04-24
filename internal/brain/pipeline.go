package brain

import (
	"context"
	"log"
	"time"
)

const (
	pipelineDebounceInterval = 5 * time.Second
	pipelineDebounceWindow   = 10 * time.Second
	pipelineCleanupInterval  = 1 * time.Minute
	consolidationWindow      = 7 * 24 * time.Hour
	similarityThreshold      = 0.85
)

// Run starts the background pipeline.
// Consolidates memories periodically.
// Blocks until ctx is cancelled.
func (b *Brain) Run(ctx context.Context) error {
	ticker := time.NewTicker(pipelineCleanupInterval)
	defer ticker.Stop()

	pending := make(map[string]time.Time) // namespace -> first seen
	debounce := time.NewTicker(pipelineDebounceInterval)
	defer debounce.Stop()

	for {
		select {
		case ns := <-b.pipelineCh:
			// Track namespace for debouncing
			if _, exists := pending[ns]; !exists {
				pending[ns] = time.Now()
			}
		case <-debounce.C:
			// Collect namespaces to process (avoid map modification during iteration)
			now := time.Now()
			var toProcess []string
			for ns, since := range pending {
				if now.Sub(since) >= pipelineDebounceWindow {
					toProcess = append(toProcess, ns)
				}
			}
			// Process and remove
			for _, ns := range toProcess {
				b.consolidate(ctx, ns)
				delete(pending, ns)
			}
		case <-ticker.C:
			// Periodic cleanup (currently no-op)
			b.purgeExpired(ctx)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// consolidate processes recent events into facts and extracts relationships.
func (b *Brain) consolidate(ctx context.Context, namespace string) {
	// Check context before starting expensive work
	if checkContext(ctx) {
		return
	}

	// Query recent events
	since := time.Now().Add(-consolidationWindow)
	records, err := b.queryRecentEventRecords(ctx, namespace, since)
	if err != nil {
		log.Printf("brain: consolidate queryRecentEventRecords failed for namespace=%q: %v", namespace, err)
		return
	}
	if len(records) == 0 {
		return
	}

	// Cluster by similarity
	clusters := b.clusterRecordsBySimilarity(records, similarityThreshold)

	for _, cluster := range clusters {
		// Check context cancellation
		if checkContext(ctx) {
			return
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
			log.Printf("brain: consolidate ReasonStructured failed: %v", err)
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
			log.Printf("brain: consolidate storeFact failed: %v", err)
			continue
		}
	}

	// Extract relationships from facts
	b.extractRelationships(ctx, namespace)
}

// extractRelationships uses the LLM to find relationships in facts.
func (b *Brain) extractRelationships(ctx context.Context, namespace string) {
	// Check context before starting
	if checkContext(ctx) {
		return
	}

	facts, err := b.queryFacts(ctx, namespace)
	if err != nil {
		log.Printf("brain: extractRelationships queryFacts failed for namespace=%q: %v", namespace, err)
		return
	}

	for _, fact := range facts {
		// Check context cancellation in loop
		if checkContext(ctx) {
			return
		}

		// Skip facts that already have relationships extracted
		if fact.Source == "relationship" {
			continue
		}

		relationships, err := b.reasoner.ReasonRelationships(ctx, fact.Content)
		if err != nil {
			log.Printf("brain: extractRelationships ReasonRelationships failed for fact %s: %v", fact.ID, err)
			continue
		}

		for _, rel := range relationships {
			err := b.storeRelationship(ctx, namespace, rel.FromEntity, rel.RelationType, rel.ToEntity, "consolidation", rel.Confidence)
			if err != nil {
				log.Printf("brain: storeRelationship failed: %v", err)
			}
		}
	}
}

// purgeExpired removes expired memories.
// Currently a no-op as Remember() does not set TTL.
func (b *Brain) purgeExpired(ctx context.Context) {
	// No-op: TTL support not implemented yet
}

// checkContext returns true if context is cancelled.
func checkContext(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
