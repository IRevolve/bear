# bear list

List discovered artifacts. Use `--tree` for dependency visualization.

```bash
bear list                      # List all
bear list --tree               # Dependency tree
bear list --tree user-api      # Tree for specific artifact
bear list user-api             # Artifact arguments also select tree mode
```

List includes languages, targets, dependencies, and deployment status separately
for `dev`, `int`, and `prd`. It uses configured language overrides and discovery
exclusions, and can fail on strict config errors or ambiguous language detection.
The plain list prints artifact `vars`; avoid storing secrets directly in those
values or exposing the output in public logs.
