# bear apply

Execute the deployment plan.

## Usage

```bash
bear apply [flags]
```

## Description

Reads the plan from `.bear/plan.toml` (created by `bear plan`) and executes the deployments in parallel. No validation is performed — only deploy steps run.

After successful deployment, the lock file is updated and automatically committed with `[skip ci]`. Use `--no-commit` to disable auto-commit.

The plan file is removed after execution.

Requires a plan file — run `bear plan` first.

## Flags

| Flag | Description |
|------|-------------|
| `--no-commit` | Skip automatic commit of the lock file |
| `--concurrency <n>` | Maximum parallel deployments (default: `10`) |
| `-v, --verbose` | Show full command output during deployment |
| `-f, --force` | Force operation, ignore pinned artifacts |

## Examples

```bash
# Plan and apply
bear plan && bear apply

# Apply existing plan
bear apply

# Apply without committing lock file
bear apply --no-commit

# Limit parallel deployments
bear apply --concurrency 5
```

## Execution Flow

1. **Read plan** — Load `.bear/plan.toml`
2. **Deploy** — Run target deploy steps in parallel for each artifact
3. **Update lock** — Write new commit hashes to `bear.lock.toml`
4. **Commit** — Auto-commit lock file with `[skip ci]` (unless `--no-commit`)
5. **Cleanup** — Remove `.bear/plan.toml`

## Output

```
  BEAR — Apply
──────────────────────────────────────

  Deploying 2 artifacts (concurrency: 10)

  ✓ user-api          deployed in 45.2s
  ✓ order-api         deployed in 32.1s

──────────────────────────────────────
  Summary: 2 deployed, 0 failed

  Lock file committed: chore(bear): update lock file [skip ci]
```

## See Also

- [bear plan](plan.md)
- [Pinning](../concepts/pinning.md)
- [Lock File](../concepts/lock-file.md)
