# Getting Started

## Install

Install the pinned v4.0.0 source with Go 1.25+:

```bash
git clone --branch v4.0.0 --depth 1 https://github.com/irevolve/bear.git bear-v4
go -C bear-v4 install -ldflags="-X github.com/irevolve/bear/commands.Version=4.0.0" .
```

Put `$(go env GOPATH)/bin` on `PATH` (or your configured `GOBIN`). Alternatively,
download the binary for your platform from the
[v4.0.0 release](https://github.com/irevolve/bear/releases/tag/v4.0.0).
For an existing monorepo, follow the [v4 migration guide](migration-v4.md) first.

Verify: `bear --version`

**Requirements:** Git (for change detection). Go 1.25+ if building Bear from source.
Planning and deployment also require the toolchains/CLIs used by your configured
steps; installing Bear alone does not install them.

## First Project

### 1. Initialize

```bash
cd my-monorepo
bear init
```

Creates an empty `bear.config.yml` using the directory name as the project name.
Init is noninteractive and does not auto-detect languages. Use
`bear init --lang go,node --target docker,cloudrun` instead to validate and import
those presets at Bear's built-in revision.

### 2. Configure

```yaml title="bear.config.yml"
name: my-platform

use:
  revision: dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a
  languages: [go, node]
  targets: [docker, cloudrun]
```

!!! tip "Presets"
    Run `bear preset list` to see the default revision's languages and targets.
    For another project revision, pass its SHA with `--revision`; preset commands
    do not read project config. See [Presets](commands/preset.md).

### 3. Add Artifacts

Each deployable service gets a `bear.artifact.yml`:

```yaml title="services/api/bear.artifact.yml"
name: api
target: cloudrun
environments: [dev, int, prd]
depends: [shared-lib]

vars:
  PROJECT: my-gcp-project
```

`environments` lists where deployment is allowed. Omitting it or setting `[]`
disables deployment everywhere while retaining validation. Use only the environments
you intend to deploy to; valid names are `dev`, `int`, and `prd`.

Libraries (validate-only, no deploy) get a `bear.lib.yml`:

```yaml title="libs/shared/bear.lib.yml"
name: shared-lib
```

### 4. Validate

```bash
bear check    # validate config, dependencies, cycles
bear list     # show discovered artifacts
```

### 5. Deploy

Commit your intended source and configuration first. Normal plans with deployments
require clean Git source before validation. Bear state is excluded. Validation
must not change tracked source or HEAD; nonignored generated files are saved in
the post-validation fingerprint and must still match at apply time. Ignored
dependencies/outputs are not fingerprinted, so preserve or reproducibly provision
them yourself. See [Source Safety](concepts/plan-apply.md#source-safety).

```bash
bear plan dev # detect changes, validate, create plan for dev
bear apply    # execute the saved plan
```

Use `bear plan <environment> [artifacts...]`: `dev`, `int`, or `prd` is required before
any artifact filters, including for validation-only plans. For example, use
`bear plan dev user-api` to select one artifact. Inherited or configured `ENVIRONMENT`
variables cannot satisfy this requirement. The positional selection is injected as
`ENVIRONMENT` into all validation and deployment steps and stored in the saved plan.
The environment does not automatically configure deployment targets or credentials.

Bear saves each successful deployment to `bear.lock.yml` and checkpoints the plan,
then commits the lock with `[skip ci]` and pushes it. Configure Git identity,
authentication, and an existing remote branch, or use `bear apply --no-commit`
and publish the local history yourself. See [CI/CD](ci-cd.md#trusted-deployments).
Failures retain the plan for recovery; a failed push can leave a local commit
requiring a manual push, not just another apply.

## What Happens Under the Hood

1. **Detect** — Compare each artifact against its last deployed commit in the selected environment (from `bear.lock.yml`)
2. **Validate** — Run language steps (tests, lint, build) in parallel
3. **Deploy** — Run target steps (docker build, push, deploy) in parallel
4. **Lock** — Persist each completion in environment history and the plan, then publish the lock

## Next

- [Configuration](configuration.md) — All config options
- [CI/CD](ci-cd.md) — Automate with GitHub Actions, GitLab CI, or Jenkins
- [Commands](commands/index.md) — Full command reference
