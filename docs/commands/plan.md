# bear plan

Show what would be validated and deployed.

## Usage

```bash
bear plan [artifacts...] [flags]
```

## Description

Analyzes changes in your repository and shows which artifacts need to be validated and deployed. This is a read-only operation that doesn't execute anything.

## Arguments

| Argument | Description |
|----------|-------------|
| `artifacts` | Optional. Specific artifacts to plan. If omitted, plans all changed artifacts. |

## Examples

```bash
# Plan all changed artifacts
bear plan

# Plan specific artifacts
bear plan user-api order-api

# Plan in a different directory
bear plan -d ./my-project
```

## Output

```
Bear Execution Plan
===================

🔍 To Validate:

  + api
    Path:     services/api
    Language: go
    Reason:   files changed
    Changed:  services/api/main.go
    Steps:    4
              - Download modules
              - Vet
              - Test
              - Build

🚀 To Deploy:

  ~ api
    Path:   services/api
    Target: docker
    Reason: artifact changed
    Last:   abc1234 (2 days ago)
    Steps:  2
            - Build image
            - Push image

⏭️  Unchanged (will skip):

  - other-service

📌 Pinned (will skip):

  - pinned-service

─────────────────────────────────

Plan: 1 to validate, 1 to deploy, 1 unchanged, 1 pinned

Run 'bear apply' to execute this plan.
```

## Change Reasons

| Reason | Description |
|--------|-------------|
| `files changed` | Uncommitted changes in artifact directory |
| `new commits` | Commits since last deploy |
| `new artifact` | Never deployed before |
| `dependency changed` | A dependency (library) changed |

## See Also

- [bear apply](apply.md)
- [Change Detection](../concepts/change-detection.md)
- [Dependencies](../concepts/dependencies.md)
