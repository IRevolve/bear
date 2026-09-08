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

The environment, allowlists, and policy decisions are snapshotted in the saved plan and displayed by
plan/apply, including skip reasons. Apply does not re-evaluate policy, so changes
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

`--verbose` streams subprocess output while retaining bounded failure diagnostics.
Without it, step output is captured and failure tails are reported. Output can
contain secrets; bounded retention is not redaction.
