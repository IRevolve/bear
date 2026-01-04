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
bear tree
```

## 6. Plan Changes

See what would be built and deployed:

```bash
bear plan
```

Example output:

```
Bear Execution Plan
===================

🔍 To Validate:

  + api
    Path:     services/api
    Language: go
    Reason:   files changed
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
    Steps:  2
            - Build image
            - Push image

Plan: 1 to validate, 1 to deploy, 0 unchanged
```

## 7. Apply Changes

Execute the plan:

```bash
bear apply
```

Or do a dry run first:

```bash
bear apply --dry-run
```

## Next Steps

- Learn about [project configuration](../configuration/project.md)
- Understand [change detection](../concepts/change-detection.md)
- Explore [available presets](../configuration/presets.md)
