# Commands

Bear provides a simple set of commands for managing your monorepo builds.

## Command Overview

| Command | Description |
|---------|-------------|
| [`bear init`](init.md) | Initialize a new Bear project |
| [`bear list`](list.md) | List all discovered artifacts |
| [`bear plan`](plan.md) | Show planned validations and deployments |
| [`bear apply`](apply.md) | Execute the plan |
| [`bear check`](check.md) | Validate configuration |
| [`bear preset`](preset.md) | Manage presets |

## Global Flags

These flags work with all commands:

| Flag | Description |
|------|-------------|
| `-d, --dir <path>` | Path to project directory (default: `.`) |
| `-f, --force` | Force operation, ignore pinned artifacts |
| `-h, --help` | Show help |

## Workflow

The typical Bear workflow:

```bash
# 1. Check your setup
bear check

# 2. See what would be built
bear plan

# 3. Execute the build
bear apply
```

## Targeting Specific Artifacts

Most commands accept artifact names as arguments:

```bash
# Plan only specific artifacts
bear plan user-api order-api

# Apply only specific artifacts
bear apply user-api
```
