# Lock File

Bear uses a lock file (`bear.lock.yml`) to track deployed versions of each artifact.

## Purpose

The lock file stores:

- Last deployed commit per artifact
- Deployment timestamp
- Version string
- Target used
- Pinned status

## Format

```yaml title="bear.lock.yml"
artifacts:
  user-api:
    commit: abc1234567890abcdef
    timestamp: "2026-01-04T10:00:00Z"
    version: abc1234
    target: cloudrun
  order-api:
    commit: def4567890123abcdef
    timestamp: "2026-01-03T15:30:00Z"
    version: def4567
    target: cloudrun
    pinned: true  # Artifact is pinned
```

## How It Works

1. **Plan** — Bear compares each artifact against its lock file entry
2. **Apply** — After successful deploy, lock file is updated
3. **Rollback** — Lock file is updated with old commit + pinned flag

## Automatic Management

The lock file is automatically managed by Bear:

- Created on first `bear apply`
- Updated after each successful deployment
- Should be committed to Git

!!! tip "Commit the Lock File"
    Always commit `bear.lock.yml` to your repository. This ensures consistent state across CI/CD runs and team members.

## Pinning

When an artifact is pinned, it's skipped during `bear apply`:

```yaml
user-api:
  commit: abc1234567890
  pinned: true
```

Artifacts get pinned when:

- You rollback: `bear apply user-api --rollback=abc1234`

To unpin and force apply:

```bash
bear apply user-api --force
```

## Clean State

If you need to force a full rebuild:

```bash
# Remove lock file (all artifacts will rebuild)
rm bear.lock.yml

# Or remove specific artifact
# (edit bear.lock.yml manually)
```

## CI/CD Usage

In CI/CD, the lock file determines what gets rebuilt:

```yaml title=".github/workflows/deploy.yml"
- name: Plan
  run: bear plan
  
- name: Apply
  run: bear apply
  
- name: Commit lock file
  run: |
    git add bear.lock.yml
    git commit -m "chore: update bear.lock.yml" || true
    git push
```

## See Also

- [Change Detection](change-detection.md)
- [Rollback](rollback.md)
- [bear apply](../commands/apply.md)
