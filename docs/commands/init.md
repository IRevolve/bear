# bear init

Create `bear.config.yml` in an existing directory, using its folder name as the
project name. Init is noninteractive: it does not prompt, scan for languages, or
select presets automatically. With no flags, it writes an empty configuration
(project name, no imported languages or targets) with commented customization examples.

```bash
bear init                              # Empty config in current directory
bear init -d ./my-project              # Different directory
bear init --lang go,node --target docker   # With presets
bear init --force                      # Overwrite existing
```

## Flags

| Flag | Description |
|------|-------------|
| `--lang <langs>` | Language presets (comma-separated) |
| `--target <targets>` | Target presets (comma-separated) |
| `--force` | Overwrite existing config |

Requested presets are validated before the config is written. When imports are
requested, the generated `use` block includes Bear's built-in immutable revision,
currently `dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a`. Valid cached presets can be
used offline; uncached imports normally require network access. Init has no
`--revision` flag: edit `use.revision` in the generated file to select another SHA,
then run `bear check`.

Existing config is refused unless `--force` is given. Init does not create the
project directory, initialize Git, or generate artifact/library definitions. It
attempts to append `.bear/` to an existing `.gitignore` if that text is missing;
it does not create `.gitignore`. Review ignore rules and add artifact definitions,
deployment allowlists, and toolchains before planning.
