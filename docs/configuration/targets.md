# Targets

Targets define where and how your artifacts get deployed. Each target specifies deployment steps and default variables.

## Using Preset Targets

The easiest way is to use preset targets:

```toml title="bear.config.toml"
name = "my-project"

[use]
targets = ["docker", "cloudrun", "kubernetes"]
```

See [Presets](presets.md) for available preset targets.

## Defining Custom Targets

Define targets in your `bear.config.toml`:

```toml title="bear.config.toml"
name = "my-project"

[targets.my-server]
vars = { HOST = "deploy.example.com", USER = "deploy" }
steps = [
  { name = "Build", run = "go build -o app ." },
  { name = "Upload", run = "scp app $USER@$HOST:/opt/app/" },
  { name = "Restart", run = 'ssh $USER@$HOST "systemctl restart app"' },
]
```

## Target Structure

```toml
[targets.target-name]
vars = { PARAM1 = "value1", PARAM2 = "value2" }  # Default variables (optional)
steps = [
  { name = "Step Name", run = "command $PARAM1" }, # Deployment steps
]
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `vars` | | Default variable values |
| `steps` | ✓ | List of deployment steps |

### Deployment Steps

Each step has:

| Field | Required | Description |
|-------|----------|-------------|
| `name` | ✓ | Display name shown during execution |
| `run` | ✓ | Shell command to execute |

## Variables

These variables are available in all deployment commands:

| Variable | Description | Example |
|----------|-------------|---------|
| `$NAME` | Artifact name | `user-api` |
| `$VERSION` | Short commit hash (7 chars) | `abc1234` |
| Target `vars` | From target's `vars` section | `$REGION`, `$BUCKET` |
| Artifact `vars` | From artifact's `vars` section | `$PROJECT`, `$MEMORY` |

### Variable Precedence

Variables are merged with the following precedence (highest wins):

1. Artifact `vars` (highest priority)
2. Target `vars`
3. Language `vars`
4. Auto-vars: `$NAME`, `$VERSION` (lowest priority)

OS environment variables are available implicitly via the shell.

## Target Examples

### Docker Registry

```toml
[targets.docker]
vars = { REGISTRY = "docker.io" }
steps = [
  { name = "Build image", run = "docker build -t $REGISTRY/$NAME:$VERSION ." },
  { name = "Push image", run = "docker push $REGISTRY/$NAME:$VERSION" },
]
```

**Usage in artifact:**

```toml title="services/api/bear.artifact.toml"
name = "api"
target = "docker"

[vars]
REGISTRY = "ghcr.io/myorg"
```

### Google Cloud Run

```toml
[targets.cloudrun]
vars = { REGION = "europe-west1", MEMORY = "512Mi", CPU = "1" }
steps = [
  { name = "Build image", run = "docker build -t gcr.io/$PROJECT/$NAME:$VERSION ." },
  { name = "Push to GCR", run = "docker push gcr.io/$PROJECT/$NAME:$VERSION" },
  { name = "Deploy to Cloud Run", run = """
gcloud run deploy $NAME \
  --image gcr.io/$PROJECT/$NAME:$VERSION \
  --region $REGION \
  --memory $MEMORY \
  --cpu $CPU""" },
]
```

**Usage in artifact:**

```toml title="services/api/bear.artifact.toml"
name = "api"
target = "cloudrun"

[vars]
PROJECT = "my-gcp-project"
MEMORY = "1Gi"
CPU = "2"
```

### Kubernetes

```toml
[targets.kubernetes]
vars = { NAMESPACE = "default" }
steps = [
  { name = "Build image", run = "docker build -t $REGISTRY/$NAME:$VERSION ." },
  { name = "Push image", run = "docker push $REGISTRY/$NAME:$VERSION" },
  { name = "Update deployment", run = """
kubectl set image deployment/$NAME \
  $NAME=$REGISTRY/$NAME:$VERSION \
  -n $NAMESPACE""" },
  { name = "Wait for rollout", run = "kubectl rollout status deployment/$NAME -n $NAMESPACE" },
]
```

### AWS Lambda

```toml
[targets.lambda]
vars = { REGION = "us-east-1", RUNTIME = "provided.al2" }
steps = [
  { name = "Build", run = "GOOS=linux GOARCH=amd64 go build -o bootstrap ." },
  { name = "Package", run = "zip function.zip bootstrap" },
  { name = "Deploy", run = """
aws lambda update-function-code \
  --function-name $FUNCTION_NAME \
  --zip-file fileb://function.zip \
  --region $REGION""" },
]
```

### S3 Static Site

```toml
[targets.s3-static]
vars = { CACHE_CONTROL = "max-age=31536000" }
steps = [
  { name = "Build", run = "npm run build" },
  { name = "Sync to S3", run = "aws s3 sync dist/ s3://$BUCKET/ --delete" },
  { name = "Invalidate CloudFront", run = """
aws cloudfront create-invalidation \
  --distribution-id $DISTRIBUTION_ID \
  --paths "/*" """ },
]
```

### SSH/SCP Deploy

```toml
[targets.ssh-deploy]
vars = { REMOTE_PATH = "/opt/app" }
steps = [
  { name = "Build", run = "go build -o app ." },
  { name = "Upload binary", run = "scp app $SSH_USER@$SSH_HOST:$REMOTE_PATH/app-$VERSION" },
  { name = "Switch symlink", run = 'ssh $SSH_USER@$SSH_HOST "ln -sfn $REMOTE_PATH/app-$VERSION $REMOTE_PATH/app"' },
  { name = "Restart service", run = 'ssh $SSH_USER@$SSH_HOST "sudo systemctl restart myapp"' },
]
```

### Helm Chart

```toml
[targets.helm]
vars = { NAMESPACE = "default" }
steps = [
  { name = "Build image", run = "docker build -t $REGISTRY/$NAME:$VERSION ." },
  { name = "Push image", run = "docker push $REGISTRY/$NAME:$VERSION" },
  { name = "Helm upgrade", run = """
helm upgrade --install $RELEASE ./chart \
  --namespace $NAMESPACE \
  --set image.repository=$REGISTRY/$NAME \
  --set image.tag=$VERSION""" },
]
```

## Multi-Step Deployments

Targets can have multiple steps that run in sequence:

```toml
[targets.full-deploy]
steps = [
  { name = "Run tests", run = "go test ./..." },
  { name = "Build binary", run = "go build -o app ." },
  { name = "Build Docker image", run = "docker build -t $REGISTRY/$NAME:$VERSION ." },
  { name = "Push to registry", run = "docker push $REGISTRY/$NAME:$VERSION" },
  { name = "Deploy to staging", run = "kubectl apply -f k8s/staging/" },
  { name = "Run smoke tests", run = "./scripts/smoke-test.sh staging" },
  { name = "Deploy to production", run = "kubectl apply -f k8s/production/" },
]
```

!!! warning "Step Failure"
    If any step fails, the deployment stops immediately. The lock file is NOT updated.

## Conditional Commands

Use shell conditionals for more complex logic:

```toml
[targets.conditional-deploy]
steps = [
  { name = "Deploy", run = """
if [ "$ENVIRONMENT" = "production" ]; then
  kubectl apply -f k8s/production/
else
  kubectl apply -f k8s/staging/
fi
""" },
]
```

## See Also

- [Presets](presets.md) — Pre-built target configurations
- [Artifacts](artifacts.md) — How to use targets in artifacts
- [bear apply](../commands/apply.md) — Executing deployments
