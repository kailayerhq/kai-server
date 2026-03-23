# Data Flow Documentation

How data moves through the Kai system — what we read, what we store, and what we don't.

---

## Overview

Kai analyzes code structure to determine test impact. It does not execute code, access secrets, or store source code on our servers.

```
Developer's Repo
      │
      ▼
  Kai CLI (local)
      │ Parses AST, extracts symbols,
      │ builds dependency graph
      ▼
  Local Graph Store (.kai/db.sqlite)
      │
      │ Push (snapshots + changesets)
      ▼
  Kailab Server (hosted)
      │ Stores semantic metadata:
      │ - File hashes (not content)
      │ - Symbol names and signatures
      │ - Dependency edges
      │ - Change classifications
      ▼
  CI Plan Output
      │ List of test targets to run
      ▼
  Customer's CI Runner
      (tests execute in their environment)
```

---

## What Kai Reads

| Data | Where | Purpose |
|------|-------|---------|
| Source files | Local repo (CLI) | AST parsing for symbol extraction |
| Git refs | Local repo (CLI) | Snapshot creation from branches/tags |
| File paths | Local repo (CLI) | Module matching and file classification |

**Kai reads source code locally on the developer's machine.** The CLI parses files using tree-sitter to extract structural information (function names, class definitions, import statements). Source code is never sent to Kailab servers.

## What Kai Stores Locally

| Data | Location | Purpose |
|------|----------|---------|
| Snapshots | `.kai/db.sqlite` | Semantic state at a point in time |
| ChangeSets | `.kai/db.sqlite` | Semantic diff between two snapshots |
| Symbols | `.kai/db.sqlite` | Functions, classes, variables extracted from code |
| File content hashes | `.kai/db.sqlite` | BLAKE3 hashes for change detection |
| Module definitions | `kai.modules.yaml` | User-defined logical groupings |
| Refs | `.kai/db.sqlite` | Named pointers to snapshots/changesets |

## What Kai Sends to Kailab (Hosted)

When a user runs `kai push`, the following is transmitted:

| Data | Sent? | Details |
|------|-------|---------|
| Source code content | No | Never transmitted |
| File content hashes | Yes | BLAKE3 hashes only |
| Symbol names | Yes | Function/class/variable names and signatures |
| Dependency edges | Yes | Which symbols depend on which |
| File paths | Yes | Relative paths within the repo |
| Change classifications | Yes | e.g., FUNCTION_ADDED, CONSTANT_UPDATED |
| Git commit hashes | Yes | For ref mapping |
| Secrets / env vars | No | Never accessed or transmitted |
| Test results | No | Unless user explicitly runs `kai ci ingest` |

## What Kailab Stores (Server-Side)

| Data | Storage | Retention |
|------|---------|-----------|
| Semantic snapshots | SQLite (per-repo) | Until deleted by user |
| ChangeSets | SQLite (per-repo) | Until deleted by user |
| Symbol metadata | SQLite (per-repo) | Until deleted by user |
| File hashes | SQLite (per-repo) | Until deleted by user |
| Ref history | SQLite (append-only) | Permanent (audit trail) |
| User accounts | SQLite (control plane) | Until account deleted |
| Access tokens | SQLite (control plane) | Until expired or revoked |

## What Kai Never Accesses

- Source code content (only structure)
- Secrets, API keys, or environment variables
- Customer databases
- Production systems
- Customer CI runner environments
- Build artifacts or binaries

---

## Data in Transit

- All communication between CLI and Kailab uses HTTPS (TLS 1.2+)
- Push/fetch payloads are zstd-compressed
- Authentication via JWT tokens (short-lived) with refresh via HTTP-only cookies

## Data at Rest

- Server-side data stored in SQLite with WAL mode
- Each repository has its own isolated database file
- No cross-tenant data access
- Database files stored on encrypted volumes (GKE persistent disks)

---

## CI Plan Output

The output of `kai ci plan` is a list of test targets:

```json
{
  "mode": "guarded",
  "targets": ["tests/auth_test.go", "tests/billing_test.go"],
  "skipped": ["tests/ui_test.go", "tests/migration_test.go"],
  "confidence": 0.94,
  "expanded": false
}
```

This output stays local or is consumed by the customer's CI runner. Kai does not execute tests.
