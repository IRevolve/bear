# bear init

Create a `bear.config.yml` with auto-detected languages.

```bash
bear init                              # Current directory
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
