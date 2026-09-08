# bear preset

Manage community presets from [bear-presets](https://github.com/irevolve/bear-presets).

```bash
bear preset list               # Show all presets
bear preset show language go   # Language details
bear preset show target docker # Target details
bear preset update             # Populate/repair the selected revision's cache
```

## Revisions

Presets are fetched at an immutable, 40-character lowercase commit SHA, not a
branch or tag. The built-in default is
`dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a`. All three subcommands accept the
preset command's `--revision` flag:

```bash
bear preset list --revision dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a
bear preset show language python --revision dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a
bear preset update --revision dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a
```

These commands do not read `use.revision` from `bear.config.yml`, even with `-d`.
Pass the project's SHA explicitly when inspecting or warming its cache. They do
not change project configuration or advance its revision. Project loading uses
`use.revision`; omitting it selects the default compiled into Bear.

`show` prints the effective language or target in normalized local YAML
(`detection`/`vars`/`steps` as applicable), not the raw upstream document. Language
aliases are `language`, `lang`, and `l`; target aliases are `target` and `t`.

## Cache

Files are cached under `~/.bear/presets/<revision>/`. Valid cached files do not
expire and can be reused offline. Cached bytes are validated before use; missing
or invalid files require a successful download and validation. First use is not
generally offline: cache the selected index/presets in advance.

`update` downloads the index and all listed presets for the selected revision and
validates the complete set before publishing anything. Download or schema failure
leaves the previous cache untouched. Already-valid files are preserved; missing or
invalid files are published with atomic per-file replacement. This is not one
transaction replacing the whole cache directory: a filesystem failure can leave
some missing files populated, but does not remove previously valid cached files.
It does not mean "upgrade to the latest upstream revision."

## Maintained Corrections

At exactly `dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a`, Bear supplies maintained
Python and Java language definitions to fix upstream commands that masked failures
with successful fallbacks. `show` and project imports use these effective
definitions; their language lookup does not require a download. Index lookup and
`update` still use the upstream revision and its cache.

- Python selects requirements installation when `requirements.txt` exists, otherwise
  editable installation. It checks syntax with `compileall`, uses pytest if the
  module is available or unittest discovery otherwise, and runs `python -m build`
  when package metadata exists. It skips package build only when metadata is absent.
  It does not create a virtualenv or run ruff/pylint. Provision pip, build, and any
  chosen test dependencies in your Python environment.
- Java selects Maven when `pom.xml` exists, otherwise Gradle. It runs dependency
  setup, Checkstyle/Gradle check, tests, and package/build. Provision those tools
  and the corresponding project tasks/plugins.

Selection depends on available files/modules, not on whether a command failed.
Installation, validation, test, and build failures propagate. Corrections are
scoped to that exact SHA, not blindly applied to other custom revisions or future
default revisions. Complete local language definitions bypass preset loading and
are not rewritten. Review arbitrary revision commands yourself; importing a preset
does not sandbox or make its shell commands safe.
