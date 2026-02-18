# Change Detection

Bear compares each artifact against its **last deployed commit** (from `bear.lock.yml`). No base branch needed.

## What Triggers a Rebuild

| Trigger | Example |
|---------|---------|
| **Uncommitted changes** | Modified, staged, or untracked files in artifact dir |
| **New commits** | Commits since last deploy touching artifact dir |
| **New artifact** | No entry in lock file |
| **Dependency changed** | A library it depends on changed |

Each artifact is tracked independently — they can be at different versions.
