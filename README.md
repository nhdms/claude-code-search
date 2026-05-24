# claude-search

Search across your entire Claude Code conversation history — keyword (FTS5) + semantic (vector) hybrid search, plus a Next.js dashboard with charts, filters, BYOK config, per-session reindex, and one-click "Resume in Claude Code".

Everything runs locally. Single SQLite file. OpenAI for best-quality embeddings, or any OpenAI-compatible local model (Ollama, llama.cpp server, TEI, vLLM).

## Stack

- **Backend**: Go 1.24, SQLite (FTS5) + [`sqlite-vec`](https://github.com/asg017/sqlite-vec) in the same file
- **Embeddings**: OpenAI `text-embedding-3-small`/`large` or any OpenAI-compatible endpoint (Ollama, etc.)
- **Search**: BM25 (FTS5) + cosine KNN (vec0) → Reciprocal Rank Fusion (k=60)
- **Dashboard**: Next.js 15 (App Router) · shadcn/ui · Tailwind v4 · recharts · SWR · sonner

## Install

**Prebuilt binary** (macOS arm64/amd64, Linux amd64/arm64):

```bash
# Pick the asset for your platform from the latest release:
#   https://github.com/nhdms/claude-code-search/releases/latest
curl -L -o claude-search.tar.gz https://github.com/nhdms/claude-code-search/releases/latest/download/claude-search_${TAG}_darwin-arm64.tar.gz
tar -xzf claude-search.tar.gz
install claude-search ~/.local/bin/
```

Each asset ships with a `.sha256` checksum next to it.

**From source** — see Quick start.

To cut a release, push a tag and GitHub Actions handles the rest:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The workflow builds CGO binaries on native macOS and Linux runners (no cross-compile headaches with sqlite-vec), packages them as `tar.gz` with SHA-256 sums, and attaches everything to the GitHub release.

## Quick start

```bash
# One-shot: build everything and run API + watcher + dashboard.
make up                                 # Ctrl-C to stop
make up-detach                          # run as daemons; logs in ~/.local/share/claude-search/
make stop                               # stop the daemons

# … or build the pieces yourself.
make build                              # → ./bin/claude-search
make install                            # → ~/.local/bin/claude-search (optional)

# 2) Backfill all history (FTS works without an API key)
claude-search import

# 3) Add embeddings for hybrid search
export OPENAI_API_KEY=sk-...            # or set base_url for a local provider
claude-search embed

# 4) Live sync (recommended: Stop hook — see "Live sync" below)
claude-search watch                     # alternative: long-running daemon

# 5) Search
claude-search search "how did I fix the auth bug"
claude-search show <session_id>
claude-search stats

# 6) Dashboard
claude-search serve &                   # 127.0.0.1:7070
cd dashboard && pnpm install --ignore-workspace && pnpm dev    # http://localhost:3737
```

## Dashboard

Three pages, dark-themed shadcn UI:

- **Overview** — stat cards · messages-per-day chart · top projects bar · role pie · top tools bar
- **Sessions & Search** — single page with a shared filter bar (text, project, role, since-date, vector toggle) and two tabs:
  - *Sessions*: paginated list with per-row **Resume** button that copies `cd "<cwd>" && claude --resume <session_id>` to the clipboard
  - *Messages*: hybrid search hits with `fts`/`vec`/`score` badges and project chips at the top (click a chip to filter sessions by that project)
- **Settings** — embedding model preset selector (OpenAI / Ollama / custom), BYOK key field, projects-dir override, **Import-from-path** job with live progress, hook snippet for live sync

Per-session detail view also has a **Reindex** button that drops & rebuilds that session's index entries.

## Embedding models

| Preset | Provider | Model | Dim | Notes |
|---|---|---|---|---|
| OpenAI · 3-small | `https://api.openai.com/v1` | `text-embedding-3-small` | 1536 | default · cheap ($0.02 / 1M tok) · great quality |
| OpenAI · 3-large | `https://api.openai.com/v1` | `text-embedding-3-large` | 3072 | best quality · 6× the cost |
| Ollama · nomic-embed-text | `http://localhost:11434/v1` | `nomic-embed-text` | 768 | free local · `ollama pull nomic-embed-text` |
| Ollama · mxbai-embed-large | `http://localhost:11434/v1` | `mxbai-embed-large` | 1024 | free local |
| Ollama · bge-m3 | `http://localhost:11434/v1` | `bge-m3` | 1024 | free local · multilingual |
| Custom | any | any | any | any OpenAI-compatible `/v1/embeddings` endpoint |

Switch model from `/settings` in the dashboard, or pass flags to the CLI:

```bash
claude-search --embed-model nomic-embed-text --embed-dim 768 embed
# (with the dashboard, set base_url=http://localhost:11434/v1 in Settings)
```

> Dimension is locked in the DB at init. Switching to a model with a different dim drops `vec_chunks` and marks all chunks as pending — the Settings page detects this and asks for confirmation before resetting.

## Live sync

**Recommended — Claude Code Stop hook** (no daemon). One command installs it:

```bash
claude-search hook install                              # → "<bin> import --embed"
claude-search hook install --hook-base-url http://localhost:11434/v1    # local model
claude-search hook install --no-embed                   # FTS-only sync
claude-search hook status
claude-search hook uninstall
```

`install` is idempotent, preserves any other hooks/permissions you already have, and pins the absolute path to the current binary so it survives `$PATH` changes. The default installs `import --embed`, so every new conversation is auto-vectorized — make sure `OPENAI_API_KEY` is exported in your shell rc (or use `--hook-base-url` for a local embedder). If you'd rather edit by hand:

```json
{
  "hooks": {
    "Stop": [{
      "matcher": "",
      "hooks": [{ "type": "command", "command": "claude-search import" }]
    }]
  }
}
```

The shipped `scripts/claude-search-hook.sh` wraps the import call with logging — point the hook at it (`--command /path/to/claude-search-hook.sh`) if you want a paper trail.

**Alternative — fsnotify watcher** (sub-second mid-session indexing, but requires a daemon):

```bash
claude-search watch &
```

## CLI reference

```bash
claude-search import [--project NAME] [--embed-tool-output none|small|all] [--embed]
claude-search embed                       # vectorize pending chunks
claude-search watch                       # fsnotify-based live sync
claude-search search "<query>" [-n 10] [--project P] [--role R] [--since 7d] [--no-vector]
claude-search show <session_id>
claude-search stats
claude-search serve [--addr 127.0.0.1:7070]
claude-search hook install [--command CMD] [--matcher MATCHER]
claude-search hook uninstall
claude-search hook status
```

Global flags:

- `--db PATH` (default `~/.local/share/claude-search/index.db`)
- `--projects-dir PATH` (default `~/.claude/projects`)
- `--embed-model NAME` (default `text-embedding-3-small`)
- `--embed-dim N`
- `--base-url URL` (or `CLAUDE_SEARCH_BASE_URL` env) — OpenAI-compatible endpoint for local models

## HTTP API

```
GET  /api/stats                                 counts, role breakdown, vector_ready, projects_dir
GET  /api/timeseries?bucket=day|hour|month
GET  /api/projects[?q=substring]
GET  /api/tools
GET  /api/sessions?q=&project=&since=&limit=&offset=
GET  /api/sessions/{id}                         session + ordered messages
POST /api/sessions/{id}/reindex                 → Job
GET  /api/search?q=&role=&project=&since=&vector=true|false&limit=
GET  /api/config                                masked key, model, dim, base_url, db_dim
POST /api/config                                {openai_api_key, projects_dir, embed_model, embed_dim, base_url, reset_vectors}
POST /api/import                                {path, tool_mode, embed} → Job
GET  /api/jobs[/{id}]                           background job status
```

Jobs return `{id, status: running|completed|failed, progress, message, stats?, error?}`.

## Schema

Everything in one SQLite file:

```
sessions(id, project_path, cwd, started_at, ended_at, message_count)
messages(uuid, session_id, parent_uuid, role, kind, model, ts, cwd, text, tool_name, tool_input, tool_output)
messages_fts (FTS5, porter unicode61)
chunks(id, message_uuid, session_id, project_path, role, ts, text, embedded)
vec_chunks (sqlite-vec virtual table, float[dim])
sync_state(file_path, last_offset, last_mtime, last_synced_at)
meta(k, v)              -- includes embed_dim lock
```

## Repo layout

```
claude-search/
├── cmd/claude-search/      # CLI entrypoint
├── internal/
│   ├── transcript/         # JSONL parser
│   ├── store/              # SQLite + sqlite-vec
│   ├── ingest/             # parser → chunker → embedder → writer
│   ├── search/             # FTS + vec0 + RRF merge
│   ├── api/                # HTTP server + jobs
│   └── cli/                # cobra commands
├── dashboard/              # Next.js 15 + shadcn
├── scripts/
│   └── claude-search-hook.sh
└── Makefile
```

## Development

```bash
make build       # builds bin/claude-search
make test        # go test
cd dashboard && pnpm dev
```

## License

MIT
