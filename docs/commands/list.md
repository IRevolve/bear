# bear list

List discovered artifacts. Use `--tree` for dependency visualization.

```bash
bear list                      # List all
bear list --tree               # Dependency tree
bear list --tree user-api      # Tree for specific artifact
bear list user-api             # Artifact arguments also select tree mode
```

List includes languages, targets, dependencies, and deployment status reported
separately for each environment declared in `bear.config.yml`, in declaration
order:

```text
   user-api → cloudrun [preprd: v2; prd: v1 (pinned)]
```

Only declared environments appear. `bear.lock.yml` history under an environment the
project has retired is kept on disk but not shown, so removing a name from
`environments` hides its status here without deleting anything. An artifact with no
history in any declared environment shows no status at all.

It uses configured language overrides and discovery
exclusions, and can fail on strict config errors or ambiguous language detection.
The plain list prints artifact `vars`; avoid storing secrets directly in those
values or exposing the output in public logs.

Before printing anything, `list` and `list --tree` load and validate the whole
artifact graph — the same checks `bear plan` and `bear validate` run: duplicate
artifact/library names, an artifact referencing an unknown target, an artifact's
`environments` allowlist naming an environment `bear.config.yml` does not declare,
unresolved dependencies, and dependency cycles. A structurally broken project fails
immediately with that same error and prints no partial or misleading output:

```text
Error: error loading artifacts: artifact "api" references unknown target "missing"
```

Run [`bear doctor`](doctor.md) instead if you want a full diagnostic report of every
issue at once, including non-fatal warnings, rather than the first blocking error.
