# CI/CD Integration

Bear is designed to work seamlessly with CI/CD systems. This guide covers common patterns and best practices.

## The Lock File Problem

After `bear apply`, the lock file (`bear.lock.yml`) is updated with deployed versions. This creates a challenge:

1. Lock file changes → Need to commit
2. Commit triggers CI → New build starts
3. New build sees no changes → Wastes resources

## Solutions

### Option 1: `--commit` Flag (Recommended)

Bear can automatically commit and push the lock file with `[skip ci]`:

```bash
bear apply --commit
```

This:

1. Runs the deployment
2. Updates `bear.lock.yml`
3. Commits with message: `chore(bear): update lock file [skip ci]`
4. Pushes to the repository

Most CI systems (GitHub Actions, GitLab CI, CircleCI, etc.) recognize `[skip ci]` and won't trigger a new build.

### Option 2: Path Filters

Configure your CI to ignore changes to only `bear.lock.yml`:

=== "GitHub Actions"

    ```yaml
    on:
      push:
        branches: [main]
        paths-ignore:
          - 'bear.lock.yml'
    ```

=== "GitLab CI"

    ```yaml
    workflow:
      rules:
        - changes:
            - bear.lock.yml
          when: never
        - when: always
    ```

### Option 3: Commit Message Check

Check if the commit is a lock file update:

```yaml
jobs:
  build:
    if: "!contains(github.event.head_commit.message, '[skip ci]')"
```

## GitHub Actions Example

Complete workflow for Bear:

```yaml title=".github/workflows/deploy.yml"
name: Deploy

on:
  push:
    branches: [main]
    paths-ignore:
      - 'bear.lock.yml'
      - 'docs/**'
      - '*.md'

jobs:
  deploy:
    runs-on: ubuntu-latest
    
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0  # Full history for change detection
      
      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      
      - name: Install Bear
        run: go install github.com/IRevolve/Bear/cmd@latest
      
      - name: Configure Git
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
      
      - name: Plan
        run: bear plan
      
      - name: Apply
        run: bear apply --commit
        env:
          # Add your deployment credentials
          DOCKER_USERNAME: ${{ secrets.DOCKER_USERNAME }}
          DOCKER_PASSWORD: ${{ secrets.DOCKER_PASSWORD }}
```

## GitLab CI Example

```yaml title=".gitlab-ci.yml"
stages:
  - deploy

deploy:
  stage: deploy
  image: golang:1.21
  
  rules:
    - if: $CI_COMMIT_BRANCH == "main"
      changes:
        - "**/*"
        - "!bear.lock.yml"
  
  before_script:
    - go install github.com/IRevolve/Bear/cmd@latest
    - git config user.name "GitLab CI"
    - git config user.email "ci@gitlab.com"
  
  script:
    - bear plan
    - bear apply --commit
```

## Environment Variables

Pass secrets via environment variables:

```yaml
env:
  # Docker
  DOCKER_USERNAME: ${{ secrets.DOCKER_USERNAME }}
  DOCKER_PASSWORD: ${{ secrets.DOCKER_PASSWORD }}
  REGISTRY: ghcr.io/${{ github.repository_owner }}
  
  # GCP
  PROJECT: ${{ secrets.GCP_PROJECT }}
  GOOGLE_APPLICATION_CREDENTIALS: ${{ secrets.GCP_KEY }}
  
  # AWS
  AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
  AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
  AWS_REGION: eu-central-1
```

## Parallel Builds

For large monorepos, you can parallelize:

```yaml
jobs:
  plan:
    runs-on: ubuntu-latest
    outputs:
      matrix: ${{ steps.plan.outputs.matrix }}
    steps:
      - uses: actions/checkout@v4
      - run: |
          # Generate matrix from bear plan output
          bear plan --json > plan.json
          echo "matrix=$(cat plan.json | jq -c '.artifacts')" >> $GITHUB_OUTPUT

  deploy:
    needs: plan
    runs-on: ubuntu-latest
    strategy:
      matrix:
        artifact: ${{ fromJson(needs.plan.outputs.matrix) }}
    steps:
      - uses: actions/checkout@v4
      - run: bear apply ${{ matrix.artifact }}
```

## Manual Approval

For production deployments, add manual approval:

```yaml
jobs:
  plan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: bear plan
  
  approve:
    needs: plan
    runs-on: ubuntu-latest
    environment: production  # Requires approval
    steps:
      - run: echo "Approved"
  
  deploy:
    needs: approve
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: bear apply --commit
```

## Dry Run in PRs

Run `bear plan` in pull requests to preview changes:

```yaml
on:
  pull_request:
    branches: [main]

jobs:
  plan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      
      - run: bear plan
      
      - name: Comment on PR
        uses: actions/github-script@v7
        with:
          script: |
            // Add plan output as PR comment
```

## See Also

- [bear apply](../commands/apply.md)
- [Lock File](lock-file.md)
