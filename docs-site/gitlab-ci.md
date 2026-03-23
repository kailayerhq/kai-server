# GitLab CI

Kai integrates into GitLab CI/CD pipelines to run only the tests affected by your merge request. This guide covers the three-stage pattern: **plan**, **test**, **ingest**.

## Quick Start

Add these stages to your `.gitlab-ci.yml`:

```yaml
stages:
  - plan
  - test
  - coverage
```

### Stage 1: Compute Test Plan

```yaml
kai-plan:
  stage: plan
  image: ruby:3.3
  before_script:
    - curl -fsSL https://kaicontext.com/install.sh | sh
  script:
    - |
      kai ci plan \
        --git-range ${CI_MERGE_REQUEST_DIFF_BASE_SHA}..${CI_COMMIT_SHA} \
        --out plan.json \
        --explain

      MODE=$(jq -r .mode plan.json)
      TEST_COUNT=$(jq '.targets.run | length' plan.json)
      CONFIDENCE=$(jq -r .safety.confidence plan.json)

      # Fall back to full suite if confidence is too low
      if [ "$(echo "$CONFIDENCE < 0.4" | bc -l)" = "1" ]; then
        MODE="full"
      fi

      echo "KAI_MODE=$MODE" >> plan.env
      echo "KAI_TEST_COUNT=$TEST_COUNT" >> plan.env
      echo "KAI_CONFIDENCE=$CONFIDENCE" >> plan.env

      echo "Plan: mode=$MODE, tests=$TEST_COUNT, confidence=$CONFIDENCE"
  artifacts:
    paths:
      - plan.json
    reports:
      dotenv: plan.env
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
```

### Stage 2: Run Affected Specs

```yaml
rspec-selective:
  stage: test
  image: ruby:3.3
  needs: [kai-plan]
  services:
    - postgres:16
    - redis:7
  before_script:
    - bundle install --jobs 4 --retry 3
    - bundle exec rails db:prepare
  script:
    - |
      if [ "$KAI_MODE" = "skip" ]; then
        echo "No tests affected by this change"
        exit 0
      fi

      SPECS=$(jq -r '.targets.run[]' plan.json | grep '_spec\.rb$' | tr '\n' ' ')

      if [ -z "$SPECS" ]; then
        echo "No Ruby specs in plan, skipping"
        exit 0
      fi

      echo "Running $KAI_TEST_COUNT specs (confidence: $KAI_CONFIDENCE)"
      COVERAGE=true bundle exec rspec $SPECS \
        --format progress \
        --format json --out rspec-results.json
  artifacts:
    paths:
      - coverage/
      - rspec-results.json
    when: always
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
```

### Stage 2 (fallback): Full Suite

```yaml
rspec-full:
  stage: test
  image: ruby:3.3
  needs: [kai-plan]
  services:
    - postgres:16
    - redis:7
  before_script:
    - bundle install --jobs 4 --retry 3
    - bundle exec rails db:prepare
  script:
    - |
      echo "Low confidence ($KAI_CONFIDENCE), running full suite"
      COVERAGE=true bundle exec rspec \
        --format progress \
        --format json --out rspec-results.json
  artifacts:
    paths:
      - coverage/
      - rspec-results.json
    when: always
  rules:
    - if: $KAI_MODE == "full"
      when: always
    - when: never
```

### Stage 3: Ingest Coverage

```yaml
ingest-coverage:
  stage: coverage
  image: ruby:3.3
  needs: [rspec-selective]
  before_script:
    - curl -fsSL https://kaicontext.com/install.sh | sh
  script:
    - |
      if [ -f coverage/.resultset.json ]; then
        kai ci ingest-coverage \
          --from coverage/.resultset.json \
          --format nyc \
          --branch "$CI_COMMIT_REF_NAME" \
          --tag "pipeline-$CI_PIPELINE_ID"
        echo "Coverage ingested for future selective runs"
      else
        echo "No coverage data found, skipping ingest"
      fi
  artifacts:
    paths:
      - .kai/coverage-map.json
  cache:
    key: kai-coverage
    paths:
      - .kai/coverage-map.json
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
```

## Variables Reference

| GitLab CI Variable                  | Usage                                      |
|-------------------------------------|--------------------------------------------|
| `CI_MERGE_REQUEST_DIFF_BASE_SHA`    | Base commit of the MR (start of git range) |
| `CI_COMMIT_SHA`                     | Head commit (end of git range)             |
| `CI_PIPELINE_SOURCE`                | Trigger type (`merge_request_event`, etc.) |
| `CI_COMMIT_BRANCH`                  | Current branch name                        |
| `CI_DEFAULT_BRANCH`                 | Default branch (usually `main`)            |

## How It Works

1. **Plan** — `kai ci plan` analyzes the merge request diff at the semantic level and outputs `plan.json` with affected test targets. The plan environment variables (`KAI_MODE`, `KAI_CONFIDENCE`, `KAI_TEST_COUNT`) are passed to downstream jobs via `dotenv` artifacts.
2. **Test** — The selective job reads `plan.json` and runs only the listed specs. A parallel full-suite job runs when confidence is below threshold.
3. **Ingest** — After tests pass on the default branch, coverage data is ingested so Kai can make better plans next time.

See the [GitHub Action](/github-action) page for the equivalent GitHub Actions setup.
