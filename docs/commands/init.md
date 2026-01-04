# bear init

Initialize a new Bear project.

## Usage

```bash
bear init [flags]
```

## Description

Creates a `bear.config.yml` file in the current directory (or specified with `-d`). The command auto-detects languages based on files present in the repository.

## Examples

```bash
# Initialize in current directory
bear init

# Initialize in a different directory
bear init -d ./my-project
```

## Output

```
🐻 Created bear.config.yml

Next steps:
  1. Add bear.artifact.yml to your services/apps
  2. Add bear.lib.yml to your libraries
  3. Run 'bear check' to validate your setup
  4. Run 'bear plan' to see what would be built
```

## Generated Config

The generated config includes:

- Project name (from directory name)
- Commented examples for custom languages and targets

```yaml
name: my-project

# Custom languages (optional, extend or override presets)
# languages:
#   - name: custom-lang
#     detection:
#       files: [custom.config]
#     validation:
#       build:
#         - name: Build
#           run: custom-build

# Custom targets (optional, extend or override presets)
# targets:
#   - name: custom-target
#     defaults:
#       PARAM: value
#     deploy:
#       - name: Deploy
#         run: custom-deploy $PARAM
```

## See Also

- [Project Configuration](../configuration/project.md)
- [Quick Start](../getting-started/quickstart.md)
