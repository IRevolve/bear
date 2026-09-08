# Configuration

## bear.config.yml

The main config in your Bear project root, which may be a subdirectory of the Git
repository. Select another project root with `-d`.

### Minimal (with presets)

```yaml
name: my-platform

use:
  revision: dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a
  languages: [go, node]
  targets: [docker, cloudrun]
```

### Presets With Local Overrides

```yaml
name: my-platform

ignore_dirs: [coverage, fixtures/generated-services]

# Use presets for common setups
use:
  revision: dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a
  languages: [go, node, python]
  targets: [docker, cloudrun]

# Override or add custom languages
languages:
  go:
    detection:
      files: [go.mod]
    vars:
      COVERAGE: "80"
    steps:
      - name: Test
        run: go test -race -cover ./...
      - name: Build
        run: go build -o dist/app .

# Override or add custom targets
targets:
  staging:
    vars:
      REGION: europe-west1
    steps:
      - name: Build
        run: docker build -t gcr.io/$PROJECT/$NAME:$VERSION .
      - name: Push
        run: docker push gcr.io/$PROJECT/$NAME:$VERSION
      - name: Deploy
        run: gcloud run deploy $NAME --image gcr.io/$PROJECT/$NAME:$VERSION --region $REGION
```

Local definitions replace the **entire** preset with the same name; fields and
step lists are not merged. A locally defined language/target bypasses that preset's
download. Include every detection rule, variable, and step you need in the override.

### Fields And Validation

The supported top-level fields are `name`, `ignore_dirs`, `use`, `languages`, and
`targets`. `use` accepts only `revision`, `languages`, and `targets`. Local languages
accept `detection`, `vars`, and `steps`; targets accept `vars` and `steps`. Their
names come from map keys, not nested `name` fields. Each step has `name` and `run`.

Project, artifact, and library configs accept exactly one YAML document and reject
unknown fields, including misspellings and obsolete keys. Project names, local
language/target names, artifact/library names, dependency names, and each step's
`name`/`run` must not be blank. Artifacts require a nonblank target. Run `bear check`
to catch unknown target references, duplicate artifact/library names, missing
dependencies, and cycles; artifact and library names share one namespace.

Imported preset names must start with a letter or digit, followed by letters,
digits, hyphens, or underscores. These preset-name restrictions are not a general
restriction on all local artifact/project names. `use.revision`, when provided,
must be an immutable 40-character lowercase hexadecimal commit SHA, not a tag,
branch, or abbreviated SHA.

Local language definitions require `steps`; use `steps: []` explicitly for no
validation. Targets must resolve to nonempty deployment steps. Upstream phased
`validation` and `defaults`/`deploy` schemas are normalized by the preset importer,
not accepted as local language/target fields.

### Discovery Exclusions

Bear scans recursively for `bear.artifact.yml` and `bear.lib.yml`. It skips
`.git`, `.bear`, `node_modules`, `vendor`, `generated`, `dist`, and `build`
directories by default. `ignore_dirs` adds exclusions; it does not replace defaults.
A bare name such as `coverage` excludes that directory name at any depth; a path
such as `fixtures/generated-services` matches relative to the project root.
Entries are literal directory names/paths, not glob patterns, and must be relative
without `..` components or backslashes. The project root itself is not skipped.

