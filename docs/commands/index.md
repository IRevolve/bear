# Commands

| Command | Description |
|---------|-------------|
| [`bear init`](init.md) | Initialize a new project |
| [`bear validate [artifacts...]`](validate.md) | Run validation steps against the working tree; no environment, no plan, no deployment |
| [`bear plan <environment> [artifacts...]`](plan.md) | Detect changes and decide what would deploy; writes a plan but runs no steps; an environment declared in `bear.config.yml` is required even when nothing will deploy |
| [`bear apply`](apply.md) | Build and deploy the plan |
| [`bear doctor`](doctor.md) | Validate config and dependencies |
| [`bear list`](list.md) | List all artifacts |
| [`bear preset`](preset.md) | Manage presets |

The command chain is [`doctor`](doctor.md) (configuration) →
[`validate`](validate.md) (merge request) → [`plan`](plan.md) (deployment review
and approval) → [`apply`](apply.md) (execution). `bear plan` runs no commands at
all — it is a pure diff against `bear.lock.yml`. `bear apply` is the only command
that runs anything: for every deploying artifact it runs the language's build
steps, then the target's deploy steps. `bear validate` runs those same build
steps on demand, with no environment or deployment involved, so it is the way to
get that assurance before merging, independent of `bear apply` actually running it
again right before every deployment.

## Global Flags

| Flag | Description |
|------|-------------|
| `-d, --dir <path>` | Project directory (default: `.`) |
| `-f, --force` | Force operation, ignore pins |
| `-v, --verbose` | Stream subprocess output; failure diagnostics retain a bounded tail |
