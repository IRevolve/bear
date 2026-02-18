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
- **Services** (`bear.artifact.yml`) — Validated and deployed

Bear detects circular dependencies. Run `bear check` to validate.
