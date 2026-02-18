# Commands

## Overview

| Command | Description |
|---------|-------------|
| `bear init` | Initialize a new project |
| `bear plan [artifacts...]` | Detect changes, validate, create deployment plan |
| `bear apply` | Execute the deployment plan |
| `bear check` | Validate config and dependencies |
| `bear list` | List all artifacts |
| `bear preset list\|show\|update` | Manage presets |

## Global Flags

| Flag | Description |
|------|-------------|
| `-d, --dir <path>` | Project directory (default: `.`) |
| `-f, --force` | Force operation, ignore pins |
| `-v, --verbose` | Show full command output |

---

## bear init

Create a `bear.config.yml` with auto-detected languages.

```bash
bear init                              # Current directory
bear init -d ./my-project              # Different directory
bear init --lang go,node --target docker   # With presets
bear init --force                      # Overwrite existing
```

| Flag | Description |
|------|-------------|
| `--lang <langs>` | Language presets (comma-separated) |
| `--target <targets>` | Target presets (comma-separated) |
| `--force` | Overwrite existing config |

---

## bear plan

Detect changes, run validation in parallel, write plan to `.bear/plan.yml`.

```bash
bear plan                      # All changed artifacts
bear plan user-api order-api   # Specific artifacts
bear plan --concurrency 5      # Limit parallelism
bear plan user-api --pin abc1234   # Pin to commit
```

| Flag | Description |
|------|-------------|
| `--concurrency <n>` | Max parallel validations (default: `10`) |
| `--pin <commit>` | Pin artifact to specific commit |

### Change Reasons

| Reason | Description |
|--------|-------------|
| `files changed` | Uncommitted changes in artifact directory |
| `new commits` | Commits since last deploy |
| `new artifact` | Never deployed before |
| `dependency changed` | A dependency changed |

---

## bear apply

Execute the plan from `.bear/plan.yml`. Deploy in parallel, update lock file.

```bash
bear apply                     # Execute plan
bear apply --no-commit         # Don't auto-commit lock file
bear apply --concurrency 3     # Limit parallelism
```

| Flag | Description |
|------|-------------|
| `--no-commit` | Skip auto-commit of lock file |
| `--concurrency <n>` | Max parallel deployments (default: `10`) |

Flow: Read plan → Deploy → Update `bear.lock.yml` → Commit `[skip ci]` → Remove plan

---

## bear check

Validate config, languages, targets, artifacts, dependencies, circular dependency detection.

```bash
bear check
bear check -d ./my-project
```

---

## bear list

List discovered artifacts. Use `--tree` for dependency visualization.

```bash
bear list                      # List all
bear list --tree               # Dependency tree
bear list --tree user-api      # Tree for specific artifact
```

---

## bear preset

Manage community presets from [bear-presets](https://github.com/irevolve/bear-presets).

```bash
bear preset list               # Show all presets
bear preset show language go   # Language details
bear preset show target docker # Target details
bear preset update             # Refresh cache
```

Presets are cached in `~/.bear/presets/` for 24 hours.
