# Testing Documentation

**Testing Philosophy:** User-level integration tests only. No unit tests. No fake implementations.

---

## Test Strategy

### Why Integration Tests Only?

**Deleted ~7000 lines** of unit tests, fake implementations, and test-only code. Here's why:

1. **Tests what ships** — Real PostgreSQL, real OpenAI, real CLI
2. **Catches real issues** — Config errors, env vars, API failures, schema problems
3. **No maintenance burden** — No fakes to keep in sync with real implementations
4. **Simpler codebase** — One test type, one workflow, clear expectations
5. **Honest about requirements** — Product needs PostgreSQL + OpenAI. Tests do too.

### Trade-offs Accepted

- **Slower feedback** — Integration tests take 30-60s vs 2s for unit tests
- **External dependencies** — Need PostgreSQL + OpenAI API key to run tests
- **API costs** — Each test run calls OpenAI (but uses cheap models: gpt-4o-mini, text-embedding-3-small)
- **No offline development** — Can't test without network access

These trade-offs are acceptable because the product **requires** these dependencies in production.

---

## Running Tests

### Prerequisites

1. **Start PostgreSQL:**
```bash
docker-compose up -d postgres
```

2. **Set environment variables:**
```bash
export STASH_POSTGRES_DSN=postgres://stash:stash_dev_password@localhost:5432/stash?sslmode=disable
export STASH_VECTOR_DIM=1536
export STASH_MAX_RESULT_SIZE=10000
export STASH_OPENAI_API_KEY=sk-...
export STASH_OPENAI_BASE_URL=https://api.openai.com/v1
export STASH_EMBEDDING_MODEL=text-embedding-3-small
export STASH_REASONER_MODEL=gpt-4o-mini
export STASH_CONTEXT_TTL=1h
export STASH_HTTP_ADDR=:8080
export STASH_LOG_LEVEL=info
export STASH_LOG_FORMAT=json
```

3. **Build the binary:**
```bash
go build -o /tmp/stash ./cmd/cli/
```

### Run All Tests

```bash
for script in test-phase3-*.sh; do
  bash "$script" || exit 1
done
```

### Run Individual Tests

```bash
# Temporal fact types
bash test-phase3-task0014.sh

# Entity relationships
bash test-phase3-task0015.sh

# Semantic consolidation
bash test-phase3-task0016.sh

# Confidence-ranked retrieval
bash test-phase3-task0017.sh
```

---

## Test Coverage

| Test Script | Feature Tested | Verifies |
|-------------|----------------|----------|
| `test-phase3-task0014.sh` | Temporal fact types | Facts classified as atemporal/state/point-in-time |
| `test-phase3-task0015.sh` | Entity relationships | Store, query, traverse knowledge graph |
| `test-phase3-task0016.sh` | Semantic consolidation | LLM extracts relationships from facts |
| `test-phase3-task0017.sh` | Confidence-ranked retrieval | Recall combines relevance + confidence |

---

## CI/CD Testing

GitHub Actions runs all integration tests on every push/PR:

- Spins up PostgreSQL with pgvector
- Sets up environment variables (OpenAI key from secrets)
- Builds the binary
- Runs all `test-phase3-*.sh` scripts
- Fails CI if any test fails

See `.github/workflows/test.yml` for details.

---

## What Each Test Does

### Task 0014: Temporal Fact Types

**Workflow:**
1. Add facts with different temporal types
2. Query by type (atemporal, state, point-in-time)
3. Verify correct facts returned

**Example commands tested:**
```bash
stash facts add "The sky is blue" --type atemporal
stash facts query --type atemporal
```

### Task 0015: Entity Relationships

**Workflow:**
1. Store relationships between entities
2. Query outgoing/incoming relationships
3. Traverse graph with depth limit
4. Visualize graph structure

**Example commands tested:**
```bash
stash facts add "Alice works at TechCorp"
stash facts relationships --entity Alice
stash facts graph --entity Alice --depth 2
```

### Task 0016: Semantic Consolidation

**Workflow:**
1. Add multiple facts
2. Extract relationships using LLM
3. Verify relationships stored correctly

**Example commands tested:**
```bash
stash facts add "Bob manages Alice"
stash facts extract-relationships --limit 10
```

### Task 0017: Confidence-Ranked Retrieval

**Workflow:**
1. Add facts with varying confidence scores
2. Recall with `--ranked` flag
3. Verify results sorted by combined score (relevance + confidence)

**Example commands tested:**
```bash
stash facts add "Alice works at TechCorp" --confidence 0.95
stash facts recall "employment" --ranked
```

---

## Debugging Failed Tests

### Test fails with "connection refused"
PostgreSQL isn't running. Start it:
```bash
docker-compose up -d postgres
sleep 5  # Wait for startup
```

### Test fails with "401 Unauthorized"
OpenAI API key is invalid or not set:
```bash
echo $STASH_OPENAI_API_KEY  # Should show sk-...
```

### Test fails with "schema not found"
Database schema not initialized. The CLI auto-migrates on first use, but if you're using a fresh database:
```bash
/tmp/stash events create "initialize schema"
```

### Test times out or hangs
- Check if OpenAI API is reachable: `curl https://api.openai.com/v1`
- Check PostgreSQL logs: `docker-compose logs postgres`
- Increase timeout in test script if using slow models

---

## Adding New Tests

### 1. Create test script

```bash
#!/bin/bash
# test-phase3-new-feature.sh

set -e
set -x

STASH="/tmp/stash"

# Test setup
$STASH events create "test fact"

# Run test
output=$($STASH events recall "test")

# Verify
echo "$output" | grep -q "test fact" || {
  echo "FAIL: Expected 'test fact' in output"
  exit 1
}

echo "✓ Test passed"
```

### 2. Make it executable

```bash
chmod +x test-phase3-new-feature.sh
```

### 3. Run it

```bash
bash test-phase3-new-feature.sh
```

### 4. Add to CI

The wildcard `test-phase3-*.sh` in `.github/workflows/test.yml` automatically picks up new tests.

---

## Cost Considerations

Each test run costs approximately:
- **Embeddings:** ~$0.0001 per fact (text-embedding-3-small)
- **LLM calls:** ~$0.0002 per consolidation (gpt-4o-mini)
- **Total per run:** < $0.01

CI runs on every push, so monthly cost for active development: **~$5-10/month**.

Use cheaper models or mock API server if cost becomes an issue.

---

## Future Test Improvements

Potential additions (not implemented yet):

- **Load testing** — `test-load-*.sh` scripts with large datasets
- **Error injection** — Test handling of API failures, network issues
- **Multi-namespace tests** — Verify namespace isolation
- **Batch operations** — Test bulk import/export
- **Performance benchmarks** — Track query latency over time

Add these only when needed. Resist the urge to over-test.
