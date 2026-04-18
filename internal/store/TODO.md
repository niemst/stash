# TODO.md

Parking lot for "while I'm here" ideas. Do not implement these unless explicitly requested.

## Ideas

### v0.2 possibilities
- Hybrid ranking for vector+text search
- Multiple vector dimensions per named vector
- Vector update without full record rewrite
- Hard delete cascading to vectors
- Connection pool configuration
- Read replicas for scale-out reads
- Composite indexes on common metadata patterns
- Default ordering in List() — currently nondeterministic when Order is nil
- Keyset pagination in iterateWithPagination — re-translates same predicate SQL every iteration

### v0.3+ possibilities
- Alternative backends (SQLite + sqlite-vss, qdrant, chroma)
- Change feed / watch API
- Bulk export/import (JSONL, parquet)
- Tenant/namespace isolation
- Vector quantization (PQ, SQ) for memory efficiency

### Unlikely but noted
- Graph-style edge relationships
- Full-text search across metadata fields
- Custom ranking functions
- Aggregation queries (sum, avg, min, max)
- Cross-record similarity joins
- Vector recalculation triggers