# Pinning

Bear supports pinning artifacts to specific versions to prevent accidental redeploys or to lock down a known-good version.

## Pin an Artifact

Pin an artifact to a specific commit:

```bash
bear apply user-api --pin abc1234
```

This will:

1. Deploy the artifact at commit `abc1234`
2. Run validation steps
3. Run deployment steps
4. Update lock file with that commit
5. **Pin** the artifact to prevent future deploys

## What Happens

```
Before pin:
  user-api @ def5678 (current)

After pin:
  user-api @ abc1234 (pinned)
```

The artifact is now pinned. Future `bear apply` commands will skip it:

```bash
bear plan

📌 Pinned (will skip):
  - user-api (pinned at abc1234)
```

## Unpinning

To allow the artifact to be deployed again:

```bash
bear apply user-api --force
```

This:

1. Removes the pin
2. Deploys to the current HEAD
3. Updates the lock file

## Use Cases

### Rollback

Pin to a previous known-good version:

```bash
bear apply user-api --pin abc1234
```

### Lock Production

Keep production stable while developing:

```bash
# Pin all services to current versions
bear apply --pin $(git rev-parse HEAD)

# Later, unpin and deploy
bear apply --force
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

bear apply $ARTIFACT --pin $COMMIT --commit
```

## Pin Status in Plan

```bash
bear plan
```

Shows pinned artifacts:

```
📌 Pinned (will skip):
  - user-api (pinned at abc1234)
  - order-api (pinned at def5678)
```

## See Also

- [Lock File](lock-file.md)
- [bear apply](../commands/apply.md)
