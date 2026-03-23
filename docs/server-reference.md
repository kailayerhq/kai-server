# Kailab Server Reference

Documentation for the Kailab data plane and control plane servers.

---

## Kailab Server

Kailab is a fast, multi-tenant, DB-backed server for hosting Kai repositories remotely. It provides HTTP APIs for pushing and fetching snapshots, changesets, and other semantic objects. A single Kailab process can serve many repositories across multiple tenants.

### Architecture

- **Multi-repo**: One server process serves many repositories
- **Multi-tenant**: Repositories are organized by `/{tenant}/{repo}`
- **Per-repo isolation**: Each repo has its own SQLite database
- **LRU caching**: Open repo handles are cached with idle eviction

### Running the Server

```bash
# Build and run
cd kailab
go build -o kailabd ./cmd/kailabd
./kailabd --data ./data --listen :7447

# Or with environment variables
KAILAB_LISTEN=:7447 KAILAB_DATA=./data ./kailabd
```

**Output:**
```
kailabd starting...
  listen:       :7447
  data:         ./data
  max_open:     256
  idle_ttl:     10m0s
Multi-repo mode: routes are /{tenant}/{repo}/v1/...
Admin routes: POST /admin/v1/repos, GET /admin/v1/repos, DELETE /admin/v1/repos/{tenant}/{repo}
```

### Server Configuration

| Variable | Flag | Default | Description |
|----------|------|---------|-------------|
| `KAILAB_LISTEN` | `--listen` | `:7447` | HTTP listen address |
| `KAILAB_DATA` | `--data` | `./data` | Base directory for repo databases |
| `KAILAB_SSH_LISTEN` | `--ssh-listen` | - | SSH listen address for git-upload-pack/receive-pack |
| `KAILAB_MAX_OPEN` | - | `256` | Max number of repos to keep open (LRU) |
| `KAILAB_IDLE_TTL` | - | `10m` | How long to keep idle repos open |
| `KAILAB_MAX_PACK_SIZE` | - | `256MB` | Maximum pack upload size |
| `KAILAB_SSH_ALLOW_USERS` | - | - | Comma/semicolon-separated SSH usernames allowed |
| `KAILAB_SSH_ALLOW_REPOS` | - | - | Comma/semicolon-separated repo allowlist (tenant/repo) |
| `KAILAB_SSH_ALLOW_KEYS` | - | - | Comma/semicolon-separated SSH key fingerprints allowed |
| `KAILAB_SSH_AUDIT` | - | `false` | Enable SSH git audit logging |
| `KAILAB_SSH_SIGN_KEYS` | - | - | Comma/semicolon-separated authorized_keys files used to verify SSH changeset signatures |
| `KAILAB_GIT_MIRROR_ENABLED` | - | `false` | Enable Kai→Git mirror sync (dual-write) |
| `KAILAB_GIT_MIRROR_DIR` | - | `<data>/git-mirror` | Base directory for mirror repos |
| `KAILAB_GIT_MIRROR_ALLOW_REPOS` | - | - | Comma/semicolon-separated allowlist of repos to mirror |
| `KAILAB_GIT_MIRROR_ROLLBACK` | - | `false` | Disable mirroring without changing other settings |
| `KAILAB_KAI_PRIMARY` | - | `false` | Make Kai refs authoritative; disable Git write path |
| `KAILAB_REQUIRE_SIGNED_CHANGESETS` | - | `false` | Require signed changesets for ref updates |
| `KAILAB_DISABLE_GIT_RECEIVE_PACK` | - | `true` | Disable git-receive-pack (Kai-only mode) |
| `KAILAB_GIT_CAPS_EXTRA` | - | - | Comma/semicolon-separated Git capabilities to append |
| `KAILAB_GIT_CAPS_DISABLE` | - | - | Comma/semicolon-separated Git capabilities to remove |
| `KAILAB_GIT_AGENT` | - | `kai` | Git agent capability string |
| `KAILAB_GIT_OBJECT_CACHE_SIZE` | - | `10000` | In-memory git object cache size (entries) |

### SSH Git Access Control

Enable SSH for git operations and optionally enforce allowlists:

