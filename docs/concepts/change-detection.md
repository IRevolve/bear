# Change Detection

Bear compares each artifact against its **last deployed commit in the selected
environment** (from `bear.lock.yml`). No base branch needed.

## What Triggers a Rebuild

| Trigger | Example |
|---------|---------|
| **Uncommitted changes** | Modified, staged, or untracked files in artifact dir |
| **New commits** | Commits since last deploy touching artifact dir |
| **New artifact** | No entry in lock file |
| **Dependency changed** | A library it depends on changed |

Each environment/artifact pair is tracked independently; they can be at different
versions. Transitive dependency changes are compared against the consumer's
deployment baseline, not the dependency's own history. Missing history means a
new artifact in that environment. Legacy artifact-only history is retained but
ignored rather than assigned to an environment.

Detecting uncommitted changes is not permission to deploy dirty source. Normal
plans with any deployments require clean source before validation; validation-only
plans may start dirty. Validation must not change HEAD or the tracked working-tree
diff. Generated nonignored files enter the saved post-validation fingerprint,
which apply verifies before pending deployments. Ignored dependencies and outputs
are outside that fingerprint. See [Source Safety](plan-apply.md#source-safety).
