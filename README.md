# Stash

**Your AI has amnesia. We fixed it.**

Every LLM starts every conversation from zero. Stash gives your agent persistent memory — it remembers, recalls, consolidates, and learns across sessions. No more explaining yourself from scratch.

Open source. Self-hosted. Works with any MCP-compatible agent.

## Quick Start

```bash
git clone https://github.com/alash3al/stash.git
cd stash
cp .env.example .env   # edit with your API key + model
docker compose up
```

That's it. Postgres + pgvector, migrations, MCP server with background consolidation — all in one command.

## MCP Client Setup

After `docker compose up`, Stash exposes an MCP server over SSE at:

```
http://localhost:8080/sse
```

Point any MCP-compatible client at that URL. Example configs:

**Cursor** — `~/.cursor/mcp.json`
```json
{
  "mcpServers": {
    "stash": {
      "url": "http://localhost:8080/sse"
    }
  }
}
```

**Claude Desktop** — `claude_desktop_config.json`
```json
{
  "mcpServers": {
    "stash": {
      "url": "http://localhost:8080/sse"
    }
  }
}
```

**OpenCode** — `~/.config/opencode/config.json`
```json
{
  "mcp": {
    "stash": {
      "type": "remote",
      "url": "http://localhost:8080/sse",
      "enabled": true
    }
  }
}
```

**Windsurf** — `~/.codeium/windsurf/mcp_config.json`
```json
{
  "mcpServers": {
    "stash": {
      "url": "http://localhost:8080/sse"
    }
  }
}
```

Works with any agent that supports MCP over SSE — Claude Desktop, Cursor, Windsurf, Cline, Continue, OpenAI Agents, Ollama, OpenRouter, and more.

## What It Does

Stash is a cognitive layer between your AI agent and the world. Episodes become facts. Facts become relationships. Relationships become patterns. Patterns become wisdom.

An 8-stage consolidation pipeline turns raw observations into structured knowledge — facts, relationships, causal links, goal tracking, failure patterns, hypothesis verification, and confidence decay. Each stage only processes new data since the last run.

## Learn More

**[alash3al.github.io/stash →](https://alash3al.github.io/stash/)**

## License

Apache 2.0