These are artifact-discovery rules, not source-fingerprint exclusions or Git ignore
rules. Nonignored source in these directories can still affect clean-source checks
and fingerprints. See [Source Safety](concepts/plan-apply.md#source-safety).

---

## bear.artifact.yml

Place in each deployable service directory.

```yaml
name: user-api             # Unique name
target: cloudrun            # Target from config
language: go                # Explicit configured language (optional)
depends: [shared-lib]       # Dependencies (optional)
environments: [dev, int]    # Allow deployment only in these environments

vars:                       # Override variables (optional)
  PROJECT: my-gcp-project
  MEMORY: 1Gi
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | ✓ | Unique artifact name |
| `target` | ✓ | Deployment target (from config or presets) |
| `language` | | Explicit configured language; bypass automatic detection |
| `depends` | | Dependencies (artifact/library names) |
| `environments` | | Deployment allowlist: `dev`, `int`, `prd`; absent or `[]` disables all deployment |
| `vars` | | Variables passed to all steps |

### Environment Deployment Policy

`environments` is a deployment allowlist. Omitting it or using `[]` disables deployment
in every environment, but retains validation. Use `[dev, int, prd]` to allow all three.
Environment names are case-sensitive and must be `dev`, `int`, or `prd`. Unknown names
are rejected. Every plan requires an explicit environment as the first positional
argument, including validation-only plans and plans selecting unchanged or pinned artifacts:

```bash
bear plan dev user-api # user-api can deploy
bear plan prd user-api # user-api validates, but deployment is skipped
bear apply            # Executes the saved plan, including its environment decision
```

Only the positional environment argument selects deployment policy. Inherited or configured `ENVIRONMENT`
variables cannot satisfy this requirement. Bear does not infer the environment from
job/process variables, branch names, or CI metadata. Selecting an environment does
not enable deployment for artifacts with absent or empty allowlists. The environment
controls deployment eligibility and is injected as `ENVIRONMENT` into all validation
and deployment steps, including validation-only plans. It does not automatically
select targets or credentials; steps can use the variable for that purpose.

Disabled artifacts still run applicable language validation steps and propagate
changes to dependents, including transitive dependents. Each dependent's own
deployment policy is evaluated independently. Deployment skips include an explicit
reason. Neither `--pin` nor `--force` overrides this policy. Skipped deployments do
not create or update lock entries.

The environment, allowlists, and resulting deploy/skip decisions are snapshotted in `.bear/plan.yml`.
`bear apply` does not re-evaluate policy. Re-run `bear plan <environment> [artifacts...]` after
changing policy or the intended environment, including changes made after approval.
Old saved plans lacking the allowlist snapshot must be regenerated before apply.

This is a breaking change: replace `disabled_environments` with `environments` listing
the environments that are **allowed**, not the ones previously denied. The old denylist
is not supported. Existing artifacts without an allowlist no longer deploy.

---

## bear.lib.yml

Place in shared library directories. Libraries are validated but never deployed.
Their changes can trigger dependent validation/deployment, subject to artifact
selection, pins, and each dependent's environment policy.

```yaml
name: shared-lib
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | ✓ | Unique library name |
| `language` | | Explicit configured language; bypass automatic detection |
| `depends` | | Dependencies on other libraries |

Library config accepts only `name`, `language`, and `depends`; deployment targets,
allowlists, and `vars` belong to artifact definitions, not libraries.

---

## Languages

Languages define **validation steps** — what runs before deployment (tests, linting, builds).

Bear detects languages from configured `detection.files` (any listed file) or
`detection.pattern` (a valid glob), relative to the artifact/library directory.
Rules must be relative, without `..` components or backslashes. Detection only
uses configured/imported languages; it does not fetch additional presets.

If multiple languages match, scanning fails with an ambiguity error rather than
choosing one. Set `language: <configured-name>` in `bear.artifact.yml` or
`bear.lib.yml` to resolve it, for example when Node and TypeScript rules overlap.
An explicit language must be configured, but need not match its detection rules.
No match yields `unknown`; `bear check` warns, and no language validation steps run.

```yaml
languages:
  go:
    detection:
      files: [go.mod]             # Any of these files → this language
    vars:                          # Default variables (optional)
      KEY: value
    steps:
      - name: Test
        run: go test ./...
      - name: Build
        run: go build -o app .
```

### Preset Languages

Available names include `go`, `node`, `typescript`, `python`, `rust`, and `java`.
Exact detection rules and commands depend on the selected revision. Inspect them
with `bear preset show language <name> --revision <sha>` before use. At the default
SHA, Python and Java use Bear's maintained corrections rather than the upstream
failure-masking commands; see [Maintained Corrections](commands/preset.md#maintained-corrections).

---

## Targets

Targets define **deployment steps** — what runs to deploy an artifact.

```yaml
targets:
  cloudrun:
    vars:
      REGION: europe-west1
      MEMORY: 512Mi
    steps:
      - name: Build
        run: docker build -t gcr.io/$PROJECT/$NAME:$VERSION .
      - name: Push
        run: docker push gcr.io/$PROJECT/$NAME:$VERSION
      - name: Deploy
        run: gcloud run deploy $NAME --image gcr.io/$PROJECT/$NAME:$VERSION --region $REGION
```

### Preset Targets

Available targets include `docker`, `cloudrun`, `cloudrun-job`, `kubernetes`,
`helm`, `lambda`, and `s3-static`. Inspect the effective defaults and commands with
`bear preset show target <name> --revision <sha>`; required variables, files,
toolchains, credentials, and runtime packaging depend on those commands. A target
name alone does not make an arbitrary application compatible with that platform.

---

## Variables

Variables are passed to steps as shell environment variables:

| Variable | Source |
|----------|--------|
| `$ENVIRONMENT` | Injected from the required positional environment argument (validation + deployment); not an input for policy selection |
| `$NAME` | Artifact name (auto, deployment only) |
| `$VERSION` | Short commit hash, 7 chars (auto, deployment only) |
| Custom | From `vars` in language, target, or artifact |

**Precedence** (highest wins):

1. `$ENVIRONMENT` injected from the positional environment argument; automatic `$NAME` and `$VERSION` for deployment
2. Artifact `vars`
3. Target `vars`
4. Language `vars`
5. Inherited OS environment

Inherited job/process `ENVIRONMENT` variables and artifact, target, or language `vars`
do not select deployment policy or satisfy the required positional argument.
Bear always injects the selected `ENVIRONMENT`, including during validation-only planning,
overriding configured or inherited values. The selection is saved with deployment variables,
so a different process environment during `bear apply` cannot override it.

Use it directly in steps or reference it from other variables:

```yaml
targets:
  kubernetes:
    vars:
      NAMESPACE: "myapp-${ENVIRONMENT}"
    steps:
      - name: Deploy
        run: kubectl apply -n "$NAMESPACE" -f "deploy/$ENVIRONMENT.yml"
```

---

## Presets

Presets come from [bear-presets](https://github.com/irevolve/bear-presets) at an
immutable revision. Pin it explicitly in project config:

```yaml
use:
  revision: dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a
  languages: [go, python]
  targets: [cloudrun]
```

Omitting `use.revision` selects Bear's compiled default, currently the SHA above.
An explicit SHA keeps the project selection stable across Bear upgrades. Cache
files live in `~/.bear/presets/<revision>/`, do not expire, and can be reused offline
when present and valid. Missing/invalid files require fetching and validation.

```bash
bear preset list              # Show all presets
bear preset show language go  # Show language details
bear preset show target docker # Show target details
bear preset update            # Populate/repair the default revision's cache
bear preset update --revision dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a
```

Preset CLI commands use their own `--revision`, not project `use.revision`, and do
not modify the project selection. Update downloads and validates the complete
revision before publishing files atomically, preserving already-valid cached files.
It does not advance to latest upstream or replace the entire cache transactionally.
See [Preset Commands](commands/preset.md) for offline behavior and the exact-SHA
Python/Java corrections. Other custom revisions and local commands are not rewritten.

Replace any preset with a complete local definition:

```yaml
use:
  languages: [go]      # Use go preset

languages:
  go:                   # Override with custom steps
    detection:
      files: [go.mod]
    steps:
      - name: Test
        run: go test -cover ./...
```

---

## Dependencies

Dependencies are resolved **transitively**. If shared-lib changes, everything that depends on it rebuilds:

```mermaid
flowchart TB
    A["shared-lib (changed)"] --> B["user-api → rebuild + redeploy"]
    A --> C["dashboard → rebuild + redeploy"]
```

Bear detects and rejects circular dependencies. Run `bear check` to validate.
Dependency-triggered redeployment still requires each dependent's own allowlist to
include the selected environment.

```bash
bear list --tree    # Visualize dependency tree
```

---

## Lock File

`bear.lock.yml` tracks deployed versions and pins per environment and artifact.
It is auto-managed by Bear.

```yaml title="bear.lock.yml"
environments:
  dev:
    user-api:
      commit: abc1234567890
      timestamp: "2026-01-04T10:00:00Z"
      version: abc1234
      target: cloudrun
    order-api:
      commit: def4567890123
      timestamp: "2026-01-03T15:30:00Z"
      version: def4567
      target: cloudrun
      pinned: true
```

During `bear apply`, each successful deployment atomically saves lock history and
then checkpoints completion in the plan. After all deployments succeed, Bear
commits the lock with `[skip ci]` and pushes it. Use `--no-commit` to skip commit
and push, retaining local history for your own publication process.

Legacy top-level `artifacts` entries have no environment provenance. They are
retained but ignored, not automatically mapped; replan and review each environment.
Regenerate old plans missing source or policy snapshots. See
[Lock File](concepts/lock-file.md) and [Retries](concepts/plan-apply.md#retries).

---

## Pinning

Pin an artifact to a specific commit to prevent redeployment or to rollback:

```bash
# Rollback to a known-good version
bear plan dev user-api --pin abc1234
bear apply

# Future plans skip pinned artifacts
bear plan dev # Shows: user-api  pinned

# Unpin and deploy latest
bear plan dev user-api --force
bear apply
```

These examples require `dev` in `user-api`'s `environments` allowlist. Neither pinning
nor forcing enables deployment when the allowlist is absent, empty, or excludes `dev`.

Pins use private worktrees at the selected commit. Plan validates there; apply
recreates the worktree, reruns saved validation/setup, and verifies the fingerprint
before pending deployments. Current configuration supplies the snapshotted policy
and steps. See [Pinning](concepts/pinning.md) for source and rollback limits.
