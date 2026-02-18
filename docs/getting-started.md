# Getting Started

## Install

```bash
go install github.com/irevolve/bear@latest
```

Or build from source:

```bash
git clone https://github.com/irevolve/bear.git && cd bear && go build -o bear .
```

Verify: `bear --version`

**Requirements:** Git (for change detection). Go 1.21+ only if building from source.

## First Project

### 1. Initialize

```bash
cd my-monorepo
bear init
```

Creates `bear.config.yml` with auto-detected languages.

### 2. Configure

```yaml title="bear.config.yml"
name: my-platform

use:
  languages: [go, node]
  targets: [docker, cloudrun]
```

!!! tip "Presets"
    Run `bear preset list` to see all available languages and targets.

### 3. Add Artifacts

Each deployable service gets a `bear.artifact.yml`:

```yaml title="services/api/bear.artifact.yml"
name: api
target: cloudrun
depends: [shared-lib]

vars:
  PROJECT: my-gcp-project
```

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

```bash
bear plan     # detect changes, validate, create plan
bear apply    # execute the plan
```

Done. Bear updates `bear.lock.yml` and auto-commits it with `[skip ci]`.

## What Happens Under the Hood

1. **Detect** — Compare each artifact against its last deployed commit (from `bear.lock.yml`)
2. **Validate** — Run language steps (tests, lint, build) in parallel
3. **Deploy** — Run target steps (docker build, push, deploy) in parallel
4. **Lock** — Update `bear.lock.yml` with new commit hashes

## Next

- [Configuration](configuration.md) — All config options
- [CI/CD](ci-cd.md) — Automate with GitHub Actions, GitLab CI, or Jenkins
- [Commands](commands.md) — Full command reference
