# Quick Start

## Install

```bash
# curl (Linux/macOS)
curl -sSL https://get.kaicontext.com | sh

# Homebrew
brew install kaicontext/kai/kai

# Verify
kai --version
```

## Initialize in a Git Repo

```bash
cd your-project
kai init
```

Kai detects your git repo and offers to:

1. **Import git history** — replays commits as semantic snapshots (last 50 by default)
2. **Install post-commit hook** — auto-captures on each `git commit`
3. **Generate CI config** — GitHub Actions or GitLab CI workflow to keep Kai in sync

## Capture & Push

```bash
kai capture -m "Add user authentication"   # Snapshot with message
kai push                                     # Push to kaicontext.com
```

The message shows as the CI run headline on kaicontext.com.

## View on the Web

Visit `https://kaicontext.com/<org>/<repo>` to see:

- **Files** — IDE-style split panel with search, language breakdown, type-specific icons
- **Reviews** — Semantic diffs, inline comments, approve/merge workflow
- **CI** — Live pipeline status with SSE updates, auto-scroll logs
- **History** — Semantic changelog with changeset details

## Code Reviews

```bash
kai review open --title "Fix login bug"     # Create review on latest changeset
kai push                                     # Push review to server
# Reviewers comment on kaicontext.com...
kai fetch --review abc123                    # Sync comments back to CLI
kai review comments abc123                   # View comments locally
```

## Import Full Git History

```bash
kai import              # Last 50 commits
kai import --all        # Entire history
kai import --max 200    # Last 200 commits
```

## MCP Server for AI Assistants

```bash
# Claude Code
claude mcp add kai -- kai mcp serve

# Cursor — add to .cursor/mcp.json
{
  "mcpServers": {
    "kai": {
      "command": "kai",
      "args": ["mcp", "serve"]
    }
  }
}
```

12 tools available: `kai_status`, `kai_symbols`, `kai_files`, `kai_diff`, `kai_impact`, `kai_callers`, `kai_callees`, `kai_context`, `kai_dependencies`, `kai_dependents`, `kai_tests`, `kai_refresh`.

## CI Sync

### GitHub Actions

Generated automatically by `kai init`, or create manually:

```yaml
# .github/workflows/kai-sync.yml
name: Kai Sync
on:
  push:
    branches: [main]
jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: curl -sSL https://get.kaicontext.com | sh
      - run: kai auth login --token "$KAI_TOKEN"
      - run: kai capture -m "${{ github.event.head_commit.message }}"
      - run: kai push
```

Add `KAI_TOKEN` as a repository secret (get from `kai auth token`).

### GitLab CI

```yaml
# .kai-sync.gitlab-ci.yml
kai-sync:
  stage: .post
  image: alpine:3.19
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
  script:
    - curl -sSL https://get.kaicontext.com | sh
    - kai auth login --token "$KAI_TOKEN"
    - kai capture -m "$CI_COMMIT_MESSAGE"
    - kai push
```

Add `KAI_TOKEN` as a CI variable.
