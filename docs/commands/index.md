# Commands

Bear provides a simple set of commands for managing your monorepo builds.

## Command Overview

| Command | Description |
|---------|-------------|
| [`bear init`](init.md) | Initialize a new Bear project |
| [`bear list`](list.md) | List all discovered artifacts |
| [`bear plan`](plan.md) | Detect changes, validate, and create deployment plan |
| [`bear apply`](apply.md) | Execute the deployment plan |
| [`bear check`](check.md) | Validate configuration |
| [`bear preset`](preset.md) | Manage presets |

## Global Flags

These flags work with all commands:

| Flag | Description |
|------|-------------|
| `-d, --dir <path>` | Path to project directory (default: `.`) |
| `-f, --force` | Force operation, ignore pinned artifacts |
| `-v, --verbose` | Show full command output |
| `-h, --help` | Show help |

## Workflow

The typical Bear workflow:

```bash
# 1. Check your setup
bear check

# 2. Detect changes, validate, and create a plan file
bear plan

# 3. Execute the deployment plan
bear apply
```

`bear plan` writes a validated plan to `.bear/plan.toml`. `bear apply` reads this file and executes only the deployments — no re-validation.

## Targeting Specific Artifacts

`plan` accepts artifact names as arguments:

```bash
# Plan only specific artifacts
bear plan user-api order-api
```

`apply` always reads from the plan file — no artifact arguments needed.
