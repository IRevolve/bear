# Lock File

`bear.lock.yml` is auto-managed deployment history and should be committed.
Successful deployments update history; policy-disabled deployments do not.

## Environment History

History and pins are scoped by environment, then artifact:

```yaml
environments:
  dev:
    user-api:
      commit: abc1234567890
      timestamp: "2026-01-04T10:00:00Z"
      version: abc1234
      target: cloudrun
```

Legacy top-level `artifacts` entries are retained with a warning, but have no
environment provenance. They are ignored for deployment decisions and **not
automatically mapped** into `dev`, `int`, or `prd`. Replan and review each
environment; regenerate legacy saved plans too.

The old shared artifact history could let one environment's deployment affect
another's change detection. Environment-scoped history avoids that: deployment to
`dev` does not advance `int` or `prd`, and pins apply only in their own environment.
Dependency changes use each consumer's deployed baseline in that environment.

Auto-commit uses `[skip ci]`; ensure any custom skip rule checks both bot identity
and an exclusively lock-only diff. With `--no-commit`, CI must persist the updated
history itself. Each successful deployment atomically saves its history, then
checkpoints completion in the saved plan. Retry skips a completed artifact only
when that history matches. Git failure returns nonzero and retains the plan and
history. A failed push can leave a local commit requiring manual publication.
See [Retries](plan-apply.md#retries) for recovery and the limits of atomic local
writes when external deployments have already succeeded.
