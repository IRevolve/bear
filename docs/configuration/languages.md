# Languages

Languages define how Bear validates your code before deployment. Each language specifies detection rules and validation steps.

## Using Preset Languages

The easiest way is to use preset languages:

```yaml title="bear.config.yml"
name: my-project

use:
  languages: [go, node, python]
```

See [Presets](presets.md) for available preset languages.

## Defining Custom Languages

Define languages in your `bear.config.yml`:

```yaml title="bear.config.yml"
name: my-project

languages:
  - name: go
    detection:
      files: [go.mod]
    validation:
      setup:
        - name: Download modules
          run: go mod download
      lint:
        - name: Vet
          run: go vet ./...
        - name: Staticcheck
          run: staticcheck ./...
      test:
        - name: Test
          run: go test -race -cover ./...
      build:
        - name: Build
          run: go build -o dist/app .
```

## Language Structure

```yaml
languages:
  - name: language-name         # Unique identifier
    detection:                  # How to detect this language
      files: [file1, file2]     # Files that identify this language
    validation:                 # Validation steps
      setup: [...]              # Setup/install steps
      lint: [...]               # Linting steps
      test: [...]               # Test steps
      build: [...]              # Build steps
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | ✓ | Unique language identifier |
| `detection.files` | ✓ | Files that identify this language |
| `validation` | ✓ | Validation configuration |

### Detection

Bear detects an artifact's language by looking for specific files in the artifact directory:

```yaml
detection:
  files: [go.mod]  # If go.mod exists, it's a Go project
```

Multiple files can be specified (any match triggers detection):

```yaml
detection:
  files: [package.json, yarn.lock, pnpm-lock.yaml]
```

### Validation Phases

Validation runs in order: `setup` → `lint` → `test` → `build`

| Phase | Purpose | Example |
|-------|---------|---------|
| `setup` | Install dependencies | `npm install`, `go mod download` |
| `lint` | Static analysis | `eslint`, `go vet`, `pylint` |
| `test` | Run tests | `npm test`, `go test`, `pytest` |
| `build` | Build artifacts | `npm run build`, `go build` |

All phases are optional. Skip phases you don't need:

```yaml
validation:
  test:
    - name: Test
      run: go test ./...
  # No setup, lint, or build phases
```

### Validation Steps

Each step has:

| Field | Required | Description |
|-------|----------|-------------|
| `name` | ✓ | Display name shown during execution |
| `run` | ✓ | Shell command to execute |

## Language Examples

### Go

```yaml
- name: go
  detection:
    files: [go.mod]
  validation:
    setup:
      - name: Download modules
        run: go mod download
    lint:
      - name: Vet
        run: go vet ./...
      - name: Staticcheck
        run: staticcheck ./...
    test:
      - name: Test
        run: go test -race -cover ./...
    build:
      - name: Build
        run: go build -o dist/app .
```

### Node.js

```yaml
- name: node
  detection:
    files: [package.json]
  validation:
    setup:
      - name: Install dependencies
        run: npm ci
    lint:
      - name: ESLint
        run: npm run lint
    test:
      - name: Test
        run: npm test
    build:
      - name: Build
        run: npm run build
```

### TypeScript

```yaml
- name: typescript
  detection:
    files: [tsconfig.json]
  validation:
    setup:
      - name: Install dependencies
        run: npm ci
    lint:
      - name: Type check
        run: npx tsc --noEmit
      - name: ESLint
        run: npm run lint
    test:
      - name: Test
        run: npm test
    build:
      - name: Build
        run: npm run build
```

### Python

```yaml
- name: python
  detection:
    files: [requirements.txt, pyproject.toml]
  validation:
    setup:
      - name: Create venv
        run: python -m venv .venv
      - name: Install dependencies
        run: .venv/bin/pip install -r requirements.txt
    lint:
      - name: Ruff
        run: .venv/bin/ruff check .
      - name: Mypy
        run: .venv/bin/mypy .
    test:
      - name: Pytest
        run: .venv/bin/pytest
```

### Rust

```yaml
- name: rust
  detection:
    files: [Cargo.toml]
  validation:
    setup:
      - name: Check
        run: cargo check
    lint:
      - name: Clippy
        run: cargo clippy -- -D warnings
      - name: Format check
        run: cargo fmt --check
    test:
      - name: Test
        run: cargo test
    build:
      - name: Build release
        run: cargo build --release
```

### Java (Maven)

```yaml
- name: java
  detection:
    files: [pom.xml]
  validation:
    lint:
      - name: Compile
        run: mvn compile
    test:
      - name: Test
        run: mvn test
    build:
      - name: Package
        run: mvn package -DskipTests
```

### Java (Gradle)

```yaml
- name: java-gradle
  detection:
    files: [build.gradle, build.gradle.kts]
  validation:
    lint:
      - name: Compile
        run: ./gradlew compileJava
    test:
      - name: Test
        run: ./gradlew test
    build:
      - name: Build
        run: ./gradlew build -x test
```

## Multiple Linters

Add multiple lint steps for thorough checking:

```yaml
validation:
  lint:
    - name: Format check
      run: gofmt -l .
    - name: Vet
      run: go vet ./...
    - name: Staticcheck
      run: staticcheck ./...
    - name: Golangci-lint
      run: golangci-lint run
```

## Conditional Steps

Use shell conditionals for optional steps:

```yaml
validation:
  setup:
    - name: Install if needed
      run: |
        if [ ! -d "node_modules" ]; then
          npm ci
        fi
```

## Step Failure

If any step fails:

1. Validation stops immediately
2. The artifact is NOT deployed
3. Bear reports the failed step and error

!!! tip "Fail Fast"
    Put quick checks (like linting) before slow steps (like tests) to fail fast.

## See Also

- [Presets](presets.md) — Pre-built language configurations
- [Artifacts](artifacts.md) — Language detection in artifacts
- [bear plan](../commands/plan.md) — Validation and planning
