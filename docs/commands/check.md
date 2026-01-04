# bear check

Validate configuration and dependencies.

## Usage

```bash
bear check [flags]
```

## Description

Performs comprehensive validation of your Bear configuration:

- Config file syntax
- Language definitions
- Target definitions
- Artifact discovery
- Dependency resolution
- Circular dependency detection

## Examples

```bash
# Check configuration
bear check

# Check in a different directory
bear check -d ./my-project
```

## Output

```
🔍 Bear Configuration Check
===========================

📄 Loading config... ✓ my-platform
🔤 Checking languages... ✓ 3 defined
🎯 Checking targets... ✓ 2 defined
📦 Scanning artifacts... ✓ 5 found (3 services, 2 libraries)
🔗 Checking dependencies... ✓ all resolved
🔄 Checking for cycles... ✓ none

✅ All checks passed!
```

## Warnings

Bear may show warnings for non-critical issues:

```
⚠️  Warnings:
   • Artifact 'api' has unknown language
   • Target 'custom' has no default parameters
```

## Errors

If there are errors, Bear will show them and exit with code 1:

```
❌ Errors:
   • Unknown target 'invalid' in artifact 'api'
   • Circular dependency: api → lib → api
   • Missing dependency 'unknown-lib' in artifact 'api'
```

## See Also

- [Project Configuration](../configuration/project.md)
- [Artifacts](../configuration/artifacts.md)
- [Dependencies](../concepts/dependencies.md)