```bash
KAILAB_SSH_LISTEN=:2222 \
KAILAB_SSH_ALLOW_USERS=alice,bob \
KAILAB_SSH_ALLOW_REPOS=acme/webapp,acme/api \
KAILAB_SSH_ALLOW_KEYS=SHA256:... \
KAILAB_SSH_AUDIT=true \
./kailabd --data ./data
```

Notes:
- If `KAILAB_SSH_ALLOW_USERS` / `KAILAB_SSH_ALLOW_REPOS` are unset, access is allowed by default.
- Repos are matched as `tenant/repo` (e.g., `acme/webapp`).

### Git-Compatible Repo View (Adapter Layer)

Kailab exposes a Git-compatible read view by adapting Kai refs and snapshots into Git objects:

- **RefAdapter** maps `snap.*`, `cs.*`, and `tag.*` to Git ref namespaces.
- **SnapshotAdapter** converts snapshots into Git trees/blobs.
- **PackBuilder** assembles packfiles for git-upload-pack requests.
- **ObjectStore** caches Git objects for repeated fetches.

This layer keeps Git operations read-only in Kai-only mode while preserving standard Git clients.

### Git Protocol Compatibility Matrix

Supported (tested):
- `git clone` / `git fetch` over SSH (upload-pack v0)
- `git push` over SSH (receive-pack v0) with ref create/update/delete
- `refs/heads/*`, `refs/tags/*`, `refs/kai/*` mappings
- `side-band-64k` response framing for upload-pack
- Shallow clone negotiation (depth=1; server emits `shallow` lines; pack is still synthetic)
- Protocol v2 (minimal): `ls-refs` + `fetch` with packfile section

Unsupported / not yet validated:
- Thin-pack / delta negotiation is accepted but server currently emits full packs
- `multi_ack` / `multi_ack_detailed`
- Partial clone / promisor remotes
- Pack bitmaps / commit-graph optimization

### Dual-Write Git Mirror (Phase 1)

Mirror Kai refs into a bare Git repo for selected pilot repositories. This keeps Git refs/tags updated alongside Kai.

Pilot rollout playbook:
1. Enable mirroring in staging with a tight allowlist.
2. Use shadow parity checks to validate diff parity and drift detection.
3. Expand the allowlist gradually in production.
4. Keep `KAILAB_GIT_MIRROR_ROLLBACK=true` ready for a fast disable.

Example:
```bash
KAILAB_GIT_MIRROR_ENABLED=true \
KAILAB_GIT_MIRROR_ALLOW_REPOS=acme/webapp,acme/api \
KAILAB_GIT_MIRROR_DIR=./data/git-mirror \
./kailabd --data ./data
```

Backfill existing repos into mirrors:
```bash
cd kailab
go run ./cmd/kailab-mirror --data ./data --tenant acme --repo webapp
```

### Migration + Coexistence Strategy

Phased rollout from Git-first to Kai-only:

**Phase 0: Shadow**
- Run `kai shadow import/parity/drift` on pilot repos.
- Validate diff parity and drift alerts without changing write paths.

**Phase 1: Dual-write**
- Enable `KAILAB_GIT_MIRROR_ENABLED` with a tight allowlist.
- Use the mirror backfill tool for existing repos.
- Monitor mirror drift and signature verification logs.

**Phase 2: Kai-primary**
- Set `KAILAB_KAI_PRIMARY=true` and `KAILAB_REQUIRE_SIGNED_CHANGESETS=true`.
- Block Git writes; require signed changesets on ref updates.
- Use the conflict-resolution playbook when pushes fail.

**Phase 3: Kai-only**
- Set `KAILAB_DISABLE_GIT_RECEIVE_PACK=true`.
- Remove `KAILAB_GIT_MIRROR_ROLLBACK` from production configs.
- Keep Git fetch/clone support as read-only compatibility.

### Kai-Primary Enforcement (Phase 2)

Disable Git writes and require signed changesets for ref updates:

```bash
KAILAB_KAI_PRIMARY=true \
KAILAB_REQUIRE_SIGNED_CHANGESETS=true \
./kailabd --data ./data
```

