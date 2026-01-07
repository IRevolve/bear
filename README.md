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
- 🌍 **Multi-language** — Go, Node.js, Python, Rust, Java, TypeScript (extensible)
- 🎯 **Configurable targets** — Docker, CloudRun, Kubernetes, Lambda, S3, Helm
- 📦 **Community presets** — Import pre-built language and target configs from [bear-presets](https://github.com/irevolve/bear-presets)

## Installation

```bash
go install github.com/irevolve/bear@latest
```

Or build from source:

```bash
git clone https://github.com/irevolve/bear.git
cd bear
go build -o bear .
```

## Quick Start

### 1. Initialize a project

```bash
bear init
```

This creates `bear.config.yml` with auto-detected languages.

### 2. Use presets (recommended)

Instead of defining languages and targets manually, use community presets:

```yaml
# bear.config.yml
name: my-platform

use:
  languages: [go, node]
  targets: [docker, cloudrun]
```

View available presets:

```bash
bear preset list              # Show all available presets
bear preset show language go  # Show language details
bear preset show target docker # Show target details
bear preset update            # Refresh preset cache from GitHub
```

### 3. Or define custom languages/targets

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

### 4. Add artifact configs

```yaml
# services/user-api/bear.artifact.yml
name: user-api
target: cloudrun
depends:
  - shared-lib
params:
  MEMORY: 1Gi
env:
  PROJECT: my-gcp-project
```

For libraries (validate-only, no deploy):

```yaml
# libs/shared/bear.lib.yml
name: shared-lib
```

### 5. Run Bear

```bash
# List all artifacts
bear list

# Show what would be built/deployed
bear plan

# Execute the plan
bear apply

# Target specific artifacts
bear plan user-api order-api

# Pin artifact to a specific version
bear apply user-api --pin abc1234

# Different project directory
bear plan -d ./other-project
```

## Commands

| Command | Description |
|---------|-------------|
| `bear init` | Initialize a new Bear project |
| `bear list` | List all discovered artifacts |
| `bear list --tree` | Show dependency tree |
| `bear plan [artifacts...]` | Show planned validations and deployments |
| `bear plan --validate` | Plan and run validation (lint, test) |
| `bear apply [artifacts...]` | Execute the plan (validate, then deploy) |
| `bear check` | Validate configuration and dependencies |
| `bear preset list` | Show available presets |
| `bear preset show <type> <name>` | Show preset details |
| `bear preset update` | Refresh preset cache |

### Global Flags

| Flag | Description |
|------|-------------|
| `-d, --dir <path>` | Path to project directory (default: `.`) |
| `-f, --force` | Force operation, ignore pinned artifacts |

### Apply Flags

| Flag | Description |
|------|-------------|
| `--pin <commit>` | Pin artifact to a specific commit |
| `-c, --commit` | Commit and push lock file with [skip ci] |

### Pinning

When you pin an artifact, it stays at that version:

```bash
# Pin user-api to commit abc1234
bear apply user-api --pin abc1234

# Future applies will skip pinned artifacts
bear plan  # Shows: user-api 📌 PINNED

# Force apply to override pin (removes the pin)
bear apply user-api --force
```

## How It Works

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Detect    │────▶│    Plan     │────▶│    Apply    │
│   Changes   │     │  (Review)   │     │  (Execute)  │
└─────────────┘     └─────────────┘     └─────────────┘
      │                   │                   │
      ▼                   ▼                   ▼
 Compare to         Show affected       Phase 1: Validate
 last deployed      artifacts with       (setup, lint,
 commit (lock)      dependencies          test, build)
                                               │
                                               ▼
                                         Phase 2: Deploy
                                          (per target)
                                               │
                                               ▼
                                         Update lock file
```

### Change Detection

Bear compares each artifact against its **last deployed commit** (from `bear.lock.yml`). For each artifact it checks:

1. **Uncommitted changes** — Staged, unstaged, or untracked files
2. **Commits since last deploy** — Changes between the deployed commit and HEAD
3. **New artifacts** — Files tracked in git but never deployed

This means Bear doesn't need a base branch — it tracks state per artifact.

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

Apache 2.0

---

## Presets

Bear loads community presets from [bear-presets](https://github.com/irevolve/bear-presets). Presets are cached locally in `~/.bear/presets/` for 24 hours.

### Available Languages

| Language | Detection | Steps |
|----------|-----------|-------|
| `go` | `go.mod` | download, vet, test, build |
| `node` | `package.json` | install, lint, test, build |
| `typescript` | `tsconfig.json` | install, typecheck, lint, test, build |
| `python` | `requirements.txt` | venv, install, lint, test |
| `rust` | `Cargo.toml` | check, clippy, test, build |
| `java` | `pom.xml` | compile, test, package |

### Available Targets

| Target | Description |
|--------|-------------|
| `docker` | Build and push Docker images |
| `cloudrun` | Deploy to Google Cloud Run |
| `cloudrun-job` | Deploy Cloud Run jobs |
| `kubernetes` | Apply Kubernetes manifests |
| `helm` | Deploy with Helm charts |
| `lambda` | Deploy AWS Lambda functions |
| `s3` | Deploy to S3 buckets |
| `s3-static` | Deploy static sites to S3 |

### Contributing Presets

Want to add or improve a preset? Contribute to [bear-presets](https://github.com/irevolve/bear-presets)!
