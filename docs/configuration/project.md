# Project Configuration

The main configuration file is `bear.config.toml` in your project root.

## Minimal Config

Using presets (recommended):

```toml
name = "my-project"

[use]
languages = ["go", "node"]
targets = ["docker", "cloudrun"]
```

## Full Config Example

```toml
name = "my-platform"

# Import presets from bear-presets repository
[use]
languages = ["go", "node", "python"]
targets = ["docker", "cloudrun", "kubernetes"]

# Custom languages (extend or override presets)
[languages.custom-lang]
detection = { files = ["custom.config"] }
vars = { OUTPUT_DIR = "dist" }
steps = [
  { name = "Install", run = "custom-install" },
  { name = "Lint", run = "custom-lint" },
  { name = "Test", run = "custom-test" },
  { name = "Build", run = "custom-build -o $OUTPUT_DIR" },
]

# Custom targets (extend or override presets)
[targets.custom-deploy]
vars = { REGION = "us-east-1" }
steps = [
  { name = "Deploy", run = "custom-deploy --region $REGION" },
]
```

## Configuration Reference

### `name`

**Required.** The project name.

```toml
name = "my-platform"
```

### `use`

Import presets from the [bear-presets](https://github.com/irevolve/bear-presets) repository.

```toml
[use]
languages = ["go", "node", "python"]
targets = ["docker", "cloudrun"]
```

### `languages`

Define or override language configurations.

```toml
[languages.go]
detection = { files = ["go.mod"] }     # Files that identify this language
vars = { KEY = "value" }               # Default variables (optional)
steps = [
  { name = "Step Name", run = "command" },
]
```

Each step has:

| Field | Description |
|-------|-------------|
| `name` | Display name for the step |
| `run` | Shell command to execute |

### `targets`

Define or override deployment targets.

```toml
[targets.cloudrun]
vars = { REGION = "europe-west1", MEMORY = "512Mi" }
steps = [
  { name = "Build", run = "docker build -t gcr.io/$PROJECT/$NAME:$VERSION ." },
  { name = "Push", run = "docker push gcr.io/$PROJECT/$NAME:$VERSION" },
  { name = "Deploy", run = "gcloud run deploy $NAME --image gcr.io/$PROJECT/$NAME:$VERSION" },
]
```

## Variables

These variables are available in all commands:

| Variable | Description |
|----------|-------------|
| `$NAME` | Artifact name |
| `$VERSION` | Short commit hash (7 chars) |
| Custom | From language `vars`, target `vars`, and artifact `vars` |

### Variable Precedence

1. Artifact `vars` (highest priority)
2. Target `vars`
3. Language `vars`
4. Auto-vars: `$NAME`, `$VERSION`

OS environment variables are available implicitly via the shell.

## Precedence

When both presets and custom configs define the same language/target:

1. Custom config takes precedence
2. Presets are used as fallback
