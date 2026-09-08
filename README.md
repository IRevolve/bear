# 🐻 Bear

> **B**uild, **E**valuate, **A**pply, **R**epeat

A Terraform-inspired CI/CD tool for monorepos. Detect changes, validate, deploy — only what changed.

**Upgrading to v4.0.0?** Read the [monorepo migration guide](docs/migration-v4.md)
before running CI. Deployment is now opt-in per environment, legacy history is not
automatically migrated, and old plans must be recreated. See the
[v4.0.0 release notes](docs/releases/v4.0.0.md).

## Install

Install the pinned v4.0.0 source with Go 1.25+:

```bash
git clone --branch v4.0.0 --depth 1 https://github.com/irevolve/bear.git bear-v4
go -C bear-v4 install -ldflags="-X github.com/irevolve/bear/commands.Version=4.0.0" .
```

Put `$(go env GOPATH)/bin` on `PATH` (or your configured `GOBIN`). Release binaries
are also available from the [v4.0.0 release](https://github.com/irevolve/bear/releases/tag/v4.0.0).

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
environments: [dev, int, prd]
depends: [shared-lib]
```

```bash
bear plan dev # detect changes, validate, plan for dev
bear apply    # deploy the saved plan
```

`environments` is a deployment allowlist: absent or `[]` means validation only,
with no deployment. Every plan requires `dev`, `int`, or `prd` as its first positional
argument, including validation-only plans; only listed environments can deploy. Inherited or configured
`ENVIRONMENT` variables cannot select policy or satisfy this requirement.

Commit intended source before planning: normal plans with any deployments require
a clean Git repository before validation. Apply verifies the saved source commit
and post-validation fingerprint before pending deployments.

## How It Works

1. **Detect** — Compare each artifact against its last deployed commit in the selected environment
2. **Plan** — Validate changed artifacts in parallel, write deployment plan
3. **Apply** — Deploy from the plan, update lock file

## Features

- **Change detection** — Git-based, per-artifact, no base branch needed
- **Dependencies** — Libraries trigger rebuilds of dependent services
- **Plan/Apply** — Review before deploying
- **Lock file** — Tracks deployed versions and pins per environment and artifact
- **Pinning** — Validate and deploy a selected revision in private worktrees
- **Presets** — Pre-built configs for Go, Node, Python, Rust, Java, TypeScript
- **Targets** — Docker, CloudRun, Kubernetes, Lambda, S3, Helm

## Commands

| Command | Description |
|---------|-------------|
| `bear init` | Initialize a new project |
| `bear plan <environment> [artifacts...]` | Detect changes, validate, create plan for the required environment (`dev`, `int`, or `prd`) |
| `bear apply` | Execute the deployment plan |
| `bear check` | Validate config and dependencies |
| `bear list [--tree]` | List artifacts / dependency tree |
| `bear preset list\|show\|update` | Manage presets |

## Documentation

CI examples use explicit environments, default-deny deployment allowlists, and
trusted-branch Git credentials. Run this repository's sample project from
[`examples/`](examples/README.md), with its required language/deployment toolchains;
the minimal Bear images do not bundle them.

Preset imports use immutable revisions with reusable offline caches. Pin
`use.revision` explicitly for reproducibility; `bear preset --revision <sha>`
selects the CLI's revision independently. `bear init` is noninteractive and imports
presets only when requested with `--lang`/`--target`.

Plans bind source and policy, use relative paths, and checkpoint each successful
deployment. Git failures retain the plan and return nonzero; a failed push after
a local commit requires manual push recovery. See
[source safety and retry limits](docs/concepts/plan-apply.md#source-safety), including
ignored dependencies and the lack of an external deployment transaction. Legacy
artifact-only history is retained but ignored, not automatically mapped to
environments; replan rather than reusing old saved plans.

Full docs: [irevolve.github.io/bear](https://irevolve.github.io/bear)

- [Getting Started](https://irevolve.github.io/bear/getting-started/) — Install and first deploy
- [Configuration](https://irevolve.github.io/bear/configuration/) — All config options
- [CI/CD](https://irevolve.github.io/bear/ci-cd/) — GitHub Actions, GitLab CI, Jenkins
- [Commands](https://irevolve.github.io/bear/commands/) — Full reference
- [Concepts](https://irevolve.github.io/bear/concepts/) — Change detection, dependencies, pinning

## License

Apache 2.0
