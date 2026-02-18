# Languages

Languages define how Bear validates your code before deployment. Each language specifies detection rules and validation steps.

## Using Preset Languages

The easiest way is to use preset languages:

```toml title="bear.config.toml"
name = "my-project"

[use]
languages = ["go", "node", "python"]
```

See [Presets](presets.md) for available preset languages.

## Defining Custom Languages

Define languages in your `bear.config.toml`:

```toml title="bear.config.toml"
name = "my-project"

[languages.go]
detection = { files = ["go.mod"] }
vars = { COVERAGE_THRESHOLD = "80" }
steps = [
  { name = "Download modules", run = "go mod download" },
  { name = "Vet", run = "go vet ./..." },
  { name = "Staticcheck", run = "staticcheck ./..." },
  { name = "Test", run = "go test -race -cover ./..." },
  { name = "Build", run = "go build -o dist/app ." },
]
```

## Language Structure

```toml
[languages.language-name]
detection = { files = ["file1", "file2"] }  # How to detect this language
vars = { KEY = "value" }                     # Default variables (optional)
steps = [
  { name = "Step Name", run = "command" },   # Validation steps
]
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `detection.files` | ✓ | Files that identify this language |
| `vars` | | Default variables for steps |
| `steps` | ✓ | List of validation steps |

### Detection

Bear detects an artifact's language by looking for specific files in the artifact directory:

```toml
[languages.go]
detection = { files = ["go.mod"] }  # If go.mod exists, it's a Go project
```

Multiple files can be specified (any match triggers detection):

```toml
[languages.node]
detection = { files = ["package.json", "yarn.lock", "pnpm-lock.yaml"] }
```

### Steps

Steps run sequentially in order. Define exactly the steps you need:

```toml
[languages.node]
detection = { files = ["package.json"] }
steps = [
  { name = "Install", run = "npm ci" },
  { name = "Lint", run = "npm run lint" },
  { name = "Test", run = "npm test" },
  { name = "Build", run = "npm run build" },
]
```

### Step Fields

Each step has:

| Field | Required | Description |
|-------|----------|-------------|
| `name` | ✓ | Display name shown during execution |
| `run` | ✓ | Shell command to execute |

### Variables

Language `vars` provide defaults that can be used in steps and overridden by target or artifact vars.

Variable precedence (highest to lowest):

1. Artifact `vars` (highest priority)
2. Target `vars`
3. Language `vars`
4. Auto-vars (`$NAME`, `$VERSION`)

## Language Examples

### Go

```toml
[languages.go]
detection = { files = ["go.mod"] }
steps = [
  { name = "Download modules", run = "go mod download" },
  { name = "Vet", run = "go vet ./..." },
  { name = "Staticcheck", run = "staticcheck ./..." },
  { name = "Test", run = "go test -race -cover ./..." },
  { name = "Build", run = "go build -o dist/app ." },
]
```

### Node.js

```toml
[languages.node]
detection = { files = ["package.json"] }
steps = [
  { name = "Install dependencies", run = "npm ci" },
  { name = "ESLint", run = "npm run lint" },
  { name = "Test", run = "npm test" },
  { name = "Build", run = "npm run build" },
]
```

### TypeScript

```toml
[languages.typescript]
detection = { files = ["tsconfig.json"] }
steps = [
  { name = "Install dependencies", run = "npm ci" },
  { name = "Type check", run = "npx tsc --noEmit" },
  { name = "ESLint", run = "npm run lint" },
  { name = "Test", run = "npm test" },
  { name = "Build", run = "npm run build" },
]
```

### Python

```toml
[languages.python]
detection = { files = ["requirements.txt", "pyproject.toml"] }
steps = [
  { name = "Create venv", run = "python -m venv .venv" },
  { name = "Install dependencies", run = ".venv/bin/pip install -r requirements.txt" },
  { name = "Ruff", run = ".venv/bin/ruff check ." },
  { name = "Mypy", run = ".venv/bin/mypy ." },
  { name = "Pytest", run = ".venv/bin/pytest" },
]
```

### Rust

```toml
[languages.rust]
detection = { files = ["Cargo.toml"] }
steps = [
  { name = "Check", run = "cargo check" },
  { name = "Clippy", run = "cargo clippy -- -D warnings" },
  { name = "Format check", run = "cargo fmt --check" },
  { name = "Test", run = "cargo test" },
  { name = "Build release", run = "cargo build --release" },
]
```

### Java (Maven)

```toml
[languages.java]
detection = { files = ["pom.xml"] }
steps = [
  { name = "Compile", run = "mvn compile" },
  { name = "Test", run = "mvn test" },
  { name = "Package", run = "mvn package -DskipTests" },
]
```

### Java (Gradle)

```toml
[languages.java-gradle]
detection = { files = ["build.gradle", "build.gradle.kts"] }
steps = [
  { name = "Compile", run = "./gradlew compileJava" },
  { name = "Test", run = "./gradlew test" },
  { name = "Build", run = "./gradlew build -x test" },
]
```

## Multiple Linters

Add multiple steps for thorough checking:

```toml
[languages.go]
detection = { files = ["go.mod"] }
steps = [
  { name = "Format check", run = "gofmt -l ." },
  { name = "Vet", run = "go vet ./..." },
  { name = "Staticcheck", run = "staticcheck ./..." },
  { name = "Golangci-lint", run = "golangci-lint run" },
]
```

## Conditional Steps

Use shell conditionals for optional steps:

```toml
[languages.node]
detection = { files = ["package.json"] }
steps = [
  { name = "Install if needed", run = """
if [ ! -d "node_modules" ]; then
  npm ci
fi
""" },
]
```

## Step Failure

If any step fails:

1. Validation stops immediately
2. The artifact is NOT deployed
