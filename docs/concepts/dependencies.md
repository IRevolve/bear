# Dependencies

Dependencies are declared in `bear.artifact.yml` and resolved **transitively**:

```
shared-lib (changed)
  ↑
auth-lib (depends on shared-lib) → revalidate
  ↑
user-api (depends on auth-lib) → revalidate + redeploy
```

- **Libraries** (`bear.lib.yml`) — Validated only, never deployed
- **Services** (`bear.artifact.yml`) — Validated; deployed only when their own `environments` allowlist includes the selected environment

The redeployment above requires `user-api` to allow the selected environment, for
example `environments: [dev, int]` with `bear plan dev`. An absent or
empty allowlist disables deployment but retains validation and propagation to
dependents. A dependency change does not bypass a dependent's allowlist.

Change detection compares the full transitive dependency closure against each
consumer's last deployed commit in the selected environment. A library's absent
history or a newer deployment of another service does not replace that baseline.
Artifact filters select output artifacts after the complete dependency graph is
validated; they do not remove transitive dependencies from change detection.

Bear detects circular dependencies. Run `bear check` to validate.
