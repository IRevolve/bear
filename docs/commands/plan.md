# bear plan

Detect changes, run validation in parallel, write plan to `.bear/plan.yml`.

```bash
bear plan <environment> [artifacts...]
```

```bash
bear plan dev                         # All changed artifacts
bear plan dev user-api order-api      # Specific artifacts
bear plan dev --concurrency 5         # Limit parallelism
bear plan dev user-api --pin abc1234  # Pin to commit
bear plan dev user-api --force        # Ignore pins
bear plan int                         # Integration deployment policy
bear plan dev                         # Also required for validation-only when all selected allowlists are absent/empty
```

## Arguments

| Argument | Description |
|----------|-------------|
| `<environment>` | Required first positional argument: `dev`, `int`, or `prd`, including for validation-only plans |
| `[artifacts...]` | Optional artifact filters, after the environment; omit to consider all artifacts |

The environment is positional only; there is no named flag or alias for it.

## Flags

| Flag | Description |
|------|-------------|
| `--concurrency <n>` | Max parallel validations (default: `10`) |
| `--pin <commit>` | Pin artifact to specific commit |
| `--force` | Ignore pins, but not environment deployment policy |
| `--verbose` | Stream subprocess output while retaining bounded failure diagnostics |

## Environment Policy

In `bear.artifact.yml`, use `environments: [dev, int]` to allow deployment only in
those environments. An absent allowlist or `environments: []` means no deployment
in any environment, while retaining validation. Use `[dev, int, prd]` to allow all three.

Every plan requires the environment argument, including validation-only plans and
plans selecting unchanged or pinned artifacts. Inherited or configured `ENVIRONMENT`
variables cannot select policy or satisfy this requirement. Bear does not infer the
environment from job/process variables, branches, or CI metadata.
Invalid environment names are rejected. Selecting an environment does not enable
deployment for artifacts with absent or empty allowlists.

The selection is passed as `$ENVIRONMENT` to all validation and deployment steps,
overriding any configured or inherited value. Deployment variables preserve the
selection in the saved plan, so apply uses the approved environment. Validation-only
plans also inject the selected value. `ENVIRONMENT` is output from selection, not an input.

Only deployment is disabled. Validation and dependency propagation still run,
and dependents use their own policies. Direct changes, dependency-triggered
deployments, `--pin`, and `--force` all respect the gate.

The saved plan includes the selected environment, artifact allowlists, and visible
skip reasons for blocked deployments. Disabled deployments are not executable
entries and will not be recorded in `bear.lock.yml` by apply. The plan snapshots
the policy decision: apply displays and executes that snapshot without checking
the current policy. Replan after changing policy or the intended environment.
Old saved plans lacking allowlist snapshots must be regenerated; they cannot be applied.

History and pins are scoped to the selected environment. Legacy artifact-only
history is retained but ignored, not automatically mapped to an environment.
Replan and review each environment after upgrading.

## Source Requirements

Normal plans with any deployments require a clean Git repository before validation;
validation-only plans may start dirty. Bear state is excluded. Validation must not
change HEAD or the tracked working-tree diff, even for validation-only plans.
Generated nonignored untracked files are allowed and included in the saved
post-validation fingerprint. Ignored untracked dependencies and outputs are excluded
and are not guaranteed immutable.

Plans save the full source commit, fingerprint, and project-relative paths. Normal
apply verifies the source without rerunning validations. `--pin` validates the
resolved commit in a private worktree; apply rebuilds it by rerunning saved
validation/setup steps, then checks the fingerprint before pending deployments.
Current project configuration supplies policy and steps, including for pins.
See [Source Safety](../concepts/plan-apply.md#source-safety) for relocation and limits.

## Saved Plan Safety

After successful CLI argument parsing and acquiring the workspace lock, planning
removes the previous `.bear/plan.yml` before loading
configuration. Failure to remove it stops planning. Invalid configuration,
provided but invalid environments, empty plans, and failed validation therefore cannot
leave an old executable plan behind. A successful validation-only plan can contain
skips and zero deployments; apply reports that there is nothing to deploy.
CLI syntax errors, including a missing environment argument (`bear plan`), do not
start planning and leave the existing plan untouched. A provided invalid environment
(for example, `bear plan qa`) starts planning and clears the stale plan before failing.
Only run apply after a successful plan.

The plan is atomically written with mode `0600`; it may contain secrets. Protect
copies and CI artifacts. Old deployment plans missing source commit, fingerprint,
or deployment-policy snapshots must be regenerated.

## Change Reasons

| Reason | Description |
|--------|-------------|
| `files changed` | Artifact files differ from its deployed baseline; dirty source is permitted only for validation-only normal plans |
| `new artifact` | No history for this artifact in the selected environment |
| `dependency '<name>' changed` | A transitive dependency differs from the consumer's deployed baseline |
| `pinned (forced deployment)` | `--force` selects an artifact pinned in this environment |
| Environment policy skip | The allowlist is absent, empty, or excludes the selected environment; validation is retained |
