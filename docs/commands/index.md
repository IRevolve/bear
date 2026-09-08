# Commands

| Command | Description |
|---------|-------------|
| [`bear init`](init.md) | Initialize a new project |
| [`bear plan <environment> [artifacts...]`](plan.md) | Detect changes, validate, create plan; `dev`, `int`, or `prd` is required even for validation-only plans |
| [`bear apply`](apply.md) | Execute the deployment plan |
| [`bear check`](check.md) | Validate config and dependencies |
| [`bear list`](list.md) | List all artifacts |
| [`bear preset`](preset.md) | Manage presets |

## Global Flags

| Flag | Description |
|------|-------------|
| `-d, --dir <path>` | Project directory (default: `.`) |
| `-f, --force` | Force operation, ignore pins |
| `-v, --verbose` | Stream subprocess output; failure diagnostics retain a bounded tail |
