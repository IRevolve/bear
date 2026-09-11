# Plan/Apply Workflow

Inspired by Terraform:

1. **`bear plan dev`** — Detects changes, validates in parallel, writes `.bear/plan.yml`
2. **`bear apply`** — Reads the plan, deploys in parallel, updates lock file

The plan file is an approval checkpoint, not a portable authorization to deploy
arbitrary source. Apply it only against the intended source and state. Failed runs
retain the plan and completed checkpoints for recovery; see [Retries](#retries).

After successful CLI argument parsing and acquiring the workspace lock, planning
removes the previous plan, including when the new
attempt fails or has no changes. A stale deployment plan cannot survive a rejected
environment, invalid configuration, or failed validation.
CLI syntax errors, including a missing environment argument (`bear plan`), leave the
previous plan untouched. A provided invalid environment (such as `bear plan qa`)
starts planning and clears the stale plan before failing. Only apply after a successful plan.

Artifacts declare an allowlist such as `environments: [dev, int]` to permit deployment
only in those environments. An absent allowlist or `[]` means no deployment, with
validation retained. Use `[dev, int, prd]` to allow all three environments.

Every plan requires an explicit `bear plan dev`, `bear plan int`, or `bear plan prd`,
including validation-only plans and plans selecting unchanged or pinned artifacts.
Artifact filters follow the environment, as in `bear plan dev user-api`.
Only the positional environment argument selects policy; inherited or configured
`ENVIRONMENT` variables cannot satisfy this requirement.
Deployment is gated after dependency propagation; validation and dependent change
detection remain active. Each dependent uses its own allowlist. Pinning and forcing
cannot bypass it.

The environment, allowlists, and policy decisions are snapshotted in the saved plan
and printed by `bear plan`, including skip reasons. Apply executes that snapshot
without re-evaluating policy, so changes
to policy or the intended environment require replanning. Disabled deployments are not
executed or recorded in the lock file.
Old deployment plans lacking allowlist snapshots, source commit, or fingerprint
must be regenerated before apply.

The selected environment is injected as `$ENVIRONMENT` into all validation and deployment
steps, including validation-only plans, overriding configured or inherited values.
Its saved deployment value takes precedence over the process environment
when applying the plan.

## Source Safety

A normal plan with any permitted deployments requires clean Git source **before
validation**, across the entire repository, not just selected artifacts or the
project subdirectory. Commit or remove tracked/index and nonignored untracked
changes first. Validation-only normal plans may start dirty. Bear state
(`bear.lock.yml` and `.bear/` at any repository depth) is excluded from source checks.

Validation must leave HEAD and the tracked working-tree diff against HEAD
unchanged, including for validation-only plans. Generated nonignored untracked
files are allowed. After validation, Bear saves the full commit and a fingerprint
of tracked and nonignored untracked working files across the repository, including
file names, contents, executable modes, symlink targets, and tracked deletions.
Absolute checkout paths and timestamps are not part of that fingerprint.

Before running pending deployments, normal apply requires both HEAD and the
post-validation fingerprint to match. It does not rerun normal validation steps.
Keep the validated workspace, or reproduce its fingerprinted generated files when
moving the plan to another checkout. Ignored untracked files such as dependencies
and build outputs are **not** fingerprinted; preserve or reproducibly rebuild them
if your deployment needs them. Bear does not guarantee their immutability.

Saved artifact and validation paths are project-relative. Apply rejects absolute
paths, traversal, and existing symlinks escaping the source root. Nested project
locations, such as `examples/`, are preserved in private pinned worktrees.

### Pinned Source

`--pin` resolves the revision to a commit and validates it in a private detached
worktree. Artifact selection, policy, and step definitions come from the current
project configuration and are snapshotted; the selected revision supplies source.
Dirty files, ignored dependencies, and outputs from the caller's checkout are not
copied into that worktree.

When pinned deployments remain pending, apply creates a fresh private worktree at
the saved commit and reruns the saved validation/setup steps to rebuild their
outputs. It checks the resulting commit and fingerprint against the approved plan
before deployment. Nondeterministic nonignored outputs cause a mismatch. Ignored
dependencies/outputs remain outside the fingerprint contract even when rebuilt.
Private worktrees are cleaned up after use; cleanup failures are reported.

### Limits

Submodules and nonignored nested repositories fail closed rather than being
silently omitted. Unsupported special files and tracked paths hidden by
`skip-worktree` or `assume-unchanged` flags are also rejected. Keep source quiescent
during checks and execution: fingerprinting is not an atomic filesystem snapshot,
path checks do not prevent concurrent symlink swaps, and configured shell steps
are not sandboxed. Pinning source is not a guarantee of identical external
dependencies, toolchains, or deployment side effects.

## History And State

History and pins are scoped as `environments -> environment -> artifact`. Deploying
to `dev` does not mark that source deployed in `prd`. Dependency changes are compared
against each consumer's deployed baseline in the selected environment.

Legacy top-level `artifacts` entries are retained with a warning, but have no
environment provenance and are not used or automatically migrated into environment
history. Replan and review each environment; regenerate old saved plans rather than
patching them by hand.

Plan and lock writes use atomic file replacement, with file sync and directory
sync where supported. Windows has OS-specific rename guarantees and no directory
sync. Plans are written with mode `0600` because saved variables can contain
secrets. Secure copies and restore restrictive permissions after artifact downloads.

Plan/apply take a nonblocking advisory lock on `.bear/workspace.lock` for the project
root, then acquire a repository lock before preparing source. The repository lock
is `bear.repository.lock` in the common Git directory, resolved with
`git rev-parse --git-common-dir`. It serializes cooperating Bear operations across
sibling/nested project roots and linked worktrees sharing that directory, and is
held through Git publication and private-source cleanup. Failure to resolve the
repository is an error, not a fallback to weaker project-local locking.

Do not delete either lock file to "clear" a lock; the OS releases locks on close or
process exit, while the files persist. Git itself and other noncooperating writers
do not honor these advisory locks. Independent clones and runners do not share a
common Git directory: CI must still coordinate jobs sharing remote branches or
deployment state. Local exclusion is not a distributed deployment lock.

## Retries

Each successful artifact deployment saves its environment lock history first,
then checkpoints `completed: true` in the plan. These writes happen per completion,
not only after the whole batch succeeds. Failed runs retain the plan. On retry,
completed entries are skipped only if lock history matches the expected commit,
target, pin status, short version, and has a timestamp; a mismatch stops apply.
Remaining deployments still require the approved source checks. A fully completed
retry runs no validation or deployment commands and only finishes state publication.

Git commit/push failures return a nonzero error and retain the completed plan and
history. `--git-remote` and `--git-branch` select the push destination, including
detached CI checkouts. A failed push can leave a new local lock commit: inspect
HEAD and the remote, then **retry the push manually** as the error directs. Simply
rerunning apply will not push a local-ahead commit: the Git helper requires the
target branch tip to equal local HEAD before it creates a lock commit. See
[Apply Recovery](../commands/apply.md#git-publication).

After successful publication, apply removes the plan. With `--no-commit`, it skips
commit and push, removes the completed plan, and leaves durable local lock history
for you to publish. A plan with no deployments is removed without source execution.

There is no transaction spanning an external deployment, the lock file, and the
plan checkpoint. A crash or persistence failure after an external success can
leave uncertain status, including saved history without a completed marker.
Inspect the real deployment and reconcile state before retrying or replanning;
Bear cannot promise exactly-once deployment or roll back external effects.

## Output

The two commands print deliberately different things. **The plan is the review
artifact; apply is the execution log.** Plan states, once, everything a reviewer
needs to approve. Apply then reports what it actually did, and what failed, without
restating the approved plan back at you.

`bear plan` closes with the rule, an aligned fact block, its counted sections, and
one sentence:

```text
────────────────────────────────────────
Environment: prd
Commit:      b11f03a

deploy (2):
  - checkout-api (services/checkout-api): new artifact
  - kira-teams-adapter (services/kira/teams-adapter): new artifact

skip (1):
  - kira-mail-adapter (services/kira/mail-adapter): deployment not enabled for environment prd

Plan complete: 4 validated, 2 to deploy, 1 skipped
```

The facts are `Environment:` always, `Commit:` with the short source commit —
`Pinned:` instead when the plan was pinned — then `Artifacts:` for an artifact
filter and `Changes:` for the number of changed files. Every entry shares that one
source, so it is reported once in the header instead of under each artifact. Each
section is labelled with its outcome and entry count, its entries are sorted by
name so two runs of the same plan produce comparable summaries even though jobs
finish in any order, and each entry is a single line:
`  - <name> (<path>): <reason>`. The path is omitted when the plan recorded none,
and empty sections are omitted.

`bear apply` prints the phase heading, the job lines, and the closing sentence. A
successful run has no rule, no `Environment:` block, and no `deploy`/`skip`
sections:

```text
Deploying 2 artifacts to prd

  checkout-api:       Deploying...
  checkout-api:       Deployment complete after 15s
  kira-teams-adapter: Deployment complete after 15s

Apply complete: 2 deployed, 1 skipped in 15s
```

The environment is named in the phase heading, so it is still stated once, beside
the work. On failure apply prints the rule, `Environment: <env>`, and a red
`failed (N):` list whose entries use the failing step's error as their reason —
the one part of a summary a deployment log genuinely needs, kept unburied because
nothing else is recapped around it. Artifacts checkpointed by an earlier run are
not redeployed and are not listed; they only raise the `skipped` count.
`bear.lock.yml` and the retained plan remain the record of what is deployed.

One sentence closes each command, so a long CI log can be read from the bottom up:
`Plan complete: 4 validated, 2 to deploy, 1 skipped` for plan, and
`Apply complete: 2 deployed, 1 skipped in 15s` — or
`Apply failed: 0 deployed, 2 failed, 1 skipped in 11s` — for apply. Plan's
validation phase closes the same way, with `Validation complete: 4 artifacts in 3s`.
A publishing apply adds a dimmed `Lock file committed with [skip ci]` line after
its sentence.

Progress is reported while the work runs, under a plain phase heading such as
`Deploying 2 artifacts to prd`. On an interactive terminal Bear draws an animated
progress bar with a per-task spinner and timers. Without a terminal it prints one
plain line per job on every status change, with job names padded into a common
column, a `Still ...` line every 10 seconds for each running job, and the
remaining backlog appended to the last running job's heartbeat as
`(1 job queued)`. Captured failure output is indented four spaces under the job
line that reported it. See [Live Output](../ci-cd.md#live-output).

`--verbose` streams subprocess output while retaining bounded failure diagnostics,
and selects the plain, line-per-status display even on a terminal. Without it,
step output is captured and failure tails are reported. Output can contain secrets;
bounded retention is not redaction.
