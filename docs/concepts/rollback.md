# Rollback

Bear supports rolling back artifacts to previous versions and pinning them to prevent accidental redeploys.

## Quick Rollback

Roll back an artifact to a specific commit:

```bash
bear apply user-api --rollback=abc1234
```

This will:

1. Check out the artifact at commit `abc1234`
2. Run validation steps
3. Run deployment steps
4. Update lock file with the old commit
5. **Pin** the artifact to prevent future deploys

## What Happens

```
Before rollback:
  user-api @ def5678 (current)

After rollback:
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

## Rollback Without Pin

If you want to rollback but not pin (allow immediate redeploy if there are newer changes):

```bash
# Rollback
bear apply user-api --rollback=abc1234

# Immediately force to HEAD
bear apply user-api --force
```

## Finding Commits

To find a commit to rollback to:

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

bear apply $ARTIFACT --rollback=$COMMIT
git add bear.lock.yml
git commit -m "rollback: $ARTIFACT to $COMMIT"
git push
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
