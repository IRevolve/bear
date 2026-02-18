# Quick Start

Get Bear up and running in 5 minutes.

## 1. Initialize a Project

Navigate to your monorepo and run:

```bash
bear init
```

This creates a `bear.config.yml` with auto-detected languages.

## 2. Configure Presets

Edit the config to use presets for your languages and targets:

```yaml title="bear.config.yml"
name: my-platform

use:
  languages: [go, node]
  targets: [docker, cloudrun]
```

!!! tip "Available Presets"
    Run `bear preset list` to see all available languages and targets.

## 3. Add Artifacts

Create a `bear.artifact.yml` in each deployable service:

```yaml title="services/api/bear.artifact.yml"
name: api
target: docker
env:
  REGISTRY: ghcr.io/myorg
```

For libraries (validate-only, no deploy):

```yaml title="libs/shared/bear.lib.yml"
name: shared-lib
```

## 4. Check Configuration

Validate your setup:

```bash
bear check
```

This verifies:

- ✓ Config syntax
- ✓ Language and target definitions
- ✓ Artifact discovery
- ✓ Dependency resolution
- ✓ No circular dependencies

## 5. View Dependency Tree

See how your artifacts depend on each other:

```bash
bear list --tree
```

## 6. Plan Changes

Detect changes, validate, and create a deployment plan:

```bash
bear plan
```

This runs validation (lint, test, build) in parallel and writes the plan to `.bear/plan.yml`.

Example output:

```
  BEAR — Plan
──────────────────────────────────────

  Detecting changes...

  Validating 1 artifact (concurrency: 10)

  ✓ api               validated in 8.2s

──────────────────────────────────────
  Deploy
──────────────────────────────────────

  api                 docker     files changed

──────────────────────────────────────
  Summary: 1 to deploy, 0 unchanged

  Plan written to .bear/plan.yml
  Run 'bear apply' to execute this plan.
```

## 7. Apply Changes

Execute the deployment plan:

```bash
bear apply
```

This reads `.bear/plan.yml`, deploys the artifacts, updates the lock file, and auto-commits with `[skip ci]`.

## Next Steps

- Learn about [project configuration](../configuration/project.md)
- Understand [change detection](../concepts/change-detection.md)
- Explore [available presets](../configuration/presets.md)
