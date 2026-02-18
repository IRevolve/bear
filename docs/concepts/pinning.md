# Pinning

Bear supports pinning artifacts to specific versions to prevent accidental redeploys or to lock down a known-good version.

## Pin an Artifact

Pin an artifact to a specific commit during planning:

```bash
bear plan user-api --pin abc1234
bear apply
```

This will:

1. Validate the artifact
2. Write the plan with the pinned commit
3. On `bear apply`: deploy the artifact at commit `abc1234`
4. Update lock file with that commit and mark it as **pinned**

## What Happens

```
Before pin:
  user-api @ def5678 (current)

After pin:
  user-api @ abc1234 (pinned)
```

The artifact is now pinned. Future `bear plan` commands will skip it:

```bash
bear plan

  Skipped:
    user-api            pinned
```

## Unpinning

To allow the artifact to be deployed again, use `--force`:

```bash
bear plan user-api --force
bear apply
```

This:

1. Removes the pin
2. Plans a deploy to the current HEAD
3. On `bear apply`: deploys and updates the lock file

## Use Cases

### Rollback

Pin to a previous known-good version:

```bash
bear plan user-api --pin abc1234
bear apply
```

### Lock Production

Keep production stable while developing:

```bash
# Pin all services to current versions
bear plan --pin $(git rev-parse HEAD)
bear apply

# Later, unpin and deploy
bear plan --force
bear apply
```

## Finding Commits

To find a commit to pin to:

```bash
# View commit history for an artifact
git log --oneline -- services/user-api

# Or check the lock file history
git log --oneline bear.lock.yml
```

## CI/CD Rollback

In a CI/CD environment:

```bash
# Emergency rollback script
ARTIFACT=$1
COMMIT=$2

bear plan $ARTIFACT --pin $COMMIT
bear apply
```

## Pin Status in Plan

```bash
bear plan
```

Shows pinned artifacts in the skipped section:

```
  Skipped:
    user-api            pinned
    order-api           pinned
```

## See Also

- [Lock File](lock-file.md)
- [bear plan](../commands/plan.md)
