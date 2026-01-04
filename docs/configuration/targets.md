# Targets

Targets define where and how your artifacts get deployed. Each target specifies deployment steps and default parameters.

## Using Preset Targets

The easiest way is to use preset targets:

```yaml title="bear.config.yml"
name: my-project

use:
  targets: [docker, cloudrun, kubernetes]
```

See [Presets](presets.md) for available preset targets.

## Defining Custom Targets

Define targets in your `bear.config.yml`:

```yaml title="bear.config.yml"
name: my-project

targets:
  - name: my-server
    defaults:
      HOST: deploy.example.com
      USER: deploy
    deploy:
      - name: Build
        run: go build -o app .
      - name: Upload
        run: scp app $USER@$HOST:/opt/app/
      - name: Restart
        run: ssh $USER@$HOST "systemctl restart app"
```

## Target Structure

```yaml
targets:
  - name: target-name           # Unique identifier
    defaults:                   # Default parameters (optional)
      PARAM1: value1
      PARAM2: value2
    deploy:                     # Deployment steps
      - name: Step Name
        run: command $PARAM1
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | ✓ | Unique target identifier |
| `defaults` | | Default parameter values |
| `deploy` | ✓ | List of deployment steps |

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
| Target defaults | From `defaults:` section | `$REGION`, `$BUCKET` |
| Artifact env | From artifact's `env:` section | `$PROJECT` |
| Artifact params | From artifact's `params:` section | `$MEMORY` |

### Variable Precedence

1. Artifact `env:` (highest priority)
2. Artifact `params:`
3. Target `defaults:` (lowest priority)

## Target Examples

### Docker Registry

```yaml
- name: docker
  defaults:
    REGISTRY: docker.io
  deploy:
    - name: Build image
      run: docker build -t $REGISTRY/$NAME:$VERSION .
    - name: Push image
      run: docker push $REGISTRY/$NAME:$VERSION
```

**Usage in artifact:**

```yaml title="services/api/bear.artifact.yml"
name: api
target: docker
env:
  REGISTRY: ghcr.io/myorg
```

### Google Cloud Run

```yaml
- name: cloudrun
  defaults:
    REGION: europe-west1
    MEMORY: 512Mi
    CPU: "1"
  deploy:
    - name: Build image
      run: docker build -t gcr.io/$PROJECT/$NAME:$VERSION .
    - name: Push to GCR
      run: docker push gcr.io/$PROJECT/$NAME:$VERSION
    - name: Deploy to Cloud Run
      run: |
        gcloud run deploy $NAME \
          --image gcr.io/$PROJECT/$NAME:$VERSION \
          --region $REGION \
          --memory $MEMORY \
          --cpu $CPU
```

**Usage in artifact:**

```yaml title="services/api/bear.artifact.yml"
name: api
target: cloudrun
env:
  PROJECT: my-gcp-project
params:
  MEMORY: 1Gi
  CPU: "2"
```

### Kubernetes

```yaml
- name: kubernetes
  defaults:
    NAMESPACE: default
  deploy:
    - name: Build image
      run: docker build -t $REGISTRY/$NAME:$VERSION .
    - name: Push image
      run: docker push $REGISTRY/$NAME:$VERSION
    - name: Update deployment
      run: |
        kubectl set image deployment/$NAME \
          $NAME=$REGISTRY/$NAME:$VERSION \
          -n $NAMESPACE
    - name: Wait for rollout
      run: kubectl rollout status deployment/$NAME -n $NAMESPACE
```

### AWS Lambda

```yaml
- name: lambda
  defaults:
    REGION: us-east-1
    RUNTIME: provided.al2
  deploy:
    - name: Build
      run: GOOS=linux GOARCH=amd64 go build -o bootstrap .
    - name: Package
      run: zip function.zip bootstrap
    - name: Deploy
      run: |
        aws lambda update-function-code \
          --function-name $FUNCTION_NAME \
          --zip-file fileb://function.zip \
          --region $REGION
```

### S3 Static Site

```yaml
- name: s3-static
  defaults:
    CACHE_CONTROL: "max-age=31536000"
  deploy:
    - name: Build
      run: npm run build
    - name: Sync to S3
      run: aws s3 sync dist/ s3://$BUCKET/ --delete
    - name: Invalidate CloudFront
      run: |
        aws cloudfront create-invalidation \
          --distribution-id $DISTRIBUTION_ID \
          --paths "/*"
```

### SSH/SCP Deploy

```yaml
- name: ssh-deploy
  defaults:
    REMOTE_PATH: /opt/app
  deploy:
    - name: Build
      run: go build -o app .
    - name: Upload binary
      run: scp app $SSH_USER@$SSH_HOST:$REMOTE_PATH/app-$VERSION
    - name: Switch symlink
      run: ssh $SSH_USER@$SSH_HOST "ln -sfn $REMOTE_PATH/app-$VERSION $REMOTE_PATH/app"
    - name: Restart service
      run: ssh $SSH_USER@$SSH_HOST "sudo systemctl restart myapp"
```

### Helm Chart

```yaml
- name: helm
  defaults:
    NAMESPACE: default
  deploy:
    - name: Build image
      run: docker build -t $REGISTRY/$NAME:$VERSION .
    - name: Push image
      run: docker push $REGISTRY/$NAME:$VERSION
    - name: Helm upgrade
      run: |
        helm upgrade --install $RELEASE ./chart \
          --namespace $NAMESPACE \
          --set image.repository=$REGISTRY/$NAME \
          --set image.tag=$VERSION
```

### Fly.io

```yaml
- name: fly
  deploy:
    - name: Deploy to Fly
      run: fly deploy --app $APP --image $REGISTRY/$NAME:$VERSION
```

### Vercel

```yaml
- name: vercel
  deploy:
    - name: Build
      run: npm run build
    - name: Deploy to Vercel
      run: vercel deploy --prod --yes
```

### Netlify

```yaml
- name: netlify
  deploy:
    - name: Build
      run: npm run build
    - name: Deploy to Netlify
      run: netlify deploy --prod --dir=dist --site=$SITE_ID
```

## Multi-Step Deployments

Targets can have multiple steps that run in sequence:

```yaml
- name: full-deploy
  deploy:
    - name: Run tests
      run: go test ./...
    - name: Build binary
      run: go build -o app .
    - name: Build Docker image
      run: docker build -t $REGISTRY/$NAME:$VERSION .
    - name: Push to registry
      run: docker push $REGISTRY/$NAME:$VERSION
    - name: Deploy to staging
      run: kubectl apply -f k8s/staging/
    - name: Run smoke tests
      run: ./scripts/smoke-test.sh staging
    - name: Deploy to production
      run: kubectl apply -f k8s/production/
```

!!! warning "Step Failure"
    If any step fails, the deployment stops immediately. The lock file is NOT updated.

## Conditional Commands

Use shell conditionals for more complex logic:

```yaml
- name: conditional-deploy
  deploy:
    - name: Deploy
      run: |
        if [ "$ENVIRONMENT" = "production" ]; then
          kubectl apply -f k8s/production/
        else
          kubectl apply -f k8s/staging/
        fi
```

## See Also

- [Presets](presets.md) — Pre-built target configurations
- [Artifacts](artifacts.md) — How to use targets in artifacts
- [bear apply](../commands/apply.md) — Executing deployments
