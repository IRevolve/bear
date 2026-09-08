# Pinning & Rollback

Pin an artifact to prevent redeployment or rollback to a previous version:

```bash
bear plan dev user-api --pin abc1234 # Pin to commit
bear apply                          # Deploy pinned version

bear plan dev                       # Future plans skip pinned artifacts
bear plan dev user-api --force      # Unpin and deploy latest
bear apply
```

These examples require `dev` in `user-api`'s `environments` allowlist, such as
`environments: [dev, int]`. Neither `--pin` nor `--force` bypasses the allowlist:
absent, empty, or excluding `dev` means no deployment, while validation is retained.
Every plan requires `dev`, `int`, or `prd` before any artifact filters, including
validation-only plans and plans selecting already pinned artifacts.
Inherited or configured `ENVIRONMENT` variables cannot satisfy this requirement.

Pins and deployment history are environment-specific. Pinning in `dev` does not
pin or advance history in `int` or `prd`.

Bear resolves the pin to a commit and validates its source in a private detached
worktree, preserving the project's repository-relative location. Current project
configuration supplies artifact selection, allowlists, and steps; these are saved
in the plan. Dirty or ignored files from the caller's checkout are not copied.

For pending deployments, apply creates a fresh worktree at the saved commit,
reruns saved validation/setup steps to rebuild outputs, then checks the resulting
source fingerprint before deployment. Nonignored generated files must reproduce
the approved fingerprint; ignored dependencies and outputs are excluded and are
not guaranteed immutable. Submodules fail closed. See
[Source Safety](plan-apply.md#source-safety) for the full contract.

Rollback deploys older source; it does not automatically reverse database changes
or other external effects. Reproducible dependencies and safe rollback steps are
the project's responsibility.