### Capabilities + Compatibility Hooks (Phase 6)

Customize advertised Git protocol capabilities for compatibility:

```bash
KAILAB_GIT_CAPS_EXTRA=thin-pack \
KAILAB_GIT_CAPS_DISABLE=report-status \
KAILAB_GIT_AGENT=kai \
./kailabd --data ./data
```

Conflict resolution playbook:
- If a Git push fails, instruct teams to re-stage changes via Kai and create a new ChangeSet.
- If a ref update is rejected (unsigned), reissue the ChangeSet with SSH signing enabled.
- If there is drift between Kai refs and mirrors, re-run mirror backfill and compare snapshot diffs.
- If CI references Git refs, update pipelines to use `snap.*` or `cs.*` refs.
- Escalate repeated conflicts to a short migration freeze and rebase via Kai.

### Kai-Only Transition (Phase 3)

Disable the Git write path entirely while keeping Git read-only compatibility:

```bash
KAILAB_DISABLE_GIT_RECEIVE_PACK=true \
./kailabd --data ./data
```

Cleanup checklist:
- Turn off any Git write endpoints in load balancers and SSH gateways.
- Remove Git mirror rollback overrides once the cutover stabilizes.
- Ensure CI/CD uses Kai refs (snap/cs) rather than Git refs.
- Document incident response for attempted Git pushes (expected to fail).
- Archive or remove legacy Git auth credentials.

Deprecations:
- Treat `KAILAB_GIT_MIRROR_ROLLBACK` as a temporary safety switch; remove it from production configs in Kai-only.

### Operational Hardening

Observability endpoints:
- `GET /health`, `GET /healthz`, `GET /readyz`
- `GET /metrics` (expvar JSON)

Suggested alerts:
- `kailab_http_errors_total` spikes for write endpoints.
- `kailab_ssh_git_errors_total` non-zero over 5m.
- Slow requests on `/v1/objects/pack` and `/v1/push/negotiate`.

Runbook quick checks:
1. `curl -s http://<host>:<port>/healthz`
2. `curl -s http://<host>:<port>/metrics | jq '.kailab_http_errors_total'`
3. Tail server logs for `upload-pack` / `receive-pack` errors.

### Signed ChangeSets (SSH)

Sign changesets from the CLI using an SSH private key, and have the server verify signatures.

CLI (sign a changeset during staging):

```bash
KAI_SSH_SIGN_KEY=~/.ssh/id_ed25519 \
kai ws stage --ws feature/auth --dir . --message "Add auth flow"
```

Server (verify signatures):

```bash
KAILAB_SSH_SIGN_KEYS=~/.ssh/authorized_keys \
KAILAB_SSH_LISTEN=:2222 \
./kailabd --data ./data
```

### Filesystem Layout

```
data/
├── acme/                    # tenant
│   ├── webapp/              # repo
│   │   └── kai.db           # SQLite database (WAL mode)
│   └── api/
│       └── kai.db
└── other-org/
    └── main/
        └── kai.db
```

### Admin API

Create and manage repositories via the admin API:

```bash
# Create a repository
curl -X POST http://localhost:7447/admin/v1/repos \
  -H "Content-Type: application/json" \
  -d '{"tenant":"acme","repo":"webapp"}'

# List all repositories
curl http://localhost:7447/admin/v1/repos

# Delete a repository
curl -X DELETE http://localhost:7447/admin/v1/repos/acme/webapp
```

### Remote Commands

#### `kai remote set`

Configure a remote server with tenant and repository.

```bash
kai remote set <name> <url> [flags]
```

**Flags:**
- `--tenant <name>` - Tenant/org name (default: `default`)
- `--repo <name>` - Repository name (default: `main`)

**Examples:**
```bash
# Set remote with default tenant/repo
kai remote set origin http://localhost:7447

# Set remote with specific tenant/repo
kai remote set origin http://localhost:7447 --tenant acme --repo webapp

# Multiple remotes for different repos
kai remote set staging http://localhost:7447 --tenant acme --repo staging
kai remote set prod https://kailab.example.com --tenant acme --repo production
```

---

#### `kai remote get`

Get a remote's configuration.

