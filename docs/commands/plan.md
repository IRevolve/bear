# bear plan

Detect changes and decide what would deploy, then write `.bear/plan.yml`. Plan
runs no commands: no language steps, no target steps, for a normal plan or a
`--pin` plan. It only compares the current source against `bear.lock.yml` per
environment and records a decision. See [Plan/Apply Workflow](../concepts/plan-apply.md)
for the full "what runs where" picture.

```bash
bear plan <environment> [artifacts...]
```

```bash
bear plan dev                         # All changed artifacts
bear plan dev user-api order-api      # Specific artifacts
bear plan dev user-api --pin abc1234  # Pin to commit
bear plan dev user-api --force        # Ignore pins
bear plan preprd                      # Any environment bear.config.yml declares
bear plan dev                         # Required even when nothing will deploy
```

## Arguments

| Argument | Description |
|----------|-------------|
| `<environment>` | Required first positional argument: an environment declared in `bear.config.yml` |
| `[artifacts...]` | Optional artifact filters, after the environment; omit to consider all artifacts |

The environment is positional only; there is no named flag or alias for it.
Every plan requires it, even one that ends up with nothing to deploy.

## Flags

| Flag | Description |
|------|-------------|
| `--pin <commit>` | Pin artifact to specific commit |
| `--force` | Ignore pins, but not environment deployment policy |

