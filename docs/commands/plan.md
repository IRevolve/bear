# bear plan

Detect changes, validate artifacts, and create a deployment plan.

## Usage

```bash
bear plan [artifacts...] [flags]
```

## Description

Analyzes changes in your repository, runs validation steps for changed artifacts **in parallel**, and writes a validated deployment plan to `.bear/plan.toml`.

If any validation fails, no plan file is written and the command exits with code 1.

After a successful plan, run `bear apply` to execute the deployments.

## Arguments

| Argument | Description |
|----------|-------------|
| `artifacts` | Optional. Specific artifacts to plan. If omitted, plans all changed artifacts. |

## Flags

| Flag | Description |
|------|-------------|
| `--concurrency <n>` | Maximum parallel validations (default: `10`) |
| `--pin <commit>` | Pin artifact(s) to a specific commit |
| `-v, --verbose` | Show full command output during validation |

## Examples

```bash
# Plan all changed artifacts
bear plan

# Plan specific artifacts
bear plan user-api order-api

# Pin artifact to a specific commit
bear plan user-api --pin abc1234

# Limit parallel validations
bear plan --concurrency 5

# Plan in a different directory
bear plan -d ./my-project
```

## Output

```
  BEAR — Plan
──────────────────────────────────────

  Detecting changes...

  Validating 3 artifacts (concurrency: 10)

  ✓ user-api          validated in 8.2s
  ✓ order-api         validated in 5.1s
  ✓ dashboard         validated in 3.4s

──────────────────────────────────────
  Deploy
──────────────────────────────────────

  user-api            docker     files changed
  order-api           cloudrun   dependency changed
  dashboard           s3         new artifact

──────────────────────────────────────
  Skipped
──────────────────────────────────────

  email-worker        unchanged
  pinned-service      pinned

──────────────────────────────────────
  Summary: 3 to deploy, 1 unchanged, 1 pinned

  Plan written to .bear/plan.toml
  Run 'bear apply' to execute this plan.
```

## Plan File

The plan is saved to `.bear/plan.toml` and contains:

- Validated artifacts with their deploy steps
- Skipped artifacts and reasons
- The commit at which the plan was created

The `.bear/` directory should be added to `.gitignore` (done automatically by `bear init`).

## Change Reasons

| Reason | Description |
|--------|-------------|
| `files changed` | Uncommitted changes in artifact directory |
| `new commits` | Commits since last deploy |
| `new artifact` | Never deployed before |
| `dependency changed` | A dependency (library) changed |

## See Also

- [bear apply](apply.md)
- [Change Detection](../concepts/change-detection.md)
- [Dependencies](../concepts/dependencies.md)
