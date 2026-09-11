# Pinning & Rollback

Pin an artifact to prevent redeployment or rollback to a previous version:

```bash
bear plan dev user-api --pin abc1234 # Pin to commit
bear apply                          # Deploy pinned version

bear plan dev                       # Future plans skip pinned artifacts
bear plan dev user-api --force      # Unpin and deploy latest
bear apply
```

These examples assume the project
[declares](../configuration.md#deployment-environments) `dev` and that `dev` is in
`user-api`'s `environments` allowlist, such as
`environments: [dev, int]`. Neither `--pin` nor `--force` bypasses the allowlist:
absent, empty, or excluding `dev` means no deployment; change detection still runs.
Every plan requires a declared environment before any artifact filters, including
a plan that ends up with nothing to deploy and one selecting already pinned
artifacts.
Inherited or configured `ENVIRONMENT` variables cannot satisfy this requirement.

Pins and deployment history are environment-specific. Pinning in `dev` does not
pin or advance history in any other environment.

`bear plan --pin` resolves the pin to a commit and takes its fingerprint in a
private detached worktree, preserving the project's repository-relative
location — it does not build, test, or otherwise validate that commit. Current
project configuration supplies artifact selection, allowlists, and steps; these
are saved in the plan. Dirty or ignored files from the caller's checkout are
never copied in.

For pending deployments, `bear apply` creates a fresh worktree at the saved
commit and checks its fingerprint against the approved plan **before** running
anything. Only then does it run the language's build steps and the target's
deploy steps, exactly as for a normal deployment — pinning changes where the
source comes from, not what runs against it. If you want assurance that the
pinned revision actually builds before you get to that point — for example ahead
of a release freeze — run `bear validate` against it explicitly first; validate
has no `--pin`/ref concept of its own, so check the revision out with
`git worktree add` and point validate at it with `-d`. See
[`bear plan`'s `--pin` section](../commands/plan.md#the-pin-flag-does-not-validate)
for the exact commands. Submodules fail closed regardless. See
[Source Safety](plan-apply.md#source-safety) for the full contract.

Rollback deploys older source; it does not automatically reverse database changes
or other external effects. Reproducible dependencies and safe rollback steps are
the project's responsibility.

See [Freeze & Unfreeze (Jenkins)](freeze-unfreeze.md) for a parameterized pipeline
pattern that wraps `--pin` and `--force` in `REF`/`DEPLOYABLE`/`FORCE` build
parameters to freeze one or all deployables and later unfreeze exactly one.
