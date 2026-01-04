# 🐻 Bear

> **B**uild, **E**valuate, **A**pply, **R**epeat

A Terraform-inspired CI/CD tool for monorepos. Bear detects changes, resolves dependencies, and orchestrates builds and deployments with a simple plan/apply workflow.

## Features

- 🔍 **Git-based change detection** — Only build what changed
- 🔗 **Dependency tracking** — Automatically rebuild dependents (transitive)
- 📋 **Plan/Apply workflow** — Review changes before deploying
- 🔒 **Lock file** — Track deployed versions per artifact
- 📚 **Library support** — Validate-only artifacts (no deploy)
- ⏪ **Rollback** — Redeploy any previous version
- 🌍 **Multi-language** — Go, Node.js, Python, Rust (extensible)
- 🎯 **Configurable targets** — CloudRun, Lambda, S3, Docker, custom

## Installation

```bash
go install github.com/IRevolve/Bear/cmd@latest
```

Or build from source:

```bash
git clone https://github.com/IRevolve/Bear.git
cd Bear
go build -o bear ./cmd/main.go
```

## Quick Start

### 1. Create a project config

```yaml
# bear.config.yml
name: my-platform

languages:
  - name: go
    detection:
      files: [go.mod]
    validation:
      setup:
        - name: Download modules
          run: go mod download
      lint:
        - name: Vet
          run: go vet ./...
      test:
        - name: Test
          run: go test -race ./...
      build:
        - name: Build
          run: go build -o dist/app .

targets:
  - name: cloudrun
    defaults:
      REGION: europe-west1
    deploy:
      - name: Build
        run: docker build -t gcr.io/$PROJECT/$NAME:$VERSION .
      - name: Push
        run: docker push gcr.io/$PROJECT/$NAME:$VERSION
      - name: Deploy
        run: gcloud run deploy $NAME --image gcr.io/$PROJECT/$NAME:$VERSION
```

### 2. Add artifact configs

```yaml
# services/user-api/bear.artifact.yml
name: user-api
target: cloudrun
depends:
  - shared-lib
params:
  MEMORY: 1Gi
```

For libraries (validate-only, no deploy):

```yaml
# libs/shared/bear.lib.yml
name: shared-lib
```

### 3. Run Bear

```bash
# List all artifacts
bear list

# Show what would be built/deployed
bear plan

# Execute the plan
bear apply

# Target specific artifacts
bear plan -a user-api -a order-api

# Rollback to a previous version
bear apply -a user-api --rollback=abc1234

# Dry run (no actual execution)
bear apply --dry-run
```

## Commands

| Command | Description |
|---------|-------------|
| `bear list [path]` | List all discovered artifacts |
| `bear plan [path]` | Show planned validations and deployments |
| `bear apply [path]` | Execute the plan (validate, then deploy) |

### Global Flags

| Flag | Description |
|------|-------------|
| `-a, --artifact <name>` | Target specific artifact(s) |
| `--base <branch>` | Base branch for change detection (default: `main`) |
| `--dry-run` | Show what would happen without executing |
| `--rollback <commit>` | Rollback to a specific commit |

## How It Works

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Detect    │────▶│    Plan     │────▶│    Apply    │
│   Changes   │     │  (Review)   │     │  (Execute)  │
└─────────────┘     └─────────────┘     └─────────────┘
      │                   │                   │
      ▼                   ▼                   ▼
 Git diff vs        Show affected       Phase 1: Validate
 base branch        artifacts with       (setup, lint,
                    dependencies          test, build)
                                               │
                                               ▼
                                         Phase 2: Deploy
                                          (per target)
                                               │
                                               ▼
                                         Update lock file
```

### Change Detection

Bear compares the current branch against the base branch (default: `main`) to detect file changes. It then:

1. Maps changed files to artifacts
2. Resolves transitive dependencies
3. Creates an execution plan

### Dependency Resolution

If artifact A depends on library B, and B changes, then A is marked for rebuild:

```
shared-lib (changed)
    ↓
user-api (dependency changed) → rebuild + redeploy
    ↓
dashboard (dependency changed) → rebuild + redeploy
```

### Lock File

`bear.lock.yml` tracks what's deployed:

```yaml
artifacts:
  user-api:
    commit: abc1234567890
    timestamp: "2026-01-04T10:00:00Z"
    version: abc1234
    target: cloudrun
```

## Project Structure

```
my-monorepo/
├── bear.config.yml          # Main config
├── bear.lock.yml            # Deployed versions (auto-generated)
├── apps/
│   └── dashboard/
│       ├── bear.artifact.yml
│       └── ...
├── libs/
│   ├── shared-go/
│   │   ├── bear.lib.yml     # Library (validate-only)
│   │   └── ...
│   └── ui-components/
│       └── bear.lib.yml
└── services/
    ├── user-api/
    │   ├── bear.artifact.yml
    │   └── ...
    └── order-api/
        └── bear.artifact.yml
```

## Configuration Reference

### bear.config.yml

```yaml
name: project-name

languages:
  - name: go                    # Language identifier
    detection:
      files: [go.mod]           # Files that identify this language
    validation:
      setup: [...]              # Setup steps
      lint: [...]               # Linting steps  
      test: [...]               # Test steps
      build: [...]              # Build steps

targets:
  - name: cloudrun              # Target identifier
    defaults:                   # Default parameters
      REGION: europe-west1
    deploy: [...]               # Deployment steps
```

### bear.artifact.yml

```yaml
name: my-service                # Unique artifact name
target: cloudrun                # Target from config
depends:                        # Dependencies (optional)
  - shared-lib
  - other-service
params:                         # Override target defaults
  MEMORY: 2Gi
```

### bear.lib.yml

```yaml
name: shared-lib                # Library name (validate-only)
```

## Variables

These variables are available in deployment steps:

| Variable | Description |
|----------|-------------|
| `$NAME` | Artifact name |
| `$VERSION` | Short commit hash (7 chars) |
| Custom params | From target defaults and artifact params |

## License

MIT
