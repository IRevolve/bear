# 🐻 Bear

> **B**uild, **E**valuate, **A**pply, **R**epeat

A Terraform-inspired CI/CD tool for monorepos. Bear detects changes, resolves dependencies, and orchestrates builds and deployments with a simple plan/apply workflow.

<div class="grid cards" markdown>

- :mag: **Git-based change detection** — Only build what changed
- :link: **Dependency tracking** — Automatically rebuild dependents
- :clipboard: **Plan/Apply workflow** — Review changes before deploying
- :lock: **Lock file** — Track deployed versions per artifact
- :books: **Library support** — Validate-only artifacts
- :arrows_counterclockwise: **Rollback** — Redeploy any previous version
- :globe_with_meridians: **Multi-language** — Go, Node, Python, Rust, Java, TypeScript
- :dart: **Many targets** — Docker, CloudRun, Kubernetes, Lambda, S3, Fly, Vercel

</div>

## Quick Example

```yaml title="bear.config.yml"
name: my-platform

use:
  languages: [go, node]
  targets: [docker, cloudrun]
```

```yaml title="services/api/bear.artifact.yml"
name: api
target: cloudrun
depends:
  - shared-lib
env:
  PROJECT: my-gcp-project
```

```bash
# See what would happen
bear plan

# Execute the plan
bear apply
```

## How It Works

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Detect    │────▶│    Plan     │────▶│    Apply    │
│   Changes   │     │  (Review)   │     │  (Execute)  │
└─────────────┘     └─────────────┘     └─────────────┘
```

1. **Detect** — Compare each artifact against its last deployed commit
2. **Plan** — Show affected artifacts with their dependencies
3. **Apply** — Validate (lint, test, build) then deploy

## Getting Started

<div class="grid cards" markdown>

- [:material-download: **Installation**](getting-started/installation.md)

    Install Bear via `go install` or build from source

- [:material-rocket-launch: **Quick Start**](getting-started/quickstart.md)

    Get up and running in 5 minutes

- [:material-cog: **Configuration**](configuration/project.md)

    Learn about `bear.config.yml` and artifacts

- [:material-package: **Presets**](configuration/presets.md)

    Use community presets for languages and targets

</div>
