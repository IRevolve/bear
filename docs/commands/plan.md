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

Ten parallel validations suit a build agent with spare cores. Lower it when steps
contend for one shared resource — a Docker daemon, a test database, a rate-limited
registry — or when interleaved job output is hard to read. `bear apply` has its own
`--concurrency` with the same default; see [its guidance](apply.md#flags) before
reusing this number for production deployments.

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

## Output

Validation reports live under a plain phase heading, then the command ends with
the run's facts, its counted `deploy`/`skip` lists, and one closing sentence. That
summary is the review artifact: it is what a human approves, and what `bear apply`
then executes without reprinting.

```text
Bear Plan
─────────

Validating 4 artifacts

  shared:             Validating...
  shared:             Validating... [1/2 Test]
  kira-mail-adapter:  Validating...
  kira-mail-adapter:  Validating... [1/2 Test]
  kira-teams-adapter: Validating...
  kira-teams-adapter: Validating... [1/2 Test]
  checkout-api:       Validating...
  checkout-api:       Validating... [1/2 Test]
  shared:             Validating... [2/2 Build]
  checkout-api:       Validating... [2/2 Build]
  kira-teams-adapter: Validating... [2/2 Build]
  kira-mail-adapter:  Validating... [2/2 Build]
  shared:             Validation complete after 3s
  checkout-api:       Validation complete after 3s
  kira-mail-adapter:  Validation complete after 3s
  kira-teams-adapter: Validation complete after 3s

Validation complete: 4 artifacts in 3s

────────────────────────────────────────
Environment: prd
Commit:      b11f03a

deploy (2):
  - checkout-api (services/checkout-api): new artifact
  - kira-teams-adapter (services/kira/teams-adapter): new artifact

skip (1):
  - kira-mail-adapter (services/kira/mail-adapter): deployment not enabled for environment prd

Plan complete: 4 validated, 2 to deploy, 1 skipped

Run 'bear apply' to execute this plan.
```

The block after the rule states the run's facts on one aligned column.
`Environment:` is always present. `Commit:` carries the short source commit the
plan was built from, replaced by `Pinned:` when `--pin` selected the revision.
`Artifacts:` follows when artifact filters were given, and `Changes:` reports the
number of changed files when there are any. Every entry in the plan comes from that
one source, so it is stated once in the header rather than repeated per artifact.

Each section is labelled with its outcome and entry count (`deploy (2):`,
`skip (1):`), and its entries are sorted by name so repeated runs of the same plan
produce comparable summaries even though validations finish in any order. An entry
is a single line, `  - <name> (<path>): <reason>`; the parenthesised path is omitted
when the plan recorded none. Empty sections are omitted. A `deploy` entry's reason
is the artifact's [change reason](#change-reasons); a `skip` entry's reason is the
recorded skip, such as `no changes detected`,
`pinned (use --force to override)`, or `deployment not enabled for environment prd`.
Neither entry repeats the commit or names a target: the commit is the header's, and
the target is configuration you can read in `bear.artifact.yml`.

The validation phase closes with `Validation complete: <n> artifacts in <time>`,
and the command closes with `Plan complete: 4 validated, 2 to deploy, 1 skipped`;
the skipped count is listed only when there are skips. The
`Run 'bear apply' to execute this plan.` hint follows only when the plan has
something to deploy, so a validation-only plan ends at the sentence. A failed
validation instead prints the bounded output tail indented four spaces under that
job's `Validation failed` line, then `Validation failed for: <names>` and a hint to
fix the errors and replan.

A plan with nothing to validate or deploy prints `No changes detected. Nothing to
plan.` (or `No artifacts found matching: [...]` for an unmatched filter) followed by
the rule, the `Environment:` line, and any `skip` list. That path reports no
`Commit:` fact and no closing sentence, because nothing was validated.

`bear apply` does not reprint this summary; it logs the deployments it runs and
lists only failures. See [apply output](apply.md#output).

Validation progress uses the same non-terminal reporting as apply: one line per
status change with job names padded into a common column, a `Still validating...`
line every 10 seconds per running job, and the remaining backlog appended to the
last running job's heartbeat as `(1 job queued)`. Interactive terminals draw an
animated progress bar instead. See [Live Output](../ci-cd.md#live-output).