```bash
kai remote get <name>
```

**Example:**
```bash
kai remote get origin
# Output:
# URL:    http://localhost:7447
# Tenant: acme
# Repo:   webapp
```

---

#### `kai remote list`

List all configured remotes.

```bash
kai remote list
```

**Output:**
```
NAME             TENANT        REPO          URL
origin           acme          webapp        http://localhost:7447
staging          acme          staging       http://localhost:7447
prod             acme          production    https://kailab.example.com
```

---

#### `kai remote del`

Delete a remote.

```bash
kai remote del <name>
```

---

#### `kai push`

Push workspaces, changesets, or snapshots to a remote server.

**In Kai, you primarily push workspaces** (the unit of collaboration). Changesets within the workspace are the meaningful units that collaborators review. Snapshots travel automatically as infrastructure.

```bash
kai push [remote] [--ws <workspace>] [target...]
```

**Arguments:**
- `[remote]` - Remote name (default: `origin`)
- `[--ws <workspace>]` - Workspace to push (default: current workspace if set)
- `[target...]` - Explicit targets with prefix: `cs:<ref>` or `snap:<ref>`

**Push hierarchy:**

| Level | Command | What gets pushed |
|-------|---------|------------------|
| **Primary** | `kai push origin` | Current workspace (all changesets + required snapshots) |
| **Secondary** | `kai push origin cs:login_fix` | Single changeset (+ its base/head snapshots) |
| **Tertiary** | `kai push origin snap:abc123` | Single snapshot (advanced/plumbing) |

**Examples:**
```bash
# Push your current workspace (the normal workflow)
kai push origin

# Push a specific workspace
kai push origin --ws feature/auth

# Push a single changeset for targeted review
kai push origin cs:reduce_timeout

# Push a snapshot (rarely needed, advanced)
kai push origin snap:4a2556c0
```

**What gets pushed for a workspace:**
1. Workspace node (metadata: name, status, description)
2. All changesets in the workspace stack (ordered)
3. All snapshots those changesets reference (base/head)
4. All file objects needed to reconstruct the snapshots
5. Refs created:
   - `ws.<name>` → workspace node (enables `fetch --ws`)
   - `ws.<name>.base` → base snapshot
   - `ws.<name>.head` → head snapshot
   - `ws.<name>.cs.<id>` → each changeset

**Flags:**
- `--dry-run` - Show what would be transferred without pushing
- `--force` - Force ref updates on name collisions (content is immutable)

**What happens:**
1. Client discovers which objects remote already has (negotiation)
2. Client computes minimal transfer set (missing changesets → snapshots → files)
3. Client builds zstd-compressed pack of missing objects
4. Pack is uploaded and ingested
5. Refs are atomically updated on server

**Note:** Because content is immutable and addressed by hash, pushes are idempotent and there are no "push conflicts." Conflicts only occur at integration time, where semantic merge handles them.

---

#### `kai fetch`

Fetch workspaces, changesets, or snapshots from a remote server.

```bash
kai fetch [remote] [--ws <workspace>] [target...]
```

**Arguments:**
- `[remote]` - Remote name (default: `origin`)
- `[target...]` - Explicit targets with prefix: `cs:<ref>` or `snap:<ref>`

**Flags:**
- `--ws <name>` - Fetch a specific workspace by name and recreate it locally

**Examples:**
```bash
# Fetch all remote refs (metadata only, lazy content)
kai fetch origin

# Fetch a specific workspace (downloads and recreates locally)
kai fetch origin --ws feature/auth

# Then checkout the workspace to filesystem
kai ws checkout --ws feature/auth --dir ./src

# Fetch a specific changeset
kai fetch origin cs:login_fix

# Fetch a specific snapshot
kai fetch origin snap:main
```

**What happens with `--ws`:**
1. Fetches the workspace ref (`ws.<name>`)
2. Downloads the workspace node and all related objects (snapshots, changesets)
3. Uses BFS to recursively fetch dependencies (parent snapshots, changeset before/after snapshots)
4. Recreates the workspace locally with proper edges (BASED_ON, HEAD_AT, HAS_CHANGESET)
5. Sets both local (`ws.<name>`) and remote tracking (`remote/origin/ws.<name>`) refs

