# Plan/Apply Workflow

Inspired by Terraform:

1. **`bear plan dev`** — Detects changes by comparing against `bear.lock.yml`,
   writes `.bear/plan.yml`. Runs no commands.
2. **`bear apply`** — Reads the plan; for every deploying artifact, runs the
   language's build steps then the target's deploy steps, in parallel across
   artifacts; updates lock file.

Plan is a pure decision. It never runs a language's or a target's steps, for a
normal plan or a `--pin` plan, so it is fast and safe to run as often as you like
— it never risks executing untrusted build or deploy commands just to show a
diff. Apply is the only command that executes anything, and it now does the whole
job for a deploying artifact in one place: build **and** deploy, back to back,
every time, whether or not the deployment is pinned.

### What Runs Where

| Command | Language steps (`languages.<lang>.steps`) | Target steps (deploy) |
|---------|---------------------------------------------|------------------------|
| `bear plan` (normal or `--pin`) | Never | Never |
| `bear apply` | For every deploying artifact, always first | For every deploying artifact, always second |
| `bear validate` | For every selected artifact/library, on demand | Never (validate has no target/deployment concept) |

Because plan never builds anything, it does not prove the code builds or passes
tests — it is a diff/preview of what would deploy, not a promise that it can.
[`bear validate`](../commands/validate.md) is where that assurance lives: it runs
the same `languages.<lang>.steps` apply's build phase runs, with no environment,
no change detection, no plan file and no Git, so a shallow pull-request checkout
with no deployment credentials is enough. Run it in the merge request for fast
feedback, and again — implicitly, as apply's first phase — right before every
deployment. The full recommended chain is
`doctor` → `validate` → `plan` → `apply`.

