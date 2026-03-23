# Encryption Model

How Kai encrypts data in transit and at rest.

---

## Data in Transit

| Path | Protocol | Encryption |
|------|----------|------------|
| CLI → Kailab Server | HTTPS | TLS 1.2+ |
| Browser → Control Plane | HTTPS | TLS 1.2+ |
| Control Plane → Data Plane (internal) | HTTPS | TLS 1.2+ |
| Push payloads | HTTPS | TLS 1.2+ with zstd compression |

- TLS certificates managed via cloud provider (GKE ingress)
- No plaintext HTTP in production
- HSTS headers enforced

## Data at Rest

| Data | Storage | Encryption |
|------|---------|------------|
| Repository databases | GKE persistent disk | AES-256 (Google-managed encryption) |
| User credentials | SQLite (control plane DB) | AES-256 (disk-level) |
| JWT signing keys | Kubernetes Secret | Encrypted at rest (etcd encryption) |
| Backup snapshots | GCE disk snapshots | AES-256 (Google-managed) |

### Disk Encryption

All persistent disks on GKE are encrypted at rest by default using Google-managed encryption keys (AES-256). This covers:
- All SQLite database files
- All tenant data
- All log files written to disk

### Secrets Management

- JWT signing key stored as Kubernetes Secret
- Kubernetes Secrets encrypted at rest in etcd
- No secrets in environment variables beyond what K8s injects
- No secrets in container images

## Content Hashing

Kai uses BLAKE3 for content-addressed hashing:
- File content is hashed locally on the developer's machine
- Only the hash is transmitted and stored server-side
- Hashes are used for deduplication and change detection
- Source code cannot be reconstructed from hashes

## Password / Credential Storage

- Kai uses passwordless authentication (magic links)
- No passwords are stored
- Personal Access Tokens are stored as bcrypt hashes (server-side)
- Refresh tokens are stored with expiration and revocation support

## SSH Signing (Optional)

- ChangeSets can be signed with SSH keys (`KAI_SSH_SIGN_KEY`)
- Signatures verified server-side against authorized_keys
- Uses standard SSH signature format (ssh-keygen compatible)

---

## What Is NOT Encrypted (By Design)

- Symbol names and function signatures in the database (needed for query performance)
- File paths in the database (needed for module matching)
- Change classifications (needed for plan generation)

These are structural metadata, not source code content. Source code is never stored server-side.

---

## Future Considerations

- Customer-managed encryption keys (CMEK) for enterprise tier
- End-to-end encryption of push payloads (CLI encrypts, server stores opaque blobs)
- Field-level encryption for symbol names (for highly sensitive codebases)
