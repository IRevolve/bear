# bear apply

Execute the plan from `.bear/plan.yml`. Deploy in parallel, update lock file.

```bash
bear apply                     # Execute plan
bear apply --no-commit         # Don't auto-commit lock file
bear apply --concurrency 3     # Limit parallelism
```

## Flags

| Flag | Description |
|------|-------------|
| `--no-commit` | Skip auto-commit of lock file |
| `--concurrency <n>` | Max parallel deployments (default: `10`) |

## Flow

Read plan → Deploy → Update `bear.lock.yml` → Commit `[skip ci]` → Remove plan
