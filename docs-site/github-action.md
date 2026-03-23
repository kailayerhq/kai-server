# GitHub Action

The official [`kaicontext/kai-action`](https://github.com/kaicontext/kai-action) GitHub Action lets you run Kai selective CI in any GitHub Actions workflow. It downloads the pre-built `kai` binary and runs `kai ci` commands — no Docker build or Node.js runtime needed.

## Quick Start

```yaml
name: Selective Tests
on: pull_request

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: kaicontext/kai-action@v1
        id: kai
        with:
          args: '--git-range ${{ github.event.pull_request.base.sha }}..${{ github.sha }} --out plan.json'

      - name: Run selected tests
        if: steps.kai.outputs.mode == 'selective'
        run: |
          echo "Running ${{ steps.kai.outputs.targets-count }} test targets"
          # Use plan.json to drive your test runner
```

## Inputs

| Input     | Required | Default  | Description                                          |
|-----------|----------|----------|------------------------------------------------------|
| `version` | no       | `latest` | Kai version to install (e.g. `v0.3.0`)              |
| `command` | no       | `plan`   | `kai ci` subcommand: `plan`, `ingest`, or `report`  |
| `args`    | **yes**  | —        | Arguments passed to the subcommand                   |

## Outputs

Outputs are populated when `command` is `plan`:

| Output          | Description                                  |
|-----------------|----------------------------------------------|
| `plan-file`     | Absolute path to the generated `plan.json`   |
| `mode`          | Plan mode: `selective`, `full`, or `skip`    |
| `confidence`    | Confidence score (0.0–1.0)                   |
| `targets-count` | Number of test targets selected              |

Use these outputs in downstream steps to decide whether to run a selective or full test suite.

## Examples

### Rails + RSpec

```yaml
name: Selective RSpec
on: pull_request

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: ruby/setup-ruby@v1
        with:
          bundler-cache: true

      - uses: kaicontext/kai-action@v1
        id: kai
        with:
          args: '--git-range ${{ github.event.pull_request.base.sha }}..${{ github.sha }} --out plan.json'

      - name: Run RSpec
        if: steps.kai.outputs.mode != 'skip'
        run: |
          if [ "${{ steps.kai.outputs.mode }}" = "selective" ]; then
            SPECS=$(jq -r '.targets.run[]' plan.json | grep '_spec\.rb$' | tr '\n' ' ')
            echo "Running selective: $SPECS"
            COVERAGE=true bundle exec rspec $SPECS \
              --format progress \
              --format json --out rspec-results.json
          else
            echo "Running full suite"
            COVERAGE=true bundle exec rspec \
              --format progress \
              --format json --out rspec-results.json
          fi

      - name: Ingest coverage
        if: always() && steps.kai.outputs.mode != 'skip'
        uses: kaicontext/kai-action@v1
        with:
          command: ingest
          args: '--coverage coverage/.resultset.json --results rspec-results.json'
```

### Node.js + Jest

```yaml
name: Selective Jest
on: pull_request

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: npm

      - run: npm ci

      - uses: kaicontext/kai-action@v1
        id: kai
        with:
          args: '--git-range ${{ github.event.pull_request.base.sha }}..${{ github.sha }} --out plan.json'

      - name: Run Jest
        if: steps.kai.outputs.mode != 'skip'
        run: |
          if [ "${{ steps.kai.outputs.mode }}" = "selective" ]; then
            TARGETS=$(jq -r '.targets.run[]' plan.json | paste -sd '|' -)
            echo "Running selective: $TARGETS"
            npx jest --testPathPattern="$TARGETS" --json --outputFile=results.json
          else
            echo "Running full suite"
            npx jest --json --outputFile=results.json
          fi

      - name: Ingest coverage
        if: always() && steps.kai.outputs.mode != 'skip'
        uses: kaicontext/kai-action@v1
        with:
          command: ingest
          args: '--coverage coverage/coverage-final.json --results results.json'
```

### Shadow Mode

Run Kai alongside your full test suite to validate accuracy before switching to selective mode. This is the recommended way to onboard:

```yaml
name: Shadow Mode
on: pull_request

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: kaicontext/kai-action@v1
        id: kai
        with:
          args: '--git-range ${{ github.event.pull_request.base.sha }}..${{ github.sha }} --out plan.json'

      - name: Log plan (shadow only)
        run: |
          echo "Kai would run: ${{ steps.kai.outputs.mode }}"
          echo "Targets: ${{ steps.kai.outputs.targets-count }}"
          echo "Confidence: ${{ steps.kai.outputs.confidence }}"
          cat plan.json

      - name: Full test suite (always runs)
        run: |
          COVERAGE=true bundle exec rspec \
            --format json --out rspec-results.json

      - name: Ingest coverage
        if: always()
        uses: kaicontext/kai-action@v1
        with:
          command: ingest
          args: '--coverage coverage/.resultset.json --results rspec-results.json'
```

### Low Confidence Fallback

Override the plan and run the full suite when Kai isn't confident enough:

```yaml
- uses: kaicontext/kai-action@v1
  id: kai
  with:
    args: '--git-range ${{ github.event.pull_request.base.sha }}..${{ github.sha }} --out plan.json'

- name: Run tests
  run: |
    CONFIDENCE="${{ steps.kai.outputs.confidence }}"
    MODE="${{ steps.kai.outputs.mode }}"

    # Fall back to full suite if confidence is below threshold
    if [ "$(echo "$CONFIDENCE < 0.4" | bc -l)" = "1" ]; then
      echo "Low confidence ($CONFIDENCE), running full suite"
      bundle exec rspec
    elif [ "$MODE" = "selective" ]; then
      SPECS=$(jq -r '.targets.run[]' plan.json | tr '\n' ' ')
      bundle exec rspec $SPECS
    elif [ "$MODE" != "skip" ]; then
      bundle exec rspec
    fi
```

## How It Works

1. **Install** — Downloads the pre-built `kai` binary from [GitHub Releases](https://github.com/kaicontext/kai/releases). No Docker image, no build step.
2. **Plan** — Runs `kai ci plan` to analyze your Git diff at the semantic level (functions, classes, symbols) and produce a `plan.json` with affected test targets.
3. **Output** — Parses `plan.json` and sets GitHub Action outputs (`mode`, `confidence`, `targets-count`) so downstream steps can act on the plan.

## Migrating from GitLab CI

If you have an existing Kai GitLab CI setup, the concepts map directly:

| GitLab CI                          | GitHub Actions                           |
|------------------------------------|------------------------------------------|
| `CI_MERGE_REQUEST_DIFF_BASE_SHA`   | `github.event.pull_request.base.sha`     |
| `CI_COMMIT_SHA`                    | `github.sha`                             |
| `artifacts: reports: dotenv:`      | `$GITHUB_OUTPUT` (automatic)             |
| `needs: [kai-plan]`               | `steps.kai.outputs.*`                    |
| `curl ... \| sh` to install       | `uses: kaicontext/kai-action@v1`         |

See the [GitLab CI example](/gitlab-ci) for the equivalent GitLab configuration.
