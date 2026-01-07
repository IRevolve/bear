# bear preset

Manage presets from the community repository.

## Usage

```bash
bear preset <command> [flags]
```

## Commands

| Command | Description |
|---------|-------------|
| `list` | Show all available presets |
| `show <type> <name>` | Show preset details |
| `update` | Refresh preset cache from GitHub |

## bear preset list

Show all available languages and targets:

```bash
bear preset list
```

```
📦 Available Presets
====================

Languages:
  • go
  • java
  • node
  • python
  • rust
  • typescript

Targets:
  • cloudrun
  • cloudrun-job
  • docker
  • helm
  • kubernetes
  • lambda
  • s3
  • s3-static

Usage in bear.config.yml:
  use:
    languages: [go, node]
    targets: [docker, cloudrun]
```

## bear preset show

Show details for a specific preset:

```bash
# Show language preset
bear preset show language go

# Show target preset
bear preset show target docker
```

### Language Output

```
📝 Language: go
───────────────────────

Detection:
  files: [go.mod]

Validation:
  setup:
    - Download modules: go mod download
  lint:
    - Vet: go vet ./...
  test:
    - Test: go test -race ./...
  build:
    - Build: go build -o dist/app .
```

### Target Output

```
🎯 Target: docker
───────────────────────

Defaults:
  REGISTRY: docker.io

Deploy:
  - Build image: docker build -t $REGISTRY/$NAME:$VERSION .
  - Push image: docker push $REGISTRY/$NAME:$VERSION
```

## bear preset update

Force refresh the preset cache from GitHub:

```bash
bear preset update
```

```
🔄 Updating presets from GitHub...
✅ Presets updated successfully!
```

## Cache

Presets are cached locally in `~/.bear/presets/` for 24 hours. The cache is automatically refreshed when:

- Running `bear preset update`
- Cache files are older than 24 hours

## See Also

- [Presets Configuration](../configuration/presets.md)
- [bear-presets Repository](https://github.com/irevolve/bear-presets)
