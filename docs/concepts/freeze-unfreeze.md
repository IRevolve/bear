# Freeze & Unfreeze Deployments (Jenkins Pattern)

This page describes a **CI-side pattern** built from existing `bear plan` flags —
`--pin` and `--force` — that a parameterized Jenkins pipeline can use to freeze one
or more deployables to a fixed commit and later unfreeze exactly one of them. It is
not a dedicated Bear "freeze" feature: Bear has no `freeze` or `unfreeze` command,
and every behaviour below is exactly [`--pin`](../commands/plan.md#flags) and
[`--force`](../commands/plan.md#flags) as documented in
[Pinning & Rollback](pinning.md). Read that page first; this one only adds the
pipeline shape around it.

See [`examples/Jenkinsfile.freeze`](https://github.com/irevolve/bear/blob/main/examples/Jenkinsfile.freeze)
for the complete, working pipeline this page describes.

## The three parameters

| Parameter | Type | Meaning |
|-----------|------|---------|
| `REF` | string, optional | A Git SHA, tag, or branch to freeze to. Empty means "no freeze requested". |
| `DEPLOYABLE` | string, optional | An artifact name from `bear list`. Empty means "every artifact in this run". |
| `FORCE` | boolean, default `false` | Unfreeze: deploy current HEAD for `DEPLOYABLE` and clear its pin. |

`bear list` (or `bear list --tree`) prints the configured artifact names — use it
to populate `DEPLOYABLE` as a Jenkins choice parameter, or to validate a free-text
value server-side before calling Bear.

## The three branches

**No `REF`, no `FORCE`** — the normal case. Plan and apply latest for every
artifact not currently pinned in the target environment:

```bash
bear plan <environment> ${DEPLOYABLE}
bear apply
```

Any artifact pinned in this environment is skipped automatically with reason
`pinned (use --force to override)`, while every non-pinned artifact in the same
run still deploys. Freezing one artifact does not affect any other — pins are
tracked per `(environment, artifact)` pair.

**`REF` given** — freeze. Resolve `REF` locally first (see
[Fetch REF before pinning](#fetch-ref-before-pinning) below), then:

```bash
bear plan <environment> ${DEPLOYABLE} --pin ${REF}
bear apply
```

This pins exactly the named deployable — or, with `DEPLOYABLE` empty, every
eligible artifact in the selection — to that exact commit in that environment.
This works whether the pinned commit is older or newer than HEAD: Bear does not
compare it against history before pinning.

**Unfreeze** — requires an explicit `DEPLOYABLE` **and** `FORCE=true`:

```bash
bear plan <environment> ${DEPLOYABLE} --force
bear apply
```

This deploys current HEAD for exactly `DEPLOYABLE` and clears its pin
(the lock entry moves to `Pinned: false`). `bear apply` itself takes no
`--force`; unpinning happens at plan time only, exactly as in
[Pinning & Rollback](pinning.md).

### The guard is scripted, not a Bear flag

`--force` **without** an artifact filter affects every artifact in the current
selection, not just one. Bear has no concept of "force applies to a single
artifact only" — that restriction has to live in the pipeline. A pipeline
implementing "explicit unfreeze of exactly one deployable" **must** reject
`FORCE=true` combined with an empty or blank `DEPLOYABLE` before calling Bear at
all, for example:

```groovy
script {
    if (params.FORCE && !params.DEPLOYABLE?.trim()) {
        error 'FORCE=true requires an explicit DEPLOYABLE; refusing to force every artifact.'
    }
}
```

Skipping this guard means a well-intentioned "unfreeze user-api" run with a blank
`DEPLOYABLE` field silently force-deploys and unpins **every** artifact in the
environment instead.

### Fetch REF before pinning

Bear does not fetch remote refs itself. `--pin <ref>` only resolves what already
exists in the local Git object database (`git rev-parse --verify`), so a tag or
branch that was created or moved on the remote after the last checkout is
invisible to it until the pipeline fetches it:

```bash
git fetch origin --tags "${REF}"
```

Fetch before every pinning `bear plan` call, not just the first one in a
pipeline's lifetime — a long-lived agent workspace can otherwise pin to a stale
local ref with the same name.

### Unknown artifact names fail loudly

An unmatched name in `bear plan`'s positional artifact filter is a hard,
non-zero-exit error, `unknown artifact "name"`, consistent with `bear validate`.
A typo in the `DEPLOYABLE` parameter therefore fails the build immediately
instead of silently planning zero artifacts — there is no need for the pipeline
to duplicate that check, only to surface Bear's exit code.

## What this pattern does not give you

- **No dedicated freeze state.** "Frozen" is just `Pinned: true` in
  `bear.lock.yml` for that `(environment, artifact)` pair, visible via
  `bear list`. There is no separate freeze registry or expiry.
- **No promotion or scheduling.** `REF`, `DEPLOYABLE`, and `FORCE` are ordinary
  Jenkins parameters; Bear does not know about "freeze windows" or maintenance
  calendars. Gate the pipeline itself (e.g. `when` conditions, approval steps)
  if you need that.
- **No environment-wide unfreeze in one call.** Unfreezing `DEPLOYABLE`s one at a
  time is the guarded behaviour above. Running `--force` with no `DEPLOYABLE` to
  unfreeze everything at once is still possible with plain Bear, but this
  pattern deliberately blocks it through the pipeline guard, not through Bear.

See [Pinning & Rollback](pinning.md) for the underlying `--pin`/`--force`
semantics, environment-scoping rules, and rollback/fingerprint limits, and
[Plan/Apply Workflow](plan-apply.md) for how a plan is decided and then applied.
Note that `--pin` no longer validates the pinned commit at plan time — it only
resolves and fingerprints it. `bear apply` builds and tests it fresh, right before
deploying, the same as any other deployment. If you want that assurance before
the freeze itself (for example, before an operator relies on a pinned commit
being ready ahead of a release window), run `bear validate` against the pinned
revision first; see [`--pin` does not validate](../commands/plan.md#the-pin-flag-does-not-validate).
