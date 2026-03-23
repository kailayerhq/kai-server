# Self-Hosted Deployment Guide

How to run Kai's server infrastructure in your own environment.

---

## Overview

Kai can be deployed in three modes:

| Mode | Description | Data Location |
|------|-------------|---------------|
| **SaaS** | Hosted on kaicontext.com | Kai Cloud |
| **Self-hosted** | Run on your infrastructure | Your servers |
| **Local-only** | CLI only, no server | Developer machine |

This guide covers self-hosted deployment.

---

## Architecture

A self-hosted Kai deployment consists of two components:

```
┌──────────────────────────────────┐
│         Your Infrastructure       │
│                                   │
│  ┌─────────────────┐             │
│  │ kailab-control   │ ← Web UI + Auth + API gateway
│  │ (control plane)  │             │
│  └────────┬─────────┘             │
│           │                       │
│  ┌────────▼─────────┐             │
│  │ kailabd           │ ← Data plane (semantic storage)
│  │ (data plane)      │             │
│  └──────────────────┘             │
│                                   │
│  ┌──────────────────┐             │
│  │ SQLite databases   │ ← Per-repo storage
│  │ (persistent disk)  │             │
│  └──────────────────┘             │
└──────────────────────────────────┘
```

---

## Prerequisites

- Linux server or Kubernetes cluster
- Go 1.24+ (for building from source)
- GCC/Clang (for CGO dependencies)
- Persistent storage (SSD recommended)
- Domain name + TLS certificate

---

## Quick Start (Single Server)

### 1. Build

```bash
# Clone the server repo
git clone https://github.com/kaicontext/kai-server.git
cd kai-server

# Build data plane
cd kailab
CGO_ENABLED=1 go build -o kailabd ./cmd/kailabd

# Build control plane
cd ../kailab-control
make build
```

### 2. Configure

```bash
# Data plane
export KAILAB_LISTEN=:7447
export KAILAB_DATA=/var/lib/kai/data

# Control plane
export KLC_LISTEN=:8080
export KLC_DB_URL=/var/lib/kai/kailab-control.db
export KLC_JWT_KEY=$(openssl rand -hex 32)
export KLC_SHARDS=default=http://localhost:7447
```

### 3. Run

```bash
# Start data plane
./kailabd --data /var/lib/kai/data --listen :7447 &

# Start control plane
./kailab-control &
```

### 4. Connect CLI

```bash
kai remote set origin https://kaicontext.com --tenant myorg --repo myrepo
kai auth login https://kaicontext.com
```

---

## Kubernetes Deployment

### Deployment manifests

```yaml
# kailabd deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kailabd
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kailabd
  template:
    metadata:
      labels:
        app: kailabd
    spec:
      containers:
        - name: kailabd
          image: your-registry/kailabd:latest
          ports:
            - containerPort: 7447
          env:
            - name: KAILAB_LISTEN
              value: ":7447"
            - name: KAILAB_DATA
              value: "/data"
          volumeMounts:
            - name: data
              mountPath: /data
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: kailabd-data
---
# kailab-control deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kailab-control
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kailab-control
  template:
    metadata:
      labels:
        app: kailab-control
    spec:
      containers:
        - name: kailab-control
          image: your-registry/kailab-control:latest
          ports:
            - containerPort: 8080
          env:
            - name: KLC_LISTEN
              value: ":8080"
            - name: KLC_DB_URL
              value: "/data/kailab-control.db"
            - name: KLC_JWT_KEY
              valueFrom:
                secretKeyRef:
                  name: kai-secrets
                  key: jwt-key
            - name: KLC_SHARDS
              value: "default=http://kailabd:7447"
          volumeMounts:
            - name: data
              mountPath: /data
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: kailab-control-data
---
# Persistent volume claims
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: kailabd-data
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 50Gi
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: kailab-control-data
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 10Gi
```

---

## Configuration Reference

### Data Plane (kailabd)

| Variable | Default | Description |
|----------|---------|-------------|
| `KAILAB_LISTEN` | `:7447` | HTTP listen address |
| `KAILAB_DATA` | `./data` | Base directory for repo databases |
| `KAILAB_MAX_OPEN` | `256` | Max open repo handles (LRU) |
| `KAILAB_IDLE_TTL` | `10m` | Idle repo eviction time |
| `KAILAB_MAX_PACK_SIZE` | `256MB` | Max push payload size |

### Control Plane (kailab-control)

| Variable | Default | Description |
|----------|---------|-------------|
| `KLC_LISTEN` | `:8080` | HTTP listen address |
| `KLC_DB_URL` | `kailab-control.db` | SQLite database path |
| `KLC_JWT_KEY` | (required) | JWT signing key |
| `KLC_SHARDS` | (required) | Comma-separated shard URLs |
| `KLC_VERSION` | `0.1.0` | Version string |

---

## Backups

SQLite databases can be backed up with:

```bash
# Online backup (safe while server is running)
sqlite3 /var/lib/kai/data/tenant/repo/kai.db ".backup /backups/repo-$(date +%Y%m%d).db"

# Or snapshot the persistent volume
```

---

## Monitoring

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Basic health check + version |
| `GET /healthz` | Liveness probe |
| `GET /readyz` | Readiness probe (checks DB) |
| `GET /metrics` | expvar JSON metrics |

---

## Security Considerations

- Generate a strong JWT key: `openssl rand -hex 32`
- Use TLS termination at the load balancer or ingress
- Restrict network access to the data plane (only control plane should reach it)
- Back up SQLite databases regularly
- Monitor `/metrics` for error spikes
