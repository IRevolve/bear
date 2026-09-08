# bear check

Validate config, languages, targets, artifacts, dependencies, circular dependency detection.

```bash
bear check
bear check -d ./my-project
```

Loads strict YAML and resolves configured presets, then checks language detection,
target references, duplicate artifact/library names, missing dependencies, and
cycles. Unknown fields, ambiguous language matches, and invalid explicit language
overrides are errors. No detected language is a warning, not a validation runner.

Check does not execute configured test/build/deploy steps or verify external
credentials and runtime compatibility. Preset resolution may require network
access unless the selected revision is cached (or locally overridden). Discovery
respects the built-in exclusions and project `ignore_dirs`.
