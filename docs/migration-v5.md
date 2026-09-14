# Upgrade to v5

v5.0.0 has three breaking changes:

1. **`bear.config.yml` must declare the project's deployment environments.** Bear
   no longer has a hardcoded `dev`/`int`/`prd` set. See
   [1. Declare the Environments](#1-declare-the-environments) below.
2. **`bear plan` no longer runs any commands, and `bear apply` now builds before
   it deploys.** Plan used to run every changed artifact's language steps in
   place; it is now a pure, side-effect-free decision. Apply now runs the
   language's build steps immediately before the target's deploy steps for
   every artifact it deploys, pinned or not. See
   [Plan No Longer Builds; Apply Now Does](#plan-no-longer-builds-apply-now-does)
   below — **read this one even if your `environments:` list is already
   correct**, because it changes what your CI pipeline must run to get
   pre-merge build/test feedback.
3. **`bear check` is renamed to `bear doctor`, with no alias.** Update any script
   or CI job that runs it. See
   [`bear check` Renamed To `bear doctor`](#bear-check-renamed-to-bear-doctor)
   below.

`bear.lock.yml` is untouched by any of these three changes. The only other changes in this
release are additive: the new [`bear validate`](commands/validate.md) command,
`bear init --environments`, and `bear list`/`bear list --tree` now validating the
artifact graph before printing anything — using the same checks and error wording
as `bear plan`/`bear validate` — so a structurally broken project fails fast
instead of showing partial output; a structurally valid project sees no change.
See the [v5.0.0 release notes](releases/v5.0.0.md).

!!! tip "Finish in-flight plans before you edit"
    Upgrading the Bear binary does not invalidate an approved `.bear/plan.yml` —
    apply never reads `bear.config.yml`. Editing `bear.config.yml` does, because it
    is ordinary tracked source and apply checks the
    [source fingerprint](concepts/plan-apply.md#source-safety). Run `bear apply` for
    any plan already approved, **then** make the edits below.

!!! note "Plans created before this upgrade are rejected, not misapplied"
    `.bear/plan.yml` now carries a schema `version` field, and `bear apply`
    checks it before anything else. A plan written by a Bear binary from
    before this release predates that field, so the new `bear apply` refuses
    it outright:

    ```text
    Error: unsafe saved plan: schema version 0 (want 1); run 'bear plan <environment>' again
    ```

    If you see this, just run `bear plan <environment>` again with the new
    binary — there is no silent-misapply risk. Applying the plan with the
    **old** binary first, or deleting `.bear/plan.yml` before replanning, both
    work too.

## 1. Declare the Environments

Add the list to `bear.config.yml`. Declare what you actually deploy to, in whatever
order you like:

```yaml title="bear.config.yml"
name: my-platform
environments: [dev, int, prd]
```

Upgrading from v4 with no changes? `[dev, int, prd]` reproduces v4's behaviour
exactly, and is also what `bear init` writes by default. Otherwise declare your own:
`[preprd, prd]`, `[prd]`, `[dev, int, uat, prd]` are all valid.

The field is required. Omitting it, or writing `environments: []`, fails to load
every command that reads the config:

```text
Error: error loading config: /src/bear.config.yml: environments must list at least one deployment environment, for example [dev, int, prd]
```

Each name must match `[a-z][a-z0-9-]*` and be 1 to 32 characters, with no
duplicates:

```text
Error: error loading config: /src/bear.config.yml: environments: invalid environment name "Prd": use 1 to 32 characters matching [a-z][a-z0-9-]*
Error: error loading config: /src/bear.config.yml: environments: duplicate environment "dev"
```

The constraint is not decorative: the name becomes a `bear.lock.yml` key, the
injected `ENVIRONMENT` value, and a positional CLI argument.

Declaration order is preserved in error messages, in `bear doctor`, and in
`bear list --tree`. It carries **no promotion semantics**. Declaring
`[dev, int, prd]` does not make Bear require `int` before `prd`. If you want a
promotion order, enforce it in CI.

## 2. Make Every Allowlist a Subset

An artifact allowlist may only name declared environments:

```yaml title="services/api/bear.artifact.yml"
name: api
target: cloudrun
environments: [dev, prd]   # both declared above
```

An entry that is not declared is an error from `bear plan`, `bear validate`, and
`bear doctor`, before any step runs. Each command wraps the same message:

```text
Error: error creating plan: artifact "api" allows undeclared environment "preprd"; bear.config.yml declares dev, int, prd
Error: error loading artifacts: artifact "api" allows undeclared environment "preprd"; bear.config.yml declares dev, int, prd
```

Run `bear doctor` from your project root. It reports **every** offender in one run
and prints the declared list, so one pass tells you the whole edit:

```text
  Environments: dev, int, prd

  Errors:
    • artifact "api" allows undeclared environment "preprd"; bear.config.yml declares dev, int, prd
    • artifact "web" allows undeclared environment "staging"; bear.config.yml declares dev, int, prd

  Doctor found 2 error(s)
```

Either add the missing name to `environments:` or remove it from the allowlist —
whichever matches what you actually deploy. Libraries are unaffected:
`bear.lib.yml` has no `environments` field and strict decoding rejects one.

## 3. Update the Plan Argument If You Renamed Anything

`bear plan <environment>` and `bear validate --environment` accept only a declared
name:

```text
Error: error creating plan: unknown environment "production": bear.config.yml declares dev, int, prd
Error: unknown environment "production": bear.config.yml declares dev, int, prd
```

If you kept `[dev, int, prd]`, no CI change is needed. If you declared something
else, update the environment in your pipelines to match.

## Plan No Longer Builds; Apply Now Does

Before this release, `bear plan` ran every changed artifact's language steps
(tests, lint, build) in place as it planned, and a `--pin` plan additionally ran
those steps in a private worktree; `bear apply` only ran target deploy steps,
except for a pinned deployment, where it replayed the saved validation/setup
steps in a fresh worktree first because the pinned worktree from plan time no
longer existed.

`bear plan` now runs **no commands at all**, for a normal plan or a `--pin`
plan. It only compares the current source against `bear.lock.yml` per
environment and writes a decision to `.bear/plan.yml`. `bear apply` is now the
**only** place anything ever runs: for every artifact it deploys, it runs the
language's build steps first, then the target's deploy steps, in one continuous
progress task — uniformly for normal and pinned deployments, replacing the old
special case with a single code path.

**This is a deployment-execution-model change, not a cosmetic one.** Concretely:

- **A `bear plan` run no longer proves the code builds or passes tests.** It is
  now purely a diff against deployment history. If your pipeline relied on
  `bear plan` alone to catch a broken build or a failing test before merge, it
  no longer will.
- **`bear apply` takes longer** than it used to, because it now does the build
  work that used to happen during plan. A deployment that previously was "plan
  builds, apply just pushes" is now "apply builds and pushes."
- **Add an explicit `bear validate` step** wherever you relied on plan's old
  build behavior for pre-merge or pre-deployment assurance. `bear validate` runs
  the exact same `languages.<lang>.steps` apply's build phase runs, with no
  environment, no plan file, and no Git — see
  [Merge Request Validation](ci-cd.md#merge-request-validation) and
  [Trusted Deployments](ci-cd.md#trusted-deployments) for where to add it.
- **Monitoring or log-parsing tooling built around plan's old output breaks.**
  Plan no longer prints a `Validating N artifacts` phase, `Validating...` /
  `Still validating...` / `Validation complete` lines, or a
  `Validation complete: N artifacts in <time>` sentence — it goes straight from
  the branding header to the summary. Its closing sentence changed from
  `Plan complete: N validated, M to deploy[, K skipped]` to
  `Plan complete: N changed, M to deploy[, K skipped]`, and a new `changed (N):`
  section appears between `deploy` and `skip`, listing artifacts and libraries
  whose source changed but that have nothing to deploy. Update any script or
  dashboard that greps plan's log for the old strings.

!!! bug "Fixed in v5.0.1"
    As shipped in v5.0.0, that `changed (N):` section had a bug: it also
    listed artifacts whose deploy action environment policy had converted
    to a skip, so they appeared twice (there and under `skip`, with
    different reasons), and it gave changed libraries a generic reason
    with no indication they were libraries. **[v5.0.1](releases/v5.0.1.md)
    removed the `changed` section entirely** — every affected artifact now
    appears in exactly one place, `deploy` or `skip`, with libraries
    tagged `lib` in `skip`. If you are upgrading from pre-v5, upgrade
    straight to v5.0.1 or later and use its output as the current
    reference instead of the section described above.

- **`.bear/plan.yml`'s schema changed.** `PlanFile` gained a `version` field,
  stamped by every plan and checked by every apply before anything else runs.
  `PlanFile.validated` is renamed to `changed`. Each `PlanArtifact` gained
  `build_steps` (the language steps, run first by apply) alongside the
  existing `steps` (target deploy steps, run second). The old `validations`
  list is gone entirely — it existed only so a pinned apply could replay
  validation in a fresh worktree, and that mechanism no longer exists now that
  every apply always builds fresh. See the note above: a plan file written
  before this change has no `version` field, so the new apply rejects it
  outright instead of guessing at its shape.
- **`--pin` no longer validates the pinned commit at plan time.** It only
  resolves and fingerprints it in an isolated worktree. `bear apply` builds and
  tests it fresh immediately before deploying, exactly like a normal deployment.
  If you want assurance the pinned revision builds *before* that point — for
  example ahead of a freeze — run `bear validate` against it explicitly; see
  [`bear plan`'s `--pin` section](commands/plan.md#the-pin-flag-does-not-validate)
  for the exact commands, since `bear validate` has no ref/pin concept of its
  own and the practical workaround is `-d` against a checked-out worktree.

The full recommended chain is now `doctor` → `validate` → `plan` → `apply`, where
`validate` is where build/test assurance actually lives, `plan` is a fast
side-effect-free diff you can run as often as you like, and `apply` does the
complete job — build and deploy — for everything it deploys.

## `bear check` Renamed To `bear doctor`

`bear check` is renamed to `bear doctor`. There is **no `bear check` alias**:
scripts, CI jobs, and habit all need to switch to the new name.

```text
Error: unknown command "check" for "bear"
```

`check` sat next to `bear validate` as an overloaded, ambiguous verb — both
names could plausibly mean "does this build," which is exactly what `validate`
does and `doctor` deliberately does not. `doctor` is the idiom several CLIs
already use (`brew doctor`, `flutter doctor`, `git fsck`'s spirit if not its
name) for a single, side-effect-free diagnostic pass over configuration and
environment, which is precisely this command's job: load the config, then check
languages, targets, artifacts, environments, dependencies, and cycles — nothing
else runs.

Output wording changed to match the new name. The phase heading is now
`Diagnosing configuration` (it was `Validating configuration`), and the closing
error line is now `Doctor found N error(s)` (it was `Check failed with N
error(s)`). The per-step verbs are unchanged — each step still reports
`Checking` / `Still checking` / `Check complete` / `Check failed`, and a clean
run still prints `All checks passed!` — only the command name and the two lines
above changed:

```text
  Environments: dev, int, prd

  Errors:
    • artifact "api" allows undeclared environment "preprd"; bear.config.yml declares dev, int, prd

  Doctor found 1 error(s)
```

Update every script, CI job, alias, and doc bookmark that still runs
`bear check`.

## What Does Not Change

- **`bear.lock.yml` is untouched.** No key is renamed, moved, or removed, and no
  migration runs. History under an environment you no longer declare is kept, and is
  neither an error nor a warning. `bear list` and `bear list --tree` simply report the
  declared set, so retiring an environment hides its history from those views without
  deleting it. Restore the name and it reappears.
- **The environments declaration itself has no effect on plan/apply's execution
  model.** Whether you declare one environment or ten, plan still runs nothing
  and apply still builds then deploys; the two changes in this release are
  independent of each other.
- **`bear apply`'s policy checks keep working on plans approved before the
  upgrade** — for the environment declaration, at least. Apply never
  reads `bear.config.yml` — the saved plan is the approved snapshot — so upgrading the
  binary strands nothing on the environment-declaration front, and no change to the
  declared environments can make apply reject an approved plan on policy grounds. Apply still enforces its existing checks:
  the saved environment name's syntax, each artifact's **saved** allowlist, and each
  saved `ENVIRONMENT` variable. Its messages no longer name a fixed
  `(dev, int, or prd)` set. Editing `bear.config.yml` is a separate matter: it is
  tracked source, so it invalidates an in-flight plan through the source fingerprint,
  as any other edit would. **This does not mean a pre-upgrade plan is safe to
  apply with the new binary** — see the plan/apply execution-model change above;
  regenerate it instead.
- **Change detection, dependency propagation, source fingerprinting, retries and
  checkpointing, Git publication, locking, and presets are unchanged** as
  mechanisms. What changed is *when* build steps run relative to them — during
  apply now, never during plan — not how change detection or the fingerprint
  contract themselves work. This release also adds
  [`bear validate`](commands/validate.md) for merge-request CI, which is purely
  additive and requires no configuration.

## Checklist

1. Apply any plan that is already approved, before editing configuration or
   upgrading the binary in a place that still has an in-flight plan.
2. Add `environments:` to `bear.config.yml`.
3. Run `bear doctor` and fix every reported allowlist.
4. Update the environment argument in CI if you renamed or dropped a name.
5. Leave `bear.lock.yml` alone.
6. Add an explicit `bear validate` step wherever your pipeline relied on
   `bear plan` to build or test the code — plan no longer does that.
7. Expect `bear apply` to take longer: it now builds before it deploys.
8. Update any tooling that parsed plan's old `Validating`/`Validation complete`
   output or its `N validated` closing sentence.
9. Delete any `.bear/plan.yml` left over from before the upgrade rather than
   applying it with the new binary.
10. Replace every `bear check` with `bear doctor` in scripts, CI jobs, and
    aliases — there is no `bear check` alias to fall back on.
