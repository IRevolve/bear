# Lock File

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
