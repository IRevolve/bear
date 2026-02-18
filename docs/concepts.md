# Concepts

## Change Detection

Bear compares each artifact against its **last deployed commit** (from `bear.lock.yml`). No base branch needed.

What triggers a rebuild:

| Trigger | Example |
|---------|---------|
| **Uncommitted changes** | Modified, staged, or untracked files in artifact dir |
| **New commits** | Commits since last deploy touching artifact dir |
| **New artifact** | No entry in lock file |
| **Dependency changed** | A library it depends on changed |

Each artifact is tracked independently — they can be at different versions.

## Dependencies

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

## Lock File

`bear.lock.yml` is auto-managed. It tracks the last deployed commit per artifact:

```yaml
artifacts:
  user-api:
    commit: abc1234567890
    timestamp: "2026-01-04T10:00:00Z"
    version: abc1234
    target: cloudrun
```

- Updated after each `bear apply`
- Auto-committed with `[skip ci]`
- Should be committed to your repo

## Pinning & Rollback

Pin an artifact to prevent redeployment or rollback to a previous version:

```bash
bear plan user-api --pin abc1234    # Pin to commit
bear apply                          # Deploy pinned version

bear plan                           # Future plans skip pinned artifacts
bear plan user-api --force          # Unpin and deploy latest
bear apply
```

## Plan/Apply Workflow

Inspired by Terraform:

1. **`bear plan`** — Detects changes, validates in parallel, writes `.bear/plan.yml`
2. **`bear apply`** — Reads the plan, deploys in parallel, updates lock file

The plan file is a checkpoint. You can review it, pass it through approval gates, or run it later. It's removed after `bear apply`.
