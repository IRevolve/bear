# Pinning & Rollback

Pin an artifact to prevent redeployment or rollback to a previous version:

```bash
bear plan user-api --pin abc1234    # Pin to commit
bear apply                          # Deploy pinned version

bear plan                           # Future plans skip pinned artifacts
bear plan user-api --force          # Unpin and deploy latest
bear apply
```
