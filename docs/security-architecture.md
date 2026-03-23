# Security Architecture

How Kai is designed to protect customer data and maintain trust.

---

## Principles

1. **Minimal data collection** — Kai reads code structure, not code content
2. **Local-first processing** — AST parsing happens on the developer's machine
3. **Tenant isolation** — each repository has its own database, no cross-tenant access
4. **No secrets access** — Kai never reads, stores, or transmits secrets
5. **No code execution** — Kai analyzes structure, it does not run customer code

---

## Authentication

### Magic Link (Web)

1. User submits email
2. Server sends a one-time magic link token
3. User clicks link or enters token
4. Server issues JWT access token (short-lived) + refresh token (HTTP-only cookie)
5. Access token expires after 15 minutes, refresh token after 7 days

### Personal Access Tokens (CLI)

- Long-lived tokens for CLI and API access
- Scoped to user identity
- Revocable at any time via web console or API
- Stored locally in `~/.kai/credentials.json`

### JWT Implementation

- Signed with HMAC-SHA256 (configurable key)
- Short-lived access tokens (15 min default)
- Refresh tokens stored server-side with revocation support
- Tokens do not contain sensitive data beyond user ID and email

---

## Authorization

### Role-Based Access Control (RBAC)

| Role | Permissions |
|------|-------------|
| Owner | Full control, can delete org, manage billing |
| Admin | Manage members, repos, settings |
| Maintainer | Push/fetch, manage repos |
| Developer | Push/fetch to assigned repos |
| Reporter | Read-only access |
| Guest | Limited read access |

Roles are per-organization. A user can have different roles in different orgs.

---

## Tenant Isolation

### Data Isolation

- Each repository is isolated within Postgres using a repo UUID
- All graph data (objects, refs, edges) is scoped by repo ID
- No cross-tenant data access
- API routes are scoped: `/{tenant}/{repo}/v1/*`

### Request Isolation

- Every API request is authenticated and scoped to a tenant/repo
- Cross-tenant requests are rejected at the routing layer
- No global queries span multiple tenants

---

## Network Security

### TLS

- All external communication over HTTPS (TLS 1.2+)
- Internal cluster communication over HTTPS
- No plaintext HTTP in production

### API Security

- Rate limiting on authentication endpoints
- Request size limits on push payloads (256MB default)
- CORS restricted to known origins

---

## Infrastructure

### Deployment

- Runs on Google Kubernetes Engine (GKE)
- Container images built from minimal base images
- No SSH access to production containers
- Secrets managed via Kubernetes Secrets (encrypted at rest)

### Storage

- Postgres database with SSL/TLS connections (encrypted at rest and in transit)
- GCS blob storage for large objects (optional)
- Backups via Postgres pg_dump and volume snapshots

### Monitoring

- Health endpoints: `/health`, `/healthz`, `/readyz`
- Metrics via `/metrics` (expvar JSON)
- Structured logging (no sensitive data in logs)

---

## Supply Chain

### Dependencies

- Go modules with checksums verified via `go.sum`
- Tree-sitter grammars are compiled C libraries, vendored
- Minimal external dependencies in kai-core (no network, no cloud SDKs)
- Core purity enforced via CI script (`check-core-purity.sh`)

### Build

- Reproducible builds via Go toolchain
- Version injected at build time via `ldflags`
- Container images tagged with Git SHA

---

## Incident Response

- Security issues reported to security@kaicontext.com
- See [SECURITY.md](https://github.com/kaicontext/kai/blob/main/SECURITY.md) for disclosure policy
- Target response time: 48 hours for initial acknowledgment
- Critical vulnerabilities: patch within 7 days

---

## What We Do Not Do

- We do not store or transmit source code content
- We do not execute customer code
- We do not access customer secrets or environment variables
- We do not access customer CI runners or production systems
- We do not sell or share customer data
- We do not perform analytics on customer code beyond what's needed for semantic analysis
