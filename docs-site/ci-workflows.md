---
outline: deep
---

<div v-pre>

# CI Workflows

Kailab CI runs GitHub Actions-compatible workflows natively. Define your pipelines in `.kailab/workflows/` and they execute on every `kai push` — no external CI system required.

## Workflow Files

Place YAML files in `.kailab/workflows/` in your repository:

```
.kailab/
  workflows/
    ci.yml
    deploy.yml
```

Workflows are synced automatically when you `kai push`. The control plane parses each file, registers triggers, and creates jobs when events match.

## Workflow Syntax

Kailab CI uses GitHub Actions workflow syntax. If you've written a GitHub Actions workflow, you can use it in Kailab with minimal changes.

### Triggers

```yaml
on:
  push:
    branches: [main]
    tags: ['v*']
    paths:
      - 'src/**'
    paths-ignore:
      - 'docs/**'

  pull_request:
    branches: [main]
    types: [opened, synchronize]

  workflow_dispatch:
    inputs:
      environment:
        description: 'Deploy target'
        required: true
        default: 'staging'
        type: choice
        options: [staging, production]

  schedule:
    - cron: '0 2 * * 1-5'  # Weekdays at 2am UTC
```

`pull_request` triggers map to Kai review events internally.

### Jobs

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    container:
      image: golang:1.25-alpine
    steps:
      - uses: actions/checkout@v4
      - name: Run tests
        run: go test ./...

  build:
    needs: [test]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Build
        run: make build
```

Jobs run in Kubernetes pods. `runs-on` selects the runner pool (`ubuntu-latest` maps to Linux runners). Use `container` to specify a custom image.

### Matrix Builds

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go-version: ['1.24', '1.25']
        os: [ubuntu-latest]
      fail-fast: false
    container:
      image: golang:${{ matrix.go-version }}-alpine
    steps:
      - uses: actions/checkout@v4
      - run: go test ./...
```

### Environment Variables and Secrets

Workflow-level and job-level `env` blocks work as expected:

```yaml
env:
  REGISTRY: us-central1-docker.pkg.dev/${{ secrets.GCP_PROJECT_ID }}/myrepo

jobs:
  build:
    runs-on: ubuntu-latest
    env:
      CGO_ENABLED: '0'
    steps:
      - run: echo $REGISTRY
```

Secrets are set via the CLI and referenced with `${{ secrets.NAME }}`:

```bash
kai ci secret-set GCP_SERVICE_KEY <base64-encoded-key>
kai ci secret-set GCP_PROJECT_ID myproject
```

### Conditionals and Timeouts

```yaml
steps:
  - name: Deploy
    if: github.ref == 'refs/heads/main'
    run: ./deploy.sh
    timeout-minutes: 10
    continue-on-error: false
```

### Concurrency

Prevent concurrent runs of the same workflow:

```yaml
concurrency:
  group: deploy-${{ github.ref }}
  cancel-in-progress: true
```

### Reusable Workflows

Call another workflow in the same repo:

```yaml
jobs:
  deploy:
    uses: ./.kailab/workflows/deploy.yml
    with:
      environment: production
    secrets: inherit
```

The called workflow must have `on: workflow_call` as a trigger.

## Built-in Actions

These GitHub Actions are implemented natively — no network fetch required:

| Action | Description |
|--------|-------------|
| `actions/checkout@v4` | Checks out from the Kai data plane (tar archive, fast) |
| `actions/cache@v4` | GCS-backed caching with key/restore-keys support |
| `actions/setup-go` | Installs Go toolchain |
| `actions/setup-node` | Installs Node.js |
| `actions/setup-python` | Installs Python |
| `actions/setup-java` | Installs JDK |
| `actions/upload-artifact` | Uploads build artifacts |
| `actions/download-artifact` | Downloads artifacts from earlier jobs |

For actions not in this list, use `run` steps with shell commands instead.

## Secrets and Variables

### Managing Secrets

```bash
# Set a secret
kai ci secret-set GCP_SERVICE_KEY "$(base64 < service-account.json)"

# List secrets (names only, values are never shown)
kai ci secrets
```

Secrets are encrypted at rest and injected as environment variables in job pods.

### Managing Variables

Variables are non-sensitive configuration values available as `${{ vars.NAME }}`:

```bash
kai ci variable-set DEPLOY_REGION us-central1
```

## CLI Commands

```bash
# List recent runs
kai ci runs

# Show run details with job status
kai ci run 15

# View logs (shows first failed job, or specify --job)
kai ci logs 15
kai ci logs 15 --job build-kailab

# Cancel a running pipeline
kai ci cancel 15

# Re-run a pipeline
kai ci rerun 15
```

## Example: Full CI/CD Pipeline

This is the workflow Kailab uses to build and deploy itself:

```yaml
name: CI

on:
  push:
    branches: [main]
    tags: ['*']

env:
  KAILAB_IMAGE: us-central1-docker.pkg.dev/${{ secrets.GCP_PROJECT_ID }}/preplan/kailab

jobs:
  test:
    runs-on: ubuntu-latest
    container:
      image: golang:1.25-alpine
    steps:
      - uses: actions/checkout@v4
      - uses: actions/cache@v4
        with:
          path: /root/go/pkg/mod
          key: go-mod-${{ hashFiles('**/go.sum') }}
          restore-keys: go-mod-
      - name: Run tests
        run: go test ./...

  build:
    needs: [test]
    runs-on: ubuntu-latest
    container:
      image: gcr.io/kaniko-project/executor:v1.23.2-debug
    steps:
      - uses: actions/checkout@v4
      - name: Configure registry auth
        run: |
          mkdir -p /kaniko/.docker
          echo "${{ secrets.GCP_SERVICE_KEY }}" | base64 -d > /kaniko/.docker/config.json
      - name: Build and push
        run: |
          /kaniko/executor \
            --context=/workspace \
            --dockerfile=/workspace/Dockerfile \
            --destination=${{ env.KAILAB_IMAGE }}:${{ github.sha }} \
            --destination=${{ env.KAILAB_IMAGE }}:latest \
            --cache=true

  deploy:
    needs: [build]
    runs-on: ubuntu-latest
    container:
      image: google/cloud-sdk:alpine
    steps:
      - uses: actions/checkout@v4
      - name: Deploy to GKE
        run: |
          echo "${{ secrets.GCP_SERVICE_KEY }}" | base64 -d > /tmp/key.json
          gcloud auth activate-service-account --key-file=/tmp/key.json
          gcloud container clusters get-credentials ${{ secrets.GKE_CLUSTER }} \
            --zone ${{ secrets.GKE_ZONE }}
          kubectl set image deployment/myapp app=${{ env.KAILAB_IMAGE }}:${{ github.sha }}
          kubectl rollout status deployment/myapp --timeout=300s
        timeout-minutes: 10
```

## How It Works

1. `kai push` sends a snapshot to the data plane
2. The data plane notifies the control plane of the push event
3. The control plane syncs workflow files from the snapshot
4. Matching workflows create a run with jobs queued
5. Runners poll for available jobs, claim them, and execute steps in Kubernetes pods
6. Job dependencies (`needs`) are resolved — downstream jobs start when dependencies succeed
7. If a job fails, dependent jobs are cancelled and the run completes as failed

</div>