**What happens without `--ws`:**
1. Fetches ref metadata from remote
2. For changesets: downloads the changeset + its base/head snapshots
3. For snapshots: downloads the snapshot + file objects

---

#### `kai clone`

Clone a repository from a remote Kailab server.

```bash
kai clone <org/repo | url> [directory]
```

**Arguments:**
- `<org/repo | url>` - Repository path or full URL
- `[directory]` - Target directory (optional, defaults to repo name)

**URL formats:**
- `org/repo` - Shorthand (uses default server: kaicontext.com)
- `http://server/tenant/repo` - Full URL with server

**Examples:**
```bash
# Clone from default server
kai clone 1m/myrepo

# Clone into specific directory
kai clone 1m/myrepo myproject

# Clone with full URL
kai clone https://kaicontext.com/myorg/myrepo

# Clone from local development server
kai clone http://localhost:8080/myorg/myrepo

# Clone with explicit tenant/repo
kai clone http://localhost:8080 --tenant myorg --repo myrepo
```

**What happens:**
1. Creates a new directory
2. Initializes Kai
3. Sets up the remote
4. Fetches all refs

The default server can be overridden with the `KAI_SERVER` environment variable.

---

#### `kai update`

Update kai CLI to the latest version.

```bash
kai update [flags]
```

**Flags:**
- `--check` - Check for updates without installing

**Examples:**
```bash
# Download and install latest version
kai update

# Check for updates only
kai update --check
```

Downloads and installs the latest kai binary from GitHub releases.

---

#### `kai remote-log`

Show the ref history from a remote server.

```bash
kai remote-log [remote]
```

**Arguments:**
- `[remote]` - Remote name (default: `origin`)

**Example:**
```bash
kai remote-log origin
```

**Output:**
```
snap.latest  abc123...  user@example.com  2024-12-02T15:30:00Z
snap.main    def456...  user@example.com  2024-12-02T14:00:00Z
```

---

#### `kai auth login`

Authenticate with a Kailab control plane server.

```bash
kai auth login [server-url]
```

