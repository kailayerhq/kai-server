# Kai Server

Server infrastructure for [Kai](https://github.com/kaicontext/kai), an intent-preserving version control system that understands *what your code does*, not just what files changed.

Kai parses your code with tree-sitter, builds semantic dependency graphs, and uses them to determine exactly which tests need to run for any given change. The result: CI that takes minutes instead of hours.

## Architecture

```
┌─────────────────────────────────────┐
│         kai-server                   │
│                                      │
│  ┌──────────────────┐               │
│  │ kailab-control    │  Control plane │
│  │ (Go + SvelteKit)  │  Web UI, API,  │
│  └────────┬──────────┘  auth, CI      │
│           │                          │
│  ┌────────▼──────────┐               │
│  │ kailab             │  Data plane   │
│  │ (Go + Postgres)    │  Graph store, │
│  └───────────────────┘  SSH, objects  │
│                                      │
│  ┌───────────────────┐               │
│  │ kai-core           │  Semantic     │
│  │ (Go + tree-sitter) │  engine       │
│  └───────────────────┘               │
└─────────────────────────────────────┘
```

- **kailab** — Data plane. Stores semantic graphs in Postgres. Handles git push/pull over SSH. Blob storage via local disk or GCS.
- **kailab-control** — Control plane. Web UI (SvelteKit), REST API, authentication (magic links), org/repo management, CI pipeline execution, code reviews.
- **kai-core** — Semantic analysis engine. Tree-sitter parsing, intent classification, dependency graph construction, semantic diffing.
- **kai-playground** — Interactive browser-based tutorial environment.
- **docs-site** — Documentation site (VitePress).

## Quick Start

### Docker Compose (recommended for local dev)

```bash
git clone https://github.com/kaicontext/kai-server.git
cd kai-server
docker compose up
```

This starts PostgreSQL, the data plane, and the control plane. The UI is available at `http://localhost:8080`.

### Build from Source

```bash
# Data plane
cd kailab
CGO_ENABLED=1 go build -o kailabd ./cmd/kailabd

# Control plane
cd ../kailab-control
make build
```

Requires Go 1.25+ and a C compiler (CGO needed for tree-sitter).

### Configure

```bash
# Data plane
export KAILAB_LISTEN=:7447
export KAILAB_DATA=/var/lib/kai/data

# Control plane
export KLC_LISTEN=:8080
export KLC_DB_URL=postgres://user:pass@localhost:5432/kailab?sslmode=disable
export KLC_JWT_SIGNING_KEY=$(openssl rand -hex 32)
export KLC_SHARDS_JSON='{"default":"http://localhost:7447"}'
```

See [docs/self-host-guide.md](docs/self-host-guide.md) for full configuration reference and Kubernetes deployment.

## Project Structure

```
kai-server/
├── kai-core/           # Semantic engine (parsing, diffing, intent)
├── kailab/             # Data plane (graph storage, SSH server)
├── kailab-control/     # Control plane (API, web UI, CI runner)
│   └── frontend/       # SvelteKit web UI
├── kai-playground/     # Interactive tutorial
├── docs-site/          # VitePress documentation
├── docs/               # Internal documentation
├── deploy/             # Kubernetes manifests, Terraform, CI configs
└── docker-compose.yml  # Local development setup
```

## MCP Server

Kai includes an MCP (Model Context Protocol) server with 12 tools for AI coding assistants:

- `kai_callers` / `kai_callees` — Call graph traversal
- `kai_impact` — Transitive downstream impact analysis
- `kai_dependencies` / `kai_dependents` — Dependency graph
- `kai_symbols` — Symbol listing per file
- `kai_tests` — Test coverage mapping
- `kai_diff` — Semantic diff between commits
- `kai_files` — File listing with language/module metadata
- `kai_context` — Aggregated context for a file
- `kai_status` / `kai_refresh` — Graph freshness management

Works with Claude Code, Cursor, and any MCP-compatible client.

## License

Apache License 2.0. See [LICENSE](LICENSE).
