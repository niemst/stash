# Stash HTTP API Server

The Stash semantic memory system is now available as an HTTP API via the `stash server` CLI command.

**API Philosophy:** The HTTP API exposes only the core agent operations (Remember and Recall). Administrative tasks like relationship consolidation are handled via CLI commands, which can be scheduled or run manually.

## Starting the Server

```bash
stash server --port 8080 --host 0.0.0.0
```

Default: `http://localhost:8080`

## API Endpoints

### Health Check
```
GET /health
```
Returns `{"status": "ok"}` if the server is running.

### 1. Remember Events
```
POST /api/v1/facts?namespace=<namespace>
Content-Type: application/json

{
  "content": "Alice works at TechCorp",
  "confidence": 0.95,
  "metadata": {
    "source": "chat",
    "user_id": "123"
  }
}
```

**Response (201):**
```json
{
  "id": "574e3c67-0ae2-4cfd-8b31-d0cd688d3bd8",
  "message": "Event remembered successfully"
}
```

### 2. Recall Facts
```
GET /api/v1/facts?query=<query>&namespace=<namespace>&limit=10&ranked=true

Query parameters:
- query (required): Search query
- namespace (optional): Filter by namespace
- limit (optional, default: 10): Max results
- ranked (optional, default: false): Use confidence-ranked retrieval
```

**Response (200):**
```json
{
  "query": "TechCorp",
  "namespace": "default",
  "ranked": true,
  "limit": 10,
  "facts": [
    {
      "id": "fact-uuid",
      "content": "Alice works at TechCorp",
      "type": "state",
      "confidence": 0.95,
      "observation_count": 1,
      "source": "event",
      "score": 0.89,
      "valid_from": "2026-04-24T02:03:22Z"
    }
  ]
}
```

## Administrative Operations (CLI Only)

For batch processing, consolidation, and maintenance tasks, use the CLI:

```bash
# Extract and consolidate relationships from recent facts
stash facts extract-relationships --namespace default --limit 100

# Consolidate recent events into facts
stash facts consolidate --namespace default --window 1h

# Find contradictions
stash facts contradictions --namespace default

# Reflect on memory state
stash facts reflect --namespace default
```

These operations are intentionally CLI-only because:
- They're background/scheduled tasks, not per-request operations
- They may take significant time (LLM calls, batch processing)
- They're administrative tools, not runtime agent needs

## Configuration

The server requires these environment variables:

```bash
# Storage (PostgreSQL required)
STASH_STORE_DRIVER=postgres
STASH_STORE_DSN=postgres://user:pass@localhost/stash
STASH_VECTOR_DIM=1536
STASH_MAX_RESULT_SIZE=10000

# Embeddings (OpenAI required)
STASH_EMBEDDER_DRIVER=openai
STASH_OPENAI_API_KEY=sk-...
STASH_OPENAI_BASE_URL=https://api.openai.com/v1
STASH_EMBEDDING_MODEL=text-embedding-3-small

# Reasoning (OpenAI required)
STASH_REASONER_DRIVER=openai
STASH_REASONER_MODEL=gpt-4o-mini

# Memory
STASH_CONTEXT_TTL=1h

# Server
STASH_HTTP_ADDR=:8080
STASH_LOG_LEVEL=info
STASH_LOG_FORMAT=json
```

## Docker

Run the server in Docker with PostgreSQL:

```bash
# Start PostgreSQL
docker-compose up -d postgres

# Run stash server
docker run -p 8080:8080 \
  --network stash-network \
  -e STASH_STORE_DRIVER=postgres \
  -e STASH_STORE_DSN=postgres://stash:stash_dev_password@postgres/stash \
  -e STASH_VECTOR_DIM=1536 \
  -e STASH_MAX_RESULT_SIZE=10000 \
  -e STASH_EMBEDDER_DRIVER=openai \
  -e STASH_OPENAI_API_KEY=sk-... \
  -e STASH_OPENAI_BASE_URL=https://api.openai.com/v1 \
  -e STASH_EMBEDDING_MODEL=text-embedding-3-small \
  -e STASH_REASONER_DRIVER=openai \
  -e STASH_REASONER_MODEL=gpt-4o-mini \
  -e STASH_CONTEXT_TTL=1h \
  -e STASH_HTTP_ADDR=:8080 \
  -e STASH_LOG_LEVEL=info \
  -e STASH_LOG_FORMAT=json \
  stash server --host 0.0.0.0
```

## Error Handling

All errors return JSON with an `error` field:

```json
{
  "error": "query is required"
}
```

HTTP status codes:
- `200 OK` — Success
- `201 Created` — Resource created
- `400 Bad Request` — Invalid input
- `500 Internal Server Error` — Server error

## Integration for Agents

### Python Agent Example

```python
import requests
import json

BASE_URL = "http://localhost:8080"

# Remember a fact
response = requests.post(
    f"{BASE_URL}/api/v1/facts",
    json={
        "content": "The user's name is Alice",
        "confidence": 0.95
    },
    params={"namespace": "user-context"}
)
fact_id = response.json()["id"]

# Recall facts
response = requests.get(
    f"{BASE_URL}/api/v1/facts",
    params={
        "query": "user name",
        "namespace": "user-context",
        "ranked": True
    }
)
facts = response.json()["facts"]


```

### JavaScript/Node.js Example

```javascript
const axios = require('axios');

const BASE_URL = "http://localhost:8080";

async function rememberFact(content, confidence = 0.9) {
  const response = await axios.post(
    `${BASE_URL}/api/v1/facts?namespace=default`,
    { content, confidence }
  );
  return response.data.id;
}

async function recallFacts(query, ranked = true) {
  const response = await axios.get(`${BASE_URL}/api/v1/facts`, {
    params: { query, ranked, limit: 10 }
  });
  return response.data.facts;
}


```

## Performance Notes

- Embeddings are generated asynchronously during `Remember` operations (~200-800ms per fact)
- LLM-based relationship extraction is called per-fact for the `extract` endpoint
- Confidence-ranked retrieval combines semantic similarity and confidence scores
- All operations support optional namespacing for multi-tenant use