`plan` has no `--concurrency` flag: it never ran anything in parallel after the
plan/apply redesign, so the flag was removed rather than kept as a silent
no-op. `-v/--verbose` is still accepted — it is the global root flag documented
in [Global Flags](index.md#global-flags) — but plan produces no subprocess output to stream,
so it has no visible effect here. It still matters for
[`bear apply`](apply.md#flags), which is where steps actually run.

## Environment Policy

The environment argument must name one of the environments the project declares in
[`bear.config.yml`](../configuration.md#deployment-environments). There is no
built-in set: `[dev, int, prd]` is only `bear init`'s default, and `[preprd, prd]`
or `[prd]` works the same way. An undeclared value is rejected, and the message
lists what the project actually declares, in declaration order:

```text
Error: error creating plan: unknown environment "production": bear.config.yml declares dev, int, prd
```

A syntactically invalid value fails first, before the declaration is consulted:
`bear plan PRD` gives `invalid environment name "PRD": use 1 to 32 characters
matching [a-z][a-z0-9-]*`. Run `bear doctor` to print the declared list.

Declaration order carries no promotion semantics. Bear will plan `prd` on source
that was never planned for `int`; enforce a promotion order in CI if you want one.

In `bear.artifact.yml`, use `environments: [dev, int]` to allow deployment only in
those environments. An absent allowlist or `environments: []` means the artifact
never deploys in any environment; plan still reports it, under `skip` with the
policy reason, when its source changed, since that reporting happens after the
environment gate converts what would have been a deployment into a skip. Every
allowlist entry must be declared by the project, and one that is not fails the
whole plan before any output is printed:

```text
Error: error creating plan: artifact "api" allows undeclared environment "preprd"; bear.config.yml declares dev, int, prd
```

Every plan requires the environment argument, including a plan that ends up with
nothing to deploy and a plan selecting unchanged or pinned artifacts. Inherited or
configured `ENVIRONMENT` variables cannot select policy or satisfy this
requirement. Bear does not infer the environment from job/process variables,
branches, or CI metadata. Selecting an environment does not enable deployment for
artifacts with absent or empty allowlists.

The selection is passed as `$ENVIRONMENT` to every build and deploy step apply
will later run for this plan, overriding any configured or inherited value. It is
saved in the plan so apply uses the approved environment; plan itself runs no
steps, so nothing consumes the variable at plan time.

Only deployment is disabled by an absent or excluded allowlist. Change detection
and dependency propagation still run, and dependents use their own policies.
Direct changes, dependency-triggered deployments, `--pin`, and `--force` all
respect the gate.

The saved plan includes the selected environment, artifact allowlists, and visible
skip reasons for blocked deployments. Disabled deployments are not executable
entries and will not be recorded in `bear.lock.yml` by apply. The plan snapshots
the policy decision: apply displays and executes that snapshot without checking
the current policy. Replan after changing policy or the intended environment.
Old saved plans lacking allowlist snapshots must be regenerated; they cannot be applied.

History and pins are scoped to the selected environment. Legacy artifact-only
history is retained but ignored, not automatically mapped to an environment.
History under an environment the project no longer declares is retained too, and is
neither an error nor a warning; it is simply not shown by `bear list`.
Replan and review each environment after upgrading.

## Source Requirements

A normal plan with any deployments requires a clean Git repository before it runs,
across the entire repository, not just selected artifacts or the project
subdirectory. Bear state (`bear.lock.yml` and `.bear/` at any repository depth) is
excluded. A plan with nothing to deploy may start dirty, since it changes and
executes nothing regardless.

Because plan runs no commands, the source it fingerprints is exactly what is on
disk when planning finishes — there is no "post-build" state to capture, and
plan never modifies HEAD or the working tree itself. Plans save the full source
commit, a fingerprint of tracked and nonignored untracked working files, and
project-relative paths. `bear apply` verifies both the commit and the fingerprint
match before it runs anything, so an edit made to the approved checkout between
plan and apply — by a person, another job, or a stray build artifact — is caught
before any step runs, exactly like editing any other tracked file.

Apply does not need any file that existed only at plan time: because apply always
runs the language's build steps immediately before the target's deploy steps, it
regenerates whatever build output it needs itself, in the same run, on the same
checkout it is about to deploy from. Moving an approved plan to a different
checkout or agent only requires that checkout's tracked commit and nonignored
untracked files to match the plan's saved fingerprint exactly — typically true of
any clean checkout of the same commit — not that it already contain build
artifacts from wherever the plan was created.

`--pin` resolves and fingerprints the pinned commit in an isolated private
worktree rather than the caller's checkout, so dirty files, ignored dependencies,
and build outputs in the caller's working tree are never part of a pinned plan's
source. Pinning does not build or run anything in that worktree either — it exists
only to resolve the commit and take its fingerprint. If you want assurance that the
pinned revision actually builds and passes its tests, run `bear validate` against
it explicitly; see [The `--pin` Flag Does Not Validate](#the-pin-flag-does-not-validate).

Current project configuration supplies policy and steps for every plan, including
pinned ones: `--pin` changes where the source comes from, not what governs it.
See [Source Safety](../concepts/plan-apply.md#source-safety) for relocation and limits.

## The `--pin` Flag Does Not Validate

`--pin` used to run the pinned commit's validation steps in its private worktree,
because that used to be the only way to know a pinned deployment's build steps had
ever run at all — pinned apply would later replay them in a fresh worktree of its
own. Neither of those things is true anymore. `bear plan --pin` only resolves the
ref, snapshots its commit and fingerprint, and records the decision; it runs no
build or deploy commands. `bear apply` then runs the language's build steps and
the target's deploy steps for every pinned artifact it deploys, exactly as it does
for a normal deployment — there is one code path for both now, not two.

That means a pinned plan proves nothing about whether the pinned revision builds
or passes its tests. It only proves the revision resolves to a real commit and
what its source looked like. If you need that assurance — for example, before
freezing a deployable ahead of a release window, or before trusting a rollback
target — run `bear validate` against the pinned revision yourself. Validate has no
`--pin`/ref concept of its own, but it does accept `-d <path>` to point at an
arbitrary directory, so the practical workaround is to check the revision out
somewhere and validate that directory:

```bash
git worktree add /tmp/pin-check abc1234
bear validate -d /tmp/pin-check
git worktree remove /tmp/pin-check
```

`bear apply` will still build and test that same revision fresh, immediately
before it deploys — pinning does not skip that — so this extra `bear validate`
step is purely for earlier, cheaper feedback, not a prerequisite apply relies on.

## Saved Plan Safety

After successful CLI argument parsing and acquiring the workspace lock, planning
removes the previous `.bear/plan.yml` before loading
configuration. Failure to remove it stops planning. Invalid configuration,
provided but invalid environments, and empty plans therefore cannot leave an old
executable plan behind. A successful plan can contain skips and zero deployments;
apply reports that there is nothing to deploy. CLI syntax errors, including a
missing environment argument (`bear plan`), do not start planning and leave the
existing plan untouched. A rejected environment — one that is malformed, or
well-formed but not declared, such as `bear plan qa` in a project declaring `dev`,
`int`, `prd` — starts planning and clears the stale plan before failing.
Only run apply after a successful plan.

The plan is atomically written with mode `0600`; it may contain secrets. Protect
copies and CI artifacts. Old deployment plans missing source commit, fingerprint,
or deployment-policy snapshots must be regenerated.

## Change Reasons

| Reason | Description |
|--------|-------------|
| `files changed` | Artifact files differ from its deployed baseline; dirty source is permitted only when the plan has nothing to deploy |
| `new artifact` | No history for this artifact in the selected environment |
| `dependency '<name>' changed` | A transitive dependency differs from the consumer's deployed baseline |
| `pinned (forced deployment)` | `--force` selects an artifact pinned in this environment |
| Environment policy skip | The allowlist is absent, empty, or excludes the selected environment |

A pipeline that offers `--pin`/`--force` as build parameters must reject `--force`
without an accompanying artifact filter if it means "unfreeze exactly this one
deployable": Bear applies `--force` to every artifact in the current selection,
with no single-artifact restriction of its own. See
[Freeze & Unfreeze (Jenkins)](../concepts/freeze-unfreeze.md) for the full pattern.

## Output

Plan is now a pure decision: it prints the branding header, then goes straight to
the summary. There is no progress phase, because nothing runs. That summary is the
review artifact: it is what a human approves, and what `bear apply` then executes
without reprinting.

```text
Bear Plan
─────────

────────────────────────────────────────
Environment: int
Commit:      68a7fe1

deploy (1):
  - api (services/api): new artifact

skip (2):
  - backoffice (services/backoffice): deployment not enabled for environment int
  - lib shared (libs/shared): new artifact

Plan complete: 3 changed, 1 to deploy, 2 skipped

Run 'bear apply' to execute this plan.
```

The block after the rule states the run's facts on one aligned column.
`Environment:` is always present. `Commit:` carries the short source commit the
plan was built from, replaced by `Pinned:` when `--pin` selected the revision.
`Artifacts:` follows when artifact filters were given, and `Changes:` reports the
number of changed files when there are any. Every entry in the plan comes from that
one source, so it is stated once in the header rather than repeated per artifact.

Each section is labelled with its outcome and entry count (`deploy (1):`,
`skip (2):`), and its entries are sorted by name so repeated runs
of the same plan produce comparable summaries. An entry is a single line,
`  - <name> (<path>): <reason>`; the parenthesised path is omitted when the plan
recorded none. A library entry additionally carries a dim `lib` tag before its
bold name (`  - lib shared (libs/shared): ...`) so it reads as a library on
sight, never as a service that simply wasn't deployed; the tag is never shown
for a regular artifact. Empty sections are omitted.

`deploy` lists what apply will actually deploy: its reason is the artifact's
[change reason](#change-reasons). `skip` is every affected artifact that will
not deploy, and why — this includes both a would-be deployment blocked by
policy, such as `no changes detected`, `pinned (use --force to override)`, or
`deployment not enabled for environment prd`, **and** any artifact or library
that changed but has no deploy action of its own, most commonly a library
(libraries are never deployable) but also any artifact whose target defines no
deploy steps, tagged `lib` in that case. There is no separate "changed" section:
every artifact affected by a source change appears in exactly one place, either
`deploy` or `skip`, never both and never neither. Neither `deploy` nor `skip`
repeats the commit or names a target: the commit is the header's, and the
target is configuration you can read in `bear.artifact.yml`.

The command closes with one sentence,
`Plan complete: 3 changed, 1 to deploy, 2 skipped`; the skipped count is listed
only when there are skips, and the changed count is the total number of artifacts
and libraries affected by a source change — the same set that feeds `deploy` and
`skip` combined, so `to deploy` + `skipped` always equals `changed`. The
`Run 'bear apply' to execute this plan.` hint follows only when the plan has
something to deploy.

A plan with nothing changed and nothing to deploy prints `No changes detected.
Nothing to plan.` followed by the rule, the `Environment:` line, and any `skip`
list. That path reports no `Commit:` fact and no closing sentence, because
nothing was found to plan. A positional artifact filter that matches nothing is
rejected before any output at all, with `unknown artifact "name"` on stderr.

`bear apply` does not reprint this summary; it logs the deployments it runs and
lists only failures. See [apply output](apply.md#output).
</content>
