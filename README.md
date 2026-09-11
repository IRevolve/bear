# 🐻 Bear

> **B**uild, **E**valuate, **A**pply, **R**epeat

A Terraform-inspired CI/CD tool for monorepos. Detect changes, validate, deploy — only what changed.

**Upgrading to v5.0.0?** It has three breaking changes: deployment environments
are now declared per project (add a required `environments:` list to
`bear.config.yml` and make every artifact allowlist a subset of it); `bear plan`
no longer runs any commands and `bear apply` now always builds immediately
before it deploys, so a plan file saved before this release is rejected, not
misapplied; and `bear check` is renamed to `bear doctor`, with no alias.
`bear.lock.yml` is untouched. Read the
[v5 upgrade guide](docs/migration-v5.md) and the
[v5.0.0 release notes](docs/releases/v5.0.0.md). Coming from v3 or earlier? Do the
[v4 migration](docs/migration-v4.md) first: deployment became opt-in per
environment, legacy history is not automatically migrated, and old plans must be
recreated.

## Install

Install the pinned v5.0.0 source with Go 1.25+:

```bash
git clone --branch v5.0.0 --depth 1 https://github.com/irevolve/bear.git bear-v5
go -C bear-v5 install -ldflags="-X github.com/irevolve/bear/commands.Version=5.0.0" .
```

Put `$(go env GOPATH)/bin` on `PATH` (or your configured `GOBIN`). Release binaries
are also available from the [v5.0.0 release](https://github.com/irevolve/bear/releases/tag/v5.0.0).

## Quick Start

```yaml
# bear.config.yml
name: my-platform
environments: [dev, int, prd]

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
bear validate # run every artifact's build steps (merge request)
bear plan dev # decide what would deploy for dev; runs nothing
bear apply    # build and deploy the plan
```

`environments` in `bear.config.yml` declares the project's deployment environments.
There is no built-in set: `[dev, int, prd]` above is just `bear init`'s default, and
`[preprd, prd]` or a single `[prd]` is equally valid. `environments` in
`bear.artifact.yml` is a deployment allowlist, and must be a subset of that
declaration: absent or `[]` means the artifact never deploys, though change
detection still runs. Every plan
requires a declared environment as its first positional
argument, even one that ends up with nothing to deploy; only listed environments can deploy. Inherited or configured
`ENVIRONMENT` variables cannot select policy or satisfy this requirement.

Commit intended source before planning: a normal plan with any deployments
requires a clean Git repository before it runs — plan itself executes nothing, so
this only guards against planning a deployment from uncommitted changes. Apply
verifies the saved source commit and fingerprint before it runs anything, then
builds and deploys fresh for every artifact it deploys.

## How It Works

1. **Detect** — Compare each artifact against its last deployed commit in the selected environment
2. **Plan** — Decide what would deploy, write the plan; runs no commands
3. **Apply** — Build (language steps) then deploy (target steps) from the plan, update lock file

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
| `bear validate [artifacts...]` | Run build steps against the working tree; no environment, no plan, no deployment |
| `bear plan <environment> [artifacts...]` | Decide what would deploy for the required environment, declared in `bear.config.yml`; runs no commands |
| `bear apply` | Build and deploy the plan |
| `bear doctor` | Validate config and dependencies |
| `bear list [--tree]` | List artifacts / dependency tree |
| `bear preset list\|show\|update` | Manage presets |

The chain is `doctor` (configuration) → `validate` (merge request) → `plan`
(deployment review and approval) → `apply` (execution). `bear validate` needs no
environment, no deployment credentials and no Git history, so it runs in a shallow
pull-request checkout. `bear plan` runs no commands at all — it only decides what
would deploy. `bear apply` is the only command that executes anything: for every
deploying artifact it runs the language's build steps, then the target's deploy
steps. See the [v5.0.0 release notes](docs/releases/v5.0.0.md).

## Documentation

CI examples use explicit environments, default-deny deployment allowlists, and
trusted-branch Git credentials. Run this repository's sample project from
[`examples/`](examples/README.md), with its required language/deployment toolchains;
the minimal Bear images do not bundle them.

Preset imports use immutable revisions with reusable offline caches. Pin
`use.revision` explicitly for reproducibility; `bear preset --revision <sha>`
selects the CLI's revision independently. `bear init` is noninteractive, writes the
project's `environments` declaration (`--environments`, default `dev,int,prd`), and
imports presets only when requested with `--lang`/`--target`.

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
- [Upgrade to v5](docs/migration-v5.md) — Declare your deployment environments

## License

Apache 2.0