**Arguments:**
- `[server-url]` - Server URL (optional, uses origin remote's URL if not provided)

**Examples:**
```bash
# Login to a specific server
kai auth login http://localhost:8080

# Login using the origin remote's URL
kai auth login
```

**What happens:**
1. Prompts for your email address
2. Sends a magic link to your email (in dev mode, token is returned directly)
3. You enter the token from your email
4. Tokens are exchanged and stored in `~/.kai/credentials.json`

---

#### `kai auth logout`

Clear stored credentials.

```bash
kai auth logout
```

---

#### `kai auth status`

Show current authentication status.

```bash
kai auth status
```

**Output:**
```
Logged in as: user@example.com
Server:       http://localhost:8080
Status:       Authenticated
```

### Server API

The Kailab server exposes these HTTP endpoints. All repo-scoped endpoints use the `/{tenant}/{repo}` prefix:

**Admin Routes:**

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `POST` | `/admin/v1/repos` | Create a new repository |
| `GET` | `/admin/v1/repos` | List all repositories |
| `DELETE` | `/admin/v1/repos/{tenant}/{repo}` | Delete a repository |

**Repo-Scoped Routes** (prefix: `/{tenant}/{repo}`):

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/{tenant}/{repo}/v1/push/negotiate` | Object negotiation |
| `POST` | `/{tenant}/{repo}/v1/objects/pack` | Ingest zstd-compressed pack |
| `GET` | `/{tenant}/{repo}/v1/objects/{digest}` | Get a single object |
| `PUT` | `/{tenant}/{repo}/v1/refs/{name}` | Update a ref |
| `GET` | `/{tenant}/{repo}/v1/refs` | List all refs |
| `GET` | `/{tenant}/{repo}/v1/log/head` | Get the latest log entry |
| `GET` | `/{tenant}/{repo}/v1/log/entries` | Get paginated ref history |

### Pack Format

Objects are transferred in zstd-compressed packs:

```
[4-byte header length][header JSON][object data...]
```

Header JSON structure:
```json
{
  "objects": [
    {"digest": "abc123...", "kind": "Snapshot", "offset": 0, "size": 1234},
    {"digest": "def456...", "kind": "File", "offset": 1234, "size": 567}
  ]
}
```

### Ref History

All ref updates are logged in an append-only history with hash chaining:

```
entry_hash = BLAKE3(prev_hash || ref_name || target || actor || timestamp)
```

This provides:
- Audit trail of all ref changes
- Tamper detection via hash chain
- Attribution to actors (users/systems)

---

## Kailab Control Plane

Kailab Control is a GitLab-like control plane service that provides user authentication, organization management, and repository metadata. It acts as a gateway to one or more Kailab data plane shards.

### Features

- **Magic Link Authentication** - Passwordless login via email
- **JWT Access Tokens** - Short-lived tokens with refresh capability
- **Personal Access Tokens (PATs)** - Long-lived tokens for CLI/API access
- **Organizations** - Namespaces for grouping repositories
- **Role-Based Access Control** - Owner, Admin, Maintainer, Developer, Reporter, Guest
- **Reverse Proxy** - Routes authenticated requests to kailabd shards
- **Web Console** - Svelte + Tailwind frontend for management

### Running the Control Plane

```bash
cd kailab-control

# Build
make build

# Run in development mode (includes debug logging, dev tokens)
make dev

# Run in production
./kailab-control
```

### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `KLC_LISTEN` | `:8080` | HTTP listen address |
| `KLC_DB_URL` | `kailab-control.db` | SQLite database path |
| `KLC_JWT_KEY` | (required) | JWT signing key |
| `KLC_SHARDS` | `default=http://localhost:7447` | Comma-separated shard URLs |
| `KLC_DEBUG` | `false` | Enable debug mode |

### API Endpoints

**Authentication:**

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/auth/magic-link` | Request magic link email |
| `POST` | `/api/v1/auth/token` | Exchange magic token for access/refresh tokens |
| `POST` | `/api/v1/auth/refresh` | Refresh access token |
| `POST` | `/api/v1/auth/logout` | Logout (delete sessions) |
| `GET` | `/api/v1/me` | Get current user info |

**Organizations:**

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/orgs` | Create organization |
| `GET` | `/api/v1/orgs` | List user's organizations |
| `GET` | `/api/v1/orgs/{org}` | Get organization details |
| `GET` | `/api/v1/orgs/{org}/members` | List members |
| `POST` | `/api/v1/orgs/{org}/members` | Add member |
| `DELETE` | `/api/v1/orgs/{org}/members/{id}` | Remove member |

**Repositories:**

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/orgs/{org}/repos` | Create repository |
| `GET` | `/api/v1/orgs/{org}/repos` | List repositories |
| `GET` | `/api/v1/orgs/{org}/repos/{repo}` | Get repository |
| `DELETE` | `/api/v1/orgs/{org}/repos/{repo}` | Delete repository |

**API Tokens:**

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/tokens` | Create PAT |
| `GET` | `/api/v1/tokens` | List PATs |
| `DELETE` | `/api/v1/tokens/{id}` | Delete PAT |

**Data Plane Proxy:**

| Pattern | Description |
|---------|-------------|
| `/{org}/{repo}/v1/*` | Proxied to kailabd shard |

### Typical Workflow

```bash
# 1. Start the control plane
cd kailab-control && make dev

# 2. Start a kailabd shard
cd kailab && ./kailabd --data ./data

# 3. Configure CLI remote
kai remote set origin http://localhost:8080 --tenant myorg --repo myrepo

# 4. Login
kai auth login http://localhost:8080

# 5. Push/fetch as usual (now authenticated)
kai push origin snap.latest
kai fetch origin snap.main
```

### Web Console

The control plane includes a web console built with Svelte and Tailwind CSS.

**Development:**
```bash
cd kailab-control

# Run frontend dev server (hot reload)
make web-dev

# In another terminal, run the Go backend
make dev
```

**Production build:**
```bash
# Build frontend (outputs to internal/api/web/)
make web

# Build Go binary (embeds frontend)
make build
```

