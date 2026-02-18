# 🐻 Bear

> **B**uild, **E**valuate, **A**pply, **R**epeat

A Terraform-inspired CI/CD tool for monorepos. Detect changes, validate, deploy — only what changed.

## Install

```bash
go install github.com/irevolve/bear@latest
```

## Quick Start

```yaml
# bear.config.yml
name: my-platform

use:
  languages: [go, node]
  targets: [docker, cloudrun]
```

```yaml
# services/api/bear.artifact.yml
name: api
target: cloudrun
depends: [shared-lib]
```

```bash
bear plan    # detect changes, validate
bear apply   # deploy
```

## How It Works

1. **Detect** — Compare each artifact against its last deployed commit
2. **Plan** — Validate changed artifacts in parallel, write deployment plan
3. **Apply** — Deploy from the plan, update lock file

## Features

- **Change detection** — Git-based, per-artifact, no base branch needed
- **Dependencies** — Libraries trigger rebuilds of dependent services
- **Plan/Apply** — Review before deploying
- **Lock file** — Tracks deployed versions per artifact
- **Pinning** — Pin to specific commits, instant rollback
- **Presets** — Pre-built configs for Go, Node, Python, Rust, Java, TypeScript
- **Targets** — Docker, CloudRun, Kubernetes, Lambda, S3, Helm

## Commands

| Command | Description |
|---------|-------------|
| `bear init` | Initialize a new project |
| `bear plan [artifacts...]` | Detect changes, validate, create plan |
| `bear apply` | Execute the deployment plan |
| `bear check` | Validate config and dependencies |
| `bear list [--tree]` | List artifacts / dependency tree |
| `bear preset list\|show\|update` | Manage presets |

## Documentation

Full docs: [irevolve.github.io/bear](https://irevolve.github.io/bear)

- [Getting Started](https://irevolve.github.io/bear/getting-started/) — Install and first deploy
- [Configuration](https://irevolve.github.io/bear/configuration/) — All config options
- [CI/CD](https://irevolve.github.io/bear/ci-cd/) — GitHub Actions, GitLab CI, Jenkins
- [Commands](https://irevolve.github.io/bear/commands/) — Full reference
- [Concepts](https://irevolve.github.io/bear/concepts/) — Change detection, dependencies, pinning

## License

Apache 2.0
