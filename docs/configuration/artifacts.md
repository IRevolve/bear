# Artifacts

Artifacts are the deployable units in your monorepo. Each service, app, or library has its own artifact config.

## Services: `bear.artifact.toml`

Services are validated and deployed.

```toml
name = "user-api"
target = "cloudrun"
depends = ["shared-lib", "auth-lib"]

[vars]
PROJECT = "my-gcp-project"
REGION = "europe-west1"
MEMORY = "1Gi"
CPU = "2"
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | ✓ | Unique artifact name |
| `target` | ✓ | Deployment target (from config) |
| `depends` | | List of dependencies (artifact names) |
| `vars` | | Variables passed to language and target steps |

## Libraries: `bear.lib.toml`

Libraries are validated but not deployed. They're used as dependencies for other artifacts.

```toml
name = "shared-lib"
```

When a library changes, all artifacts that depend on it are marked for rebuild.

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | ✓ | Unique library name |
| `depends` | | List of dependencies (other library names) |

## Discovery

Bear automatically discovers artifacts by scanning for:

- `bear.artifact.toml` — Services
- `bear.lib.toml` — Libraries

Artifacts are discovered recursively from the project root.

## Dependencies

Dependencies are resolved transitively. If A depends on B, and B depends on C:

```
C (changed)
  ↓
B (dependency changed) → revalidate
  ↓
A (dependency changed) → revalidate + redeploy
```

!!! warning "Circular Dependencies"
    Bear detects and rejects circular dependencies. Run `bear check` to validate.

## Language Detection

Bear automatically detects the language of each artifact based on files present:

| Language | Detection Files |
|----------|-----------------|
| Go | `go.mod` |
| Node.js | `package.json` |
| TypeScript | `tsconfig.json` |
| Python | `requirements.txt`, `pyproject.toml` |
| Rust | `Cargo.toml` |
| Java | `pom.xml` |

## Example Structure

```
my-monorepo/
├── bear.config.toml
├── bear.lock.toml              # Auto-generated
├── apps/
│   └── dashboard/
│       ├── bear.artifact.toml
│       ├── package.json
│       └── src/
├── libs/
│   ├── shared-go/
│   │   ├── bear.lib.toml
│   │   └── go.mod
│   └── ui-components/
│       ├── bear.lib.toml
│       └── package.json
└── services/
    ├── user-api/
    │   ├── bear.artifact.toml
    │   ├── go.mod
    │   └── main.go
    └── order-api/
        ├── bear.artifact.toml
        └── go.mod
```
