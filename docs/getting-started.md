# Getting Started

## Install

Install the pinned v5.0.0 source with Go 1.25+:

```bash
git clone --branch v5.0.0 --depth 1 https://github.com/irevolve/bear.git bear-v5
go -C bear-v5 install -ldflags="-X github.com/irevolve/bear/commands.Version=5.0.0" .
```

Put `$(go env GOPATH)/bin` on `PATH` (or your configured `GOBIN`). Alternatively,
download the binary for your platform from the
[v5.0.0 release](https://github.com/irevolve/bear/releases/tag/v5.0.0).
For an existing monorepo, follow the [v5 migration guide](migration-v5.md) first.

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

Creates a `bear.config.yml` using the directory name as the project name and
`dev,int,prd` as the deployment environments. Init is noninteractive and does not
auto-detect languages. Use `--environments preprd,prd` to declare your own, and
`bear init --lang go,node --target docker,cloudrun` to validate and import those
presets at Bear's built-in revision.

### 2. Configure

```yaml title="bear.config.yml"
name: my-platform
environments: [dev, int, prd]

use:
  revision: dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a
  languages: [go, node]
  targets: [docker, cloudrun]
```

`environments` is required and declares every deployment environment this project
has. There is no built-in set: `[dev, int, prd]` above is just `bear init`'s default,
and `[preprd, prd]` or a single `[prd]` works the same. Names must match
`[a-z][a-z0-9-]*`, at most 32 characters, with no duplicates. Order is preserved in
every message and listing, and implies no promotion order. See
[Deployment Environments](configuration.md#deployment-environments).

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

`environments` here lists where deployment is allowed, and must be a subset of the
environments the project declares. Omitting it or setting `[]`
disables deployment everywhere; change detection and dependency propagation still
run. List only the
environments you intend to deploy to; an entry the project does not declare fails
`bear doctor`, `bear validate`, and `bear plan`.

Libraries are never deployed, but `bear validate` runs their language's steps like
any other artifact. They get a `bear.lib.yml`:

```yaml title="libs/shared/bear.lib.yml"
name: shared-lib
```

### 4. Validate

```bash
bear doctor  # validate config, dependencies, cycles
bear list    # show discovered artifacts
```

### 5. Deploy

Commit your intended source and configuration first. A normal plan with
deployments requires clean Git source before it runs — plan itself never runs a
command or modifies the tree, so this is purely a "commit first" gate, not a
build/test check. Bear state is excluded. `bear apply` verifies the saved commit
and fingerprint still match before it runs anything, then builds (the language's
steps) and deploys (the target's steps) fresh for every artifact it deploys.
Ignored dependencies/outputs are never fingerprinted; provision them the same way
you would for any fresh build. See [Source Safety](concepts/plan-apply.md#source-safety).

```bash
bear plan dev # decide what would deploy for dev; runs nothing
bear apply    # build and deploy the plan
```

Use `bear plan <environment> [artifacts...]`: an environment declared in
`bear.config.yml` is required before
any artifact filters, even for a plan that ends up with nothing to deploy. For example, use
`bear plan dev user-api` to select one artifact. `bear doctor` prints the declared
list if you are unsure. Inherited or configured `ENVIRONMENT`
variables cannot satisfy this requirement. The positional selection is injected as
`ENVIRONMENT` into every step `bear apply` later runs for a deploying artifact,
and is stored in the saved plan; plan itself runs no steps.
The environment does not automatically configure deployment targets or credentials.

Bear saves each successful deployment to `bear.lock.yml` and checkpoints the plan,
then commits the lock with `[skip ci]` and pushes it. Configure Git identity,
authentication, and an existing remote branch, or use `bear apply --no-commit`
and publish the local history yourself. See [CI/CD](ci-cd.md#trusted-deployments).
Failures retain the plan for recovery; a failed push can leave a local commit
requiring a manual push, not just another apply.

## What Happens Under the Hood

1. **Detect** — Compare each artifact against its last deployed commit in the selected environment (from `bear.lock.yml`)
2. **Plan** — Decide what would deploy and write `.bear/plan.yml`; runs no commands
3. **Build + Deploy** — For each deploying artifact, in parallel across artifacts, `bear apply` runs its language steps (tests, lint, build) then its target steps (docker build, push, deploy)
4. **Lock** — Persist each completion in environment history and the plan, then publish the lock

## Next

- [Configuration](configuration.md) — All config options
- [CI/CD](ci-cd.md) — Automate with GitHub Actions, GitLab CI, or Jenkins
- [Commands](commands/index.md) — Full command reference
