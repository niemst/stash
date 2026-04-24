# Agent Integration Checklist

Stash is ready for agent integration via HTTP API.

## ✅ What's Ready

### Core API
- ✅ `POST /api/v1/facts` — Remember events (with confidence, metadata, namespacing)
- ✅ `GET /api/v1/facts?query=...&ranked=true` — Recall facts (semantic + confidence-ranked)
- ✅ `GET /health` — Health check
- ✅ JSON request/response format
- ✅ Error handling with proper HTTP status codes

### Deployment
- ✅ Single static binary (`stash`)
- ✅ Docker image (~15MB distroless)
- ✅ Docker Compose for local dev (PostgreSQL + pgAdmin)
- ✅ Environment variable configuration
- ✅ Multi-platform builds (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64)

### Storage Options
- ✅ PostgreSQL (production, with pgvector for embeddings)
- ✅ In-memory (development/testing)

### AI Backends
- ✅ OpenAI (embeddings + reasoning)
- ✅ Fake (testing without API keys)

### Documentation
- ✅ API reference with examples (`API-SERVER.md`)
- ✅ CLI reference (`CLI-REFERENCE.md`)
- ✅ Deployment guide (`DEPLOYMENT.md`)
- ✅ Testing strategy (`TESTING.md`)

### Testing
- ✅ Unit tests (140+ tests, 80%+ coverage)
- ✅ Integration test scripts
- ✅ Verified endpoints work end-to-end

## 🚀 Quick Start for Agents

### 1. Start the Server

**Option A: Local (in-memory)**
```bash
stash server --port 8080
```

**Option B: Docker with PostgreSQL**
```bash
docker-compose up -d postgres
docker run -p 8080:8080 \
  -e STASH_STORE_TYPE=postgres \
  -e STASH_POSTGRES_URL=postgres://stash:stash_dev_password@postgres/stash \
  -e STASH_OPENAI_API_KEY=sk-... \
  stash server
```

### 2. Integrate with Your Agent

**Python Example:**
```python
import requests

BASE = "http://localhost:8080"

# Remember a fact
requests.post(f"{BASE}/api/v1/facts", json={
    "content": "User prefers dark mode",
    "confidence": 0.95
})

# Recall relevant facts
response = requests.get(f"{BASE}/api/v1/facts", params={
    "query": "user preferences",
    "ranked": True,
    "limit": 10
})
facts = response.json()["facts"]
```

**JavaScript Example:**
```javascript
const axios = require('axios');
const BASE = "http://localhost:8080";

// Remember a fact
await axios.post(`${BASE}/api/v1/facts`, {
  content: "User's favorite color is blue",
  confidence: 0.9
});

// Recall facts
const { data } = await axios.get(`${BASE}/api/v1/facts`, {
  params: { query: "user color", ranked: true }
});
console.log(data.facts);
```

### 3. Background Maintenance (Optional)

Schedule these CLI commands via cron or systemd timers:

```bash
# Extract relationships from recent facts (daily)
0 2 * * * stash facts extract-relationships --limit 1000

# Consolidate events into facts (hourly)
0 * * * * stash facts consolidate --window 1h

# Reflect on memory state (weekly)
0 3 * * 0 stash facts reflect
```

## 📊 Architecture

```
┌─────────────┐
│   Agent     │
│  (Python,   │
│   Node.js,  │
│   Go, etc)  │
└──────┬──────┘
       │ HTTP
       ▼
┌─────────────────┐
│  Stash Server   │
│  (Port 8080)    │
│                 │
│  POST /facts    │◄── Remember
│  GET /facts     │◄── Recall
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   PostgreSQL    │
│   + pgvector    │
└─────────────────┘
```

## 🔧 Configuration

Set via environment variables:

```bash
# Storage
STASH_STORE_TYPE=postgres  # or 'memory'
STASH_POSTGRES_URL=postgres://user:pass@host/db

# Embeddings
STASH_EMBEDDER_TYPE=openai  # or 'fake'
STASH_OPENAI_API_KEY=sk-...
STASH_OPENAI_EMBEDDING_MODEL=text-embedding-3-small

# Reasoning (for CLI consolidation)
STASH_REASONER_TYPE=openai  # or 'fake'
STASH_OPENAI_MODEL=gpt-4o-mini
```

## 🎯 Agent Integration Patterns

### Pattern 1: Stateless Agent (Recommended)
Agent keeps no state. Every request:
1. Recall relevant context from Stash
2. Process user input with context
3. Remember new facts to Stash

### Pattern 2: Hybrid Agent
Agent maintains short-term session state, but persists:
- Important facts discovered during conversation
- User preferences/corrections
- Long-term knowledge

### Pattern 3: Multi-Agent System
Multiple agents share knowledge via Stash:
- Use `namespace` to separate agent contexts
- Central Stash server serves all agents
- Background consolidation finds cross-agent patterns

## 📝 Best Practices

### Confidence Scores
- **0.9-1.0**: User-stated facts ("My name is Alice")
- **0.7-0.9**: Inferred from clear context ("User likely prefers email")
- **0.5-0.7**: Tentative inference ("User might be interested in X")
- **Below 0.5**: Speculation (avoid storing)

### Namespaces
- Use per-user namespaces: `user:{user_id}`
- Or per-agent: `agent:customer-support`
- Or per-session: `session:{session_id}`
- Default namespace: `""` (global)

### Metadata
Store searchable attributes:
```json
{
  "content": "User prefers email notifications",
  "confidence": 0.95,
  "metadata": {
    "source": "settings_page",
    "timestamp": "2026-04-24T02:00:00Z",
    "user_id": "123"
  }
}
```

### Ranked Retrieval
Always use `ranked=true` for user-facing facts:
- Combines semantic relevance + confidence
- Filters out low-confidence noise
- Returns most reliable results first

## 🔍 Troubleshooting

### Server won't start
```bash
# Check logs
stash server 2>&1 | tee server.log

# Test with minimal config (in-memory)
STASH_STORE_TYPE=memory \
STASH_EMBEDDER_TYPE=fake \
STASH_REASONER_TYPE=fake \
stash server
```

### No results from recall
- Ensure facts were remembered successfully (check response)
- Try broader query terms
- Lower the relevance threshold by requesting more results (`limit=50`)
- Check namespace matches

### Slow responses
- First embedding call is slow (~500ms), subsequent calls are cached
- Use PostgreSQL instead of in-memory for production
- Consider connection pooling if high concurrency

## 🎉 You're Ready!

The product is fully functional and ready for agent integration. Start with the Quick Start examples above and see `API-SERVER.md` for complete API reference.
