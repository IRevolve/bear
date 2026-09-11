# bear doctor

Validate config, languages, targets, artifacts, environments, dependencies, circular
dependency detection.

```bash
bear doctor
bear doctor -d ./my-project
```

Loads strict YAML and resolves configured presets, then checks language detection,
target references, declared environments, duplicate artifact/library names, missing
dependencies, and cycles. Unknown fields, ambiguous language matches, and invalid
explicit language overrides are errors. No detected language is a warning, not a
validation runner.

## Environments

Doctor is how you read a project's deployment environments without opening
`bear.config.yml`. It prints the declared list, in declaration order, above the
verdict:

```text
  Environments: dev, int, prd

  All checks passed!
```

Loading the config already rejects a missing, empty, malformed, or duplicated
declaration, so by the time this line prints, the list is valid. What doctor adds is
the rule [`bear plan`](plan.md) and [`bear validate`](validate.md) enforce through
the artifact graph: **every entry of an artifact's `environments` allowlist must be
declared by the project**. Doctor applies it per artifact, so one run lists every
offender instead of stopping at the first:

```text
  Environments: preprd, prd

  Errors:
    • artifact "api" allows undeclared environment "dev"; bear.config.yml declares preprd, prd
    • artifact "web" allows undeclared environment "int"; bear.config.yml declares preprd, prd

  Doctor found 2 error(s)
```

Deployment history in `bear.lock.yml` under an environment the project no longer
declares is **not** reported — not as an error and not as a warning. Retiring an
environment hides its history from `bear list`, it does not make the project
invalid.

Doctor does not execute configured test/build/deploy steps or verify external
credentials and runtime compatibility. Preset resolution may require network
access unless the selected revision is cached (or locally overridden). Discovery
respects the built-in exclusions and project `ignore_dirs`.
</content>
