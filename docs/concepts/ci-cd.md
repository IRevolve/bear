# CI/CD Integration

Bear is designed to work seamlessly with CI/CD systems. This guide covers common patterns and best practices.

## The Lock File Problem

After `bear apply`, the lock file (`bear.lock.yml`) is updated with deployed versions. This creates a challenge:

1. Lock file changes → Need to commit
2. Commit triggers CI → New build starts
3. New build sees no changes → Wastes resources

## Solutions

### Option 1: Auto-Commit (Default)

Bear automatically commits and pushes the lock file with `[skip ci]` after every `bear apply`:

```bash
bear plan && bear apply
```

This:

1. Creates and validates a deployment plan
2. Runs the deployments
3. Updates `bear.lock.yml`
4. Commits with message: `chore(bear): update lock file [skip ci]`
5. Pushes to the repository

Most CI systems (GitHub Actions, GitLab CI, CircleCI, etc.) recognize `[skip ci]` and won't trigger a new build.

To disable auto-commit, use `--no-commit`:

```bash
bear apply --no-commit
```

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

## Using Docker Images

Bear provides official Docker images for easy CI integration without installing Go:

| Image | Size | Use Case |
|-------|------|----------|
| `ghcr.io/irevolve/bear:latest` | ~5MB | Minimal, just Bear binary |
| `ghcr.io/irevolve/bear:0.4.0-alpine` | ~15MB | With Git and shell |
| `ghcr.io/irevolve/bear:0.4.0-debian` | ~50MB | Full environment |

!!! tip "Which image to choose?"
    - Use `:latest` (scratch) for pure `plan`/`apply` with `--no-commit`
    - Use `-alpine` or `-debian` for auto-commit (default, requires Git)

### GitHub Actions with Docker

```yaml title=".github/workflows/deploy.yml"
name: Deploy

on:
  push:
    branches: [main]
    paths-ignore:
      - 'bear.lock.yml'

jobs:
  deploy:
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/irevolve/bear:latest
    
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      
      - name: Plan
        run: bear plan
      
      - name: Apply
        run: bear apply

  # With auto-commit (default, requires Git)
  deploy-with-commit:
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/irevolve/bear:0.4.0-alpine
    
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      
      - name: Configure Git
        run: |
          git config --global user.name "github-actions[bot]"
          git config --global user.email "github-actions[bot]@users.noreply.github.com"
          git config --global --add safe.directory $GITHUB_WORKSPACE
      
      - name: Apply
        run: bear apply
```

### GitLab CI with Docker

```yaml title=".gitlab-ci.yml"
deploy:
  image: ghcr.io/irevolve/bear:0.4.0-alpine
  stage: deploy
  
  before_script:
    - git config --global user.name "GitLab CI"
    - git config --global user.email "ci@gitlab.com"
  
  script:
    - bear plan
    - bear apply
```

### Generic Docker Usage

```bash
# Run plan in any CI
docker run --rm -v $(pwd):/workspace -w /workspace \
  ghcr.io/irevolve/bear:latest plan

# Run apply
docker run --rm -v $(pwd):/workspace -w /workspace \
  -e DOCKER_USERNAME -e DOCKER_PASSWORD \
  ghcr.io/irevolve/bear:latest apply
```

## GitHub Actions Example (without Docker)

Complete workflow installing Bear from source:

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
        run: go install github.com/irevolve/bear@latest
      
      - name: Configure Git
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
      
      - name: Plan
        run: bear plan
      
      - name: Apply
        run: bear apply
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
    - go install github.com/irevolve/bear@latest
    - git config user.name "GitLab CI"
    - git config user.email "ci@gitlab.com"
  
  script:
    - bear plan
    - bear apply
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

## Parallel Execution

Bear runs validations and deployments in parallel by default (`--concurrency 10`).
For large monorepos, you can tune the concurrency:

```bash
# Limit to 5 parallel validations
bear plan --concurrency 5

# Limit to 3 parallel deployments
bear apply --concurrency 3
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
      - run: bear apply
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
