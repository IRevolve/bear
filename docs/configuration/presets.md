# Presets

Bear loads community presets from [bear-presets](https://github.com/irevolve/bear-presets). This allows you to use pre-built configurations without defining them yourself.

## Using Presets

Add the `use` section to your `bear.config.yml`:

```yaml
name: my-project

use:
  languages: [go, node, python]
  targets: [docker, cloudrun, kubernetes]
```

## Available Languages

| Language | Detection | Validation Steps |
|----------|-----------|------------------|
| `go` | `go.mod` | download, vet, test, build |
| `node` | `package.json` | install, lint, test, build |
| `typescript` | `tsconfig.json` | install, typecheck, lint, test, build |
| `python` | `requirements.txt` | venv, install, lint, test |
| `rust` | `Cargo.toml` | check, clippy, test, build |
| `java` | `pom.xml` | compile, test, package |

## Available Targets

| Target | Description | Required Vars |
|--------|-------------|---------------|
| `docker` | Build and push Docker images | `REGISTRY` |
| `cloudrun` | Deploy to Google Cloud Run | `PROJECT`, `REGION` |
| `cloudrun-job` | Deploy Cloud Run jobs | `PROJECT`, `REGION` |
| `kubernetes` | Apply Kubernetes manifests | `NAMESPACE`, `REGISTRY` |
| `helm` | Deploy with Helm charts | `NAMESPACE`, `REGISTRY` |
| `lambda` | Deploy AWS Lambda functions | `REGION` |
| `s3` | Deploy to S3 buckets | `BUCKET` |
| `s3-static` | Deploy static sites to S3 + CloudFront | `BUCKET`, `CF_DIST` |

## Preset Commands

### List Presets

```bash
bear preset list
```

Output:

```
📦 Available Presets
====================

Languages:
  • go
  • java
  • node
  • python
  • rust
  • typescript

Targets:
  • cloudrun
  • docker
  • kubernetes
  • helm
  • lambda
  • s3
  ...
```

### Show Preset Details

```bash
bear preset show language go
bear preset show target docker
```

### Update Cache

Presets are cached locally for 24 hours. Force refresh:

```bash
bear preset update
```

## Cache Location

Presets are cached in `~/.bear/presets/`:

```
~/.bear/presets/
├── index.yml
├── languages/
│   ├── go.yml
│   ├── node.yml
│   └── ...
└── targets/
    ├── docker.yml
    ├── cloudrun.yml
    └── ...
```

## Custom Presets

You can override presets by defining the same language/target in your config:

```yaml
name: my-project

use:
  languages: [go]
  targets: [docker]

# Override the go preset with custom steps
languages:
  - name: go
    detection:
      files: [go.mod]
    validation:
      test:
        - name: Test with coverage
          run: go test -cover -race ./...
```

## Contributing Presets

Want to add or improve a preset? Contribute to [bear-presets](https://github.com/irevolve/bear-presets)!

See the repository README for contribution guidelines.
