# Project Configuration

The main configuration file is `bear.config.yml` in your project root.

## Minimal Config

Using presets (recommended):

```yaml
name: my-project

use:
  languages: [go, node]
  targets: [docker, cloudrun]
```

## Full Config Example

```yaml
name: my-platform

# Import presets from bear-presets repository
use:
  languages: [go, node, python]
  targets: [docker, cloudrun, kubernetes]

# Custom languages (extend or override presets)
languages:
  - name: custom-lang
    detection:
      files: [custom.config]
    validation:
      setup:
        - name: Install deps
          run: custom-install
      lint:
        - name: Lint
          run: custom-lint
      test:
        - name: Test
          run: custom-test
      build:
        - name: Build
          run: custom-build

# Custom targets (extend or override presets)
targets:
  - name: custom-deploy
    defaults:
      REGION: us-east-1
    deploy:
      - name: Deploy
        run: custom-deploy --region $REGION
```

## Configuration Reference

### `name`

**Required.** The project name.

```yaml
name: my-platform
```

### `use`

Import presets from the [bear-presets](https://github.com/irevolve/bear-presets) repository.

```yaml
use:
  languages: [go, node, python]
  targets: [docker, cloudrun]
```

### `languages`

Define or override language configurations.

```yaml
languages:
  - name: go                      # Language identifier
    detection:
      files: [go.mod]             # Files that identify this language
    validation:
      setup: [...]                # Setup steps (optional)
      lint: [...]                 # Linting steps (optional)
      test: [...]                 # Test steps (optional)
      build: [...]                # Build steps (optional)
```

Each step has:

| Field | Description |
|-------|-------------|
| `name` | Display name for the step |
| `run` | Shell command to execute |

### `targets`

Define or override deployment targets.

```yaml
targets:
  - name: cloudrun                # Target identifier
    defaults:                     # Default parameters
      REGION: europe-west1
      MEMORY: 512Mi
    deploy:                       # Deployment steps
      - name: Build
        run: docker build -t gcr.io/$PROJECT/$NAME:$VERSION .
      - name: Push
        run: docker push gcr.io/$PROJECT/$NAME:$VERSION
      - name: Deploy
        run: gcloud run deploy $NAME --image gcr.io/$PROJECT/$NAME:$VERSION
```

## Variables

These variables are available in all commands:

| Variable | Description |
|----------|-------------|
| `$NAME` | Artifact name |
| `$VERSION` | Short commit hash (7 chars) |
| Custom | From target `defaults` and artifact `env`/`params` |

## Precedence

When both presets and custom configs define the same language/target:

1. Custom config takes precedence
2. Presets are used as fallback
