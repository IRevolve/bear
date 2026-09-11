# bear init

Create `bear.config.yml` in an existing directory, using its folder name as the
project name. Init is noninteractive: it does not prompt, scan for languages, or
select presets automatically. With no flags, it writes a project name and the
default deployment environments, no imported languages or targets, and commented
customization examples.

```bash
bear init                              # Current directory, no presets, dev/int/prd
bear init --environments preprd,prd    # Declare your own environments
bear init -d ./my-project              # Different directory
bear init --lang go,node --target docker   # With presets
bear init --force                      # Overwrite existing
```

## Flags

| Flag | Description |
|------|-------------|
| `--lang <langs>` | Language presets (comma-separated) |
| `--target <targets>` | Target presets (comma-separated) |
| `--environments <names>` | Deployment environments to declare (comma-separated; default `dev,int,prd`) |
| `--force` | Overwrite existing config |

## Environments

The generated config declares the project's deployment environments, because
[`environments` is required](../configuration.md#deployment-environments):

```yaml title="bear.config.yml"
name: my-platform
environments:
  - dev
  - int
  - prd
languages: {}
```

`dev,int,prd` is only the default. `--environments` takes a comma-separated list or
repeats:

```bash
bear init --environments preprd,prd
bear init --environments dev --environments uat --environments prd
```

Declaration order is preserved in the generated file, and carries no promotion
semantics. Each name must match `[a-z][a-z0-9-]*` and be 1 to 32 characters, and
duplicates are rejected. An unloadable config is worse than no config, so init
validates the list **before** writing anything and refuses rather than leaving a
broken file behind:

```text
Error: --environments: invalid environment name "PRD": use 1 to 32 characters matching [a-z][a-z0-9-]*
Error: --environments: duplicate environment "prd"
Error: --environments must list at least one deployment environment, for example dev,int,prd
```

Init's closing "Next steps" repeats the list it wrote, so the first
`bear plan <environment>` uses a name that exists:

```text
  4. Run 'bear plan <environment>' to plan deployments (dev, int, prd)
```

Requested presets are validated before the config is written. When imports are
requested, the generated `use` block includes Bear's built-in immutable revision,
currently `dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a`. Valid cached presets can be
used offline; uncached imports normally require network access. Init has no
`--revision` flag: edit `use.revision` in the generated file to select another SHA,
then run `bear doctor`.

Existing config is refused unless `--force` is given. Init does not create the
project directory, initialize Git, or generate artifact/library definitions. It
attempts to append `.bear/` to an existing `.gitignore` if that text is missing;
it does not create `.gitignore`. Review ignore rules and add artifact definitions,
deployment allowlists, and toolchains before planning. Each artifact allowlist must
be a subset of the environments declared here.
