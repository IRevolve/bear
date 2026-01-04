# bear tree

Show dependency tree.

## Usage

```bash
bear tree [artifact] [flags]
```

## Description

Displays the dependency tree for all artifacts or a specific artifact. Useful for understanding how changes propagate through your monorepo.

## Arguments

| Argument | Description |
|----------|-------------|
| `artifact` | Optional. Show tree for a specific artifact. |

## Examples

```bash
# Show full dependency tree
bear tree

# Show tree for specific artifact
bear tree user-api
```

## Output

### Full Tree

```
Dependency Tree
===============

user-api
├── shared-lib
└── auth-lib

order-api
├── shared-lib
└── payment-lib
    └── shared-lib

dashboard
├── ui-components
└── shared-lib
```

### Single Artifact

```bash
bear tree order-api
```

```
order-api
├── shared-lib
└── payment-lib
    └── shared-lib
```

## Reverse Dependencies

The tree shows what each artifact depends on. To see what depends on a specific artifact, use `bear check` which shows all relationships.

## See Also

- [Dependencies](../concepts/dependencies.md)
- [Artifacts](../configuration/artifacts.md)