The plan file is an approval checkpoint, not a portable authorization to deploy
arbitrary source. Apply it only against the intended source and state. Failed runs
retain the plan and completed checkpoints for recovery; see [Retries](#retries).

After successful CLI argument parsing and acquiring the workspace lock, planning
removes the previous plan, including when the new
attempt fails or has no changes. A stale deployment plan cannot survive a rejected
environment or invalid configuration.
CLI syntax errors, including a missing environment argument (`bear plan`), leave the
previous plan untouched. A rejected environment — malformed, or well-formed but not
declared, such as `bear plan qa` in a project declaring `dev`, `int`, `prd` —
starts planning and clears the stale plan before failing. Only apply after a successful plan.

Artifacts declare an allowlist such as `environments: [dev, int]` to permit deployment
only in those environments. An absent allowlist or `[]` means no deployment; change
detection and dependency propagation still run. Repeat the project's whole list to
allow all of them.

Both the plan argument and every allowlist entry are checked against the
environments the project
[declares in `bear.config.yml`](../configuration.md#deployment-environments). There
is no fixed set: `[dev, int, prd]` is what `bear init` writes by default, and
`[preprd, prd]` or a single `[prd]` is equally valid. An undeclared plan argument
fails with `unknown environment "production": bear.config.yml declares dev, int,
prd`, and an undeclared allowlist entry fails with `artifact "api" allows undeclared
environment "preprd"; bear.config.yml declares dev, int, prd`. Declaration order is
preserved in those messages and everywhere else, and means nothing beyond ordering:
Bear does not require `int` before `prd`.

Every plan requires an explicit environment as its first positional argument, as in
`bear plan dev` or `bear plan preprd`,
including a plan that ends up with nothing to deploy and one selecting unchanged or
pinned artifacts. Artifact filters follow the environment, as in `bear plan dev
user-api`. Only the positional environment argument selects policy; inherited or
configured `ENVIRONMENT` variables cannot satisfy this requirement.
Deployment is gated after dependency propagation; change detection and dependent
propagation remain active regardless. Each dependent uses its own allowlist.
Pinning and forcing cannot bypass it.

The environment, allowlists, and policy decisions are snapshotted in the saved plan
and printed by `bear plan`, including skip reasons. Apply executes that snapshot
without re-evaluating policy — it never reads `bear.config.yml` at all — so
changes
to policy or the intended environment require replanning, while no change to the
project's declared environments can make apply reject an approved plan on policy
grounds.
Disabled deployments are not
executed or recorded in the lock file.
Old deployment plans lacking allowlist snapshots, source commit, or fingerprint
must be regenerated before apply.

That is a statement about policy, not about files. `bear.config.yml` is ordinary
tracked source: editing it in the approved checkout changes the
[source fingerprint](#source-safety) and apply refuses the plan, exactly as it would
for any other edited file. Apply an in-flight plan before changing configuration, or
replan afterwards.

The selected environment is injected as `$ENVIRONMENT` into every step apply will
later run for a deploying artifact, overriding configured or inherited values.
Plan itself runs no steps, so nothing consumes the variable at plan time; it is
saved in the plan so apply's build and deploy steps see the approved selection.
Its saved deployment value takes precedence over the process environment
when applying the plan.

## Source Safety

A normal plan with any permitted deployments requires clean Git source before it
runs, across the entire repository, not just selected artifacts or the project
subdirectory. Commit or remove tracked/index and nonignored untracked changes
first. A plan with nothing to deploy may start dirty. Bear state
(`bear.lock.yml` and `.bear/` at any repository depth) is excluded from source checks.

Plan itself never modifies HEAD or the tracked working-tree diff — it runs no
commands at all — so its fingerprint is simply of the source as it already sits on
disk. Bear saves the full commit and a fingerprint of tracked and nonignored
untracked working files across the repository, including file names, contents,
executable modes, symlink targets, and tracked deletions. Absolute checkout paths
and timestamps are not part of that fingerprint.

Before running pending deployments, apply requires both HEAD and the saved
fingerprint to match — for a normal plan, against the caller's checkout; for a
pinned plan, against a fresh private worktree at the saved commit (see
[Pinned Source](#pinned-source) below). This is the only assurance that what
apply is about to build is the source that was approved: unlike the old
plan-then-apply split, apply never assumes plan already ran build steps whose
output needs to survive until now, because apply always builds fresh, itself,
immediately before it deploys. Moving a plan to another checkout therefore only
requires that checkout's tracked commit and nonignored untracked files to match
exactly — a plain clean checkout of the same commit satisfies this trivially —
not that it already contain generated build artifacts. Ignored untracked files
such as dependencies and prior build outputs are **not** fingerprinted regardless;
provision them the same way you would for any fresh build.

Saved artifact paths are project-relative. Apply rejects absolute
paths, traversal, and existing symlinks escaping the source root. Nested project
locations, such as `examples/`, are preserved in private pinned worktrees.

### Pinned Source

`--pin` resolves the revision to a commit and takes its fingerprint in a private
detached worktree — it does not build, test, or otherwise validate that revision.
Artifact selection, policy, and step definitions come from the current project
configuration and are snapshotted; the selected revision supplies only source.
Dirty files, ignored dependencies, and outputs from the caller's checkout are not
copied into that worktree.

When pinned deployments remain pending, apply creates a fresh private worktree at
the saved commit and checks its commit and fingerprint against the approved plan
**before** running any step. Only then does it run the language's build steps and
the target's deploy steps, exactly as it would for a normal deployment — pinning
changes where the source comes from, not what runs against it, and there is no
separate "replay validation" phase to keep in sync with normal apply's behavior
anymore. Because the fingerprint check happens against the fresh worktree before
any step runs, ignored dependencies and outputs the build steps later produce are
never part of it either way. Private worktrees are cleaned up after use; cleanup
failures are reported.

A pipeline that exposes `--pin` and `--force` as build parameters to freeze and
unfreeze deployables on demand is a CI-side pattern built from these two flags,
not a separate Bear feature; see [Freeze & Unfreeze (Jenkins)](freeze-unfreeze.md).

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

The keys are whatever environments have been deployed to, which is not necessarily
what the project declares today. History under a retired name is kept and is neither
an error nor a warning; `bear list` and `bear list --tree` report only the declared
set, so retiring an environment hides its history from those views without deleting
it. `bear apply` still writes history under the environment its plan approved,
declared or not.

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
Remaining deployments still require the approved source checks, and still run
their full build-then-deploy step sequence from scratch: apply checkpoints
per-artifact completion, not per-step progress within an artifact. A fully
completed retry runs no build, deploy, or source-preparation commands and only
finishes state publication.

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
needs to approve — computed instantly, since it ran nothing to get there. Apply
then reports what it actually built and deployed, and what failed, without
restating the approved plan back at you.

`bear plan` goes straight from its branding header to the rule, an aligned fact
block, its counted sections, and one sentence — there is no progress phase in
between, because plan runs no commands:

```text
Bear Plan
─────────

────────────────────────────────────────
Environment: int
Commit:      cadc967

deploy (1):
  - api (services/api): new artifact

skip (1):
  - backoffice (services/backoffice): deployment not enabled for environment int

Plan complete: 2 changed, 1 to deploy, 1 skipped
```

The facts are `Environment:` always, `Commit:` with the short source commit —
`Pinned:` instead when the plan was pinned — then `Artifacts:` for an artifact
filter and `Changes:` for the number of changed files. Every entry shares that one
source, so it is reported once in the header instead of under each artifact. Each
section is labelled with its outcome and entry count, its entries are sorted by
name so two runs of the same plan produce comparable summaries, and each entry is
a single line: `  - <name> (<path>): <reason>`. The path is omitted when the plan
recorded none, and empty sections are omitted.

Two sections, not three: `deploy` lists what apply will run; `skip` lists every
other non-library artifact affected by a source change and why it did not
deploy — a would-be deployment blocked by policy (`deployment not enabled for
environment prd`, `pinned (use --force to override)`, and so on). A library
never appears in either section, changed or not: libraries are never
deployable, so listing one would read as a missed deployment it never was. A
library's own change surfaces only indirectly, through the `dependency
'<name>' changed` reason on whatever depends on it. Every non-library artifact
affected by a source change appears in exactly one of these two sections,
never both and never neither.


`bear apply` prints its own branding header, then the phase heading, the job
lines, and the closing sentence. A successful run has no rule, no `Environment:`
block, and no `deploy`/`skip` sections:

```text
Bear Apply
──────────

Deploying 1 artifact to prd

  api: Deploying...
  api: Deploying... [1/4 Test]
  api: Deploying... [2/4 Build]
  api: Deploying... [3/4 Build image]
  api: Deploying... [4/4 Push image]
  api: Deployment complete after 0s

Apply complete: 1 deployed, 1 skipped in 0s
```

The environment is named in the phase heading, so it is still stated once, beside
the work. `api`'s step counter runs `[1/4]` through `[4/4]` because its language
defines two steps and its target defines two more: apply numbers the language's
build steps and the target's deploy steps as one continuous sequence per
artifact, build first, for every deployment — pinned or not, there is exactly one
execution path. On failure apply prints the rule, `Environment: <env>`, and a red
`failed (N):` list whose entries use the failing step's error as their reason,
whether that step was a build step or a deploy step:

```text
────────────────────────────────────────
Environment: prd

failed (1):
  - api (services/api): Test: exit status 1

Apply failed: 0 deployed, 1 failed, 1 skipped in 0s
```

That is the one part of a summary a deployment log genuinely needs, kept unburied
because nothing else is recapped around it. Artifacts checkpointed by an earlier
run are not redeployed and are not listed; they only raise the `skipped` count.
`bear.lock.yml` and the retained plan remain the record of what is deployed.

One sentence closes each command, so a long CI log can be read from the bottom up:
`Plan complete: 2 changed, 1 to deploy, 1 skipped` for plan, and
`Apply complete: 1 deployed, 1 skipped in 0s` — or
`Apply failed: 0 deployed, 1 failed, 1 skipped in 0s` — for apply. Plan's `changed`
count is not "validated": it is however many non-library artifacts a source
change affected, whether or not they ended up in `deploy`; libraries are never
part of this count, just as they are never part of `deploy` or `skip`. An
unaffected artifact (`no changes detected`) is not "changed" either, so `skip`
can hold more than `changed` minus `to deploy` — `changed` only ever equals
`to deploy` plus the skips an actual change caused, such as a policy block or
a pin. A publishing apply adds
a dimmed `Lock file committed with [skip ci]` line after its sentence.

Progress is reported while the work runs, under a plain phase heading such as
`Deploying 1 artifact to prd`. On an interactive terminal Bear draws an animated
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
