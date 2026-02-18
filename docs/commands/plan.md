# bear plan

Detect changes, run validation in parallel, write plan to `.bear/plan.yml`.

```bash
bear plan                      # All changed artifacts
bear plan user-api order-api   # Specific artifacts
bear plan --concurrency 5      # Limit parallelism
bear plan user-api --pin abc1234   # Pin to commit
```

## Flags

| Flag | Description |
|------|-------------|
| `--concurrency <n>` | Max parallel validations (default: `10`) |
| `--pin <commit>` | Pin artifact to specific commit |

## Change Reasons

| Reason | Description |
|--------|-------------|
| `files changed` | Uncommitted changes in artifact directory |
| `new commits` | Commits since last deploy |
| `new artifact` | Never deployed before |
| `dependency changed` | A dependency changed |
