# bear list

List all discovered artifacts.

## Usage

```bash
bear list [artifacts...] [flags]
```

## Description

Scans the project for `bear.artifact.toml` and `bear.lib.toml` files and displays all discovered artifacts.

## Arguments

| Argument | Description |
|----------|-------------|
| `artifacts` | Optional. Filter tree view to specific artifacts. Only used with `--tree`. |

## Flags

| Flag | Description |
|------|-------------|
| `--tree` | Display as dependency tree |

## Examples

```bash
# List all artifacts
bear list

# Show dependency tree
bear list --tree

# Show tree for specific artifact
bear list --tree user-api

# List artifacts in a different directory
bear list -d ./my-project
```

## Output

```
Discovered Artifacts
====================

Services:
  • user-api (services/user-api) → cloudrun
  • order-api (services/order-api) → cloudrun
  • dashboard (apps/dashboard) → docker

Libraries:
  • shared-lib (libs/shared)
  • ui-components (libs/ui)

Total: 5 artifacts (3 services, 2 libraries)
```

## See Also

- [Artifacts](../configuration/artifacts.md)
- [bear check](check.md)
