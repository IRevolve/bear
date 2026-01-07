# bear apply

Execute the build and deployment plan.

## Usage

```bash
bear apply [artifacts...] [flags]
```

## Description

Validates and deploys artifacts that have changes. First runs all validation steps (setup, lint, test, build), then runs deployment steps.

## Arguments

| Argument | Description |
|----------|-------------|
| `artifacts` | Optional. Specific artifacts to apply. If omitted, applies all changed artifacts. |

## Flags

| Flag | Description |
|------|-------------|
| `-c, --commit` | Commit and push lock file with `[skip ci]` |
| `--pin <commit>` | Pin artifact to a specific commit |
| `-f, --force` | Force operation, ignore pinned artifacts |

## Examples

```bash
# Apply all changed artifacts
bear apply

# Apply specific artifacts
bear apply user-api order-api

# Apply and auto-commit lock file (for CI/CD)
bear apply --commit

# Pin artifact to a specific commit
bear apply user-api --pin abc1234

# Force apply a pinned artifact
bear apply user-api --force
```

## Execution Flow

1. **Validation Phase** — For each artifact:
    - Setup steps
    - Lint steps
    - Test steps
    - Build steps

2. **Deployment Phase** — For each validated artifact:
    - Run target deploy steps
    - Update lock file

## Output

```
Bear Execution Plan
===================
...

Proceed with apply? [y/N]: y

═══════════════════════════════════════
 Validating: api
═══════════════════════════════════════

▶ Download modules
  go mod download
  ✓ completed in 2.3s

▶ Vet
  go vet ./...
  ✓ completed in 1.1s

▶ Test
  go test -race ./...
  ✓ completed in 5.2s

▶ Build
  go build -o dist/app .
  ✓ completed in 3.4s

═══════════════════════════════════════
 Deploying: api
═══════════════════════════════════════

▶ Build image
  docker build -t ghcr.io/myorg/api:abc1234 .
  ✓ completed in 45.2s

▶ Push image
  docker push ghcr.io/myorg/api:abc1234
  ✓ completed in 12.1s

✓ Apply complete!
  1 artifact validated
  1 artifact deployed
```

## See Also

- [bear plan](plan.md)
- [Pinning](../concepts/pinning.md)
- [Lock File](../concepts/lock-file.md)
