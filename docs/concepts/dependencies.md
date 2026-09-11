# Dependencies

Dependencies are declared in `bear.artifact.yml` and resolved **transitively**:

```
shared-lib (changed)
  ↑
auth-lib (depends on shared-lib) → changed
  ↑
user-api (depends on auth-lib) → changed + deploy
```

- **Libraries** (`bear.lib.yml`) — Never deployed; `bear plan` still reports them
  under `changed` when their source changed, and `bear validate` runs their
  build steps like any other artifact when asked to
- **Services** (`bear.artifact.yml`) — Reported by `bear plan` when their source
  or a dependency's changed; deployed only when their own `environments`
  allowlist includes the selected environment

The redeployment above requires `user-api` to allow the selected environment, for
example `environments: [dev, int]` with `bear plan dev`. An absent or
empty allowlist disables deployment but retains change detection and propagation to
dependents. A dependency change does not bypass a dependent's allowlist.

Change detection compares the full transitive dependency closure against each
consumer's last deployed commit in the selected environment. A library's absent
history or a newer deployment of another service does not replace that baseline.
Artifact filters select output artifacts after the complete dependency graph is
resolved; they do not remove transitive dependencies from change detection.

Bear detects circular dependencies as part of loading the artifact graph — the same
load `bear plan`, `bear validate`, `bear list`, and `bear list --tree` all share, so
a cycle fails all of them identically. Run `bear doctor` for a full diagnostic
report of everything wrong at once, including non-fatal warnings, instead of the
first blocking error.
