# CI/CD Integration

Bear works with any CI system. Select your platform — all examples stay in sync.

## Basic Pipeline

=== "GitHub Actions"

    ```yaml title=".github/workflows/deploy.yml"
    name: Deploy
    on:
      push:
        branches: [main]
        paths-ignore:
          - 'bear.lock.yml'

    jobs:
      deploy:
        runs-on: ubuntu-latest
        steps:
          - uses: actions/checkout@v4
            with:
              fetch-depth: 0

          - uses: actions/setup-go@v5
            with:
              go-version: '1.21'

          - run: go install github.com/irevolve/bear@latest

          - name: Configure Git
            run: |
              git config user.name "github-actions[bot]"
              git config user.email "github-actions[bot]@users.noreply.github.com"

          - run: bear plan
          - run: bear apply
            env:
              DOCKER_USERNAME: ${{ secrets.DOCKER_USERNAME }}
              DOCKER_PASSWORD: ${{ secrets.DOCKER_PASSWORD }}
    ```

=== "GitLab CI"

    ```yaml title=".gitlab-ci.yml"
    stages:
      - deploy

    deploy:
      stage: deploy
      image: golang:1.21
      rules:
        - if: $CI_COMMIT_BRANCH == "main"
          changes:
            - "**/*"
      before_script:
        - go install github.com/irevolve/bear@latest
        - git config user.name "GitLab CI"
        - git config user.email "ci@gitlab.com"
      script:
        - bear plan
        - bear apply
      variables:
        DOCKER_USERNAME: $DOCKER_USERNAME
        DOCKER_PASSWORD: $DOCKER_PASSWORD
    ```

=== "Jenkins"

    ```groovy title="Jenkinsfile"
    pipeline {
        agent any
        
        environment {
            DOCKER_USERNAME = credentials('docker-username')
            DOCKER_PASSWORD = credentials('docker-password')
        }
        
        stages {
            stage('Setup') {
                steps {
                    checkout scm
                    sh 'go install github.com/irevolve/bear@latest'
                    sh '''
                        git config user.name "Jenkins"
                        git config user.email "jenkins@ci"
                    '''
                }
            }
            stage('Plan') {
                steps {
                    sh 'bear plan'
                }
            }
            stage('Apply') {
                steps {
                    sh 'bear apply'
                }
            }
        }
    }
    ```

## With Docker Image

Bear provides Docker images so you don't need Go installed:

| Image | Size | Use Case |
|-------|------|----------|
| `ghcr.io/irevolve/bear:latest` | ~5MB | Minimal, just Bear binary |
| `ghcr.io/irevolve/bear:alpine` | ~15MB | With Git (for auto-commit) |
| `ghcr.io/irevolve/bear:debian` | ~50MB | Full environment |

=== "GitHub Actions"

    ```yaml title=".github/workflows/deploy.yml"
    name: Deploy
    on:
      push:
        branches: [main]
        paths-ignore:
          - 'bear.lock.yml'

    jobs:
      deploy:
        runs-on: ubuntu-latest
        container:
          image: ghcr.io/irevolve/bear:alpine
        steps:
          - uses: actions/checkout@v4
            with:
              fetch-depth: 0

          - name: Configure Git
            run: |
              git config --global user.name "github-actions[bot]"
              git config --global user.email "github-actions[bot]@users.noreply.github.com"
              git config --global --add safe.directory $GITHUB_WORKSPACE

          - run: bear plan
          - run: bear apply
    ```

=== "GitLab CI"

    ```yaml title=".gitlab-ci.yml"
    deploy:
      image: ghcr.io/irevolve/bear:alpine
      stage: deploy
      before_script:
        - git config --global user.name "GitLab CI"
        - git config --global user.email "ci@gitlab.com"
      script:
        - bear plan
        - bear apply
    ```

=== "Jenkins"

    ```groovy title="Jenkinsfile"
    pipeline {
        agent {
            docker {
                image 'ghcr.io/irevolve/bear:alpine'
            }
        }
        stages {
            stage('Plan') {
                steps {
                    sh '''
                        git config --global user.name "Jenkins"
                        git config --global user.email "jenkins@ci"
                        bear plan
                    '''
                }
            }
            stage('Apply') {
                steps {
                    sh 'bear apply'
                }
            }
        }
    }
    ```

## Preview in Pull Requests

Run `bear plan` on PRs to preview what would be deployed:

=== "GitHub Actions"

    ```yaml title=".github/workflows/pr-preview.yml"
    name: Preview
    on:
      pull_request:
        branches: [main]

    jobs:
      plan:
        runs-on: ubuntu-latest
        steps:
          - uses: actions/checkout@v4
            with:
              fetch-depth: 0
          - run: go install github.com/irevolve/bear@latest
          - run: bear plan
    ```

=== "GitLab CI"

    ```yaml title=".gitlab-ci.yml"
    preview:
      stage: test
      image: ghcr.io/irevolve/bear:alpine
      rules:
        - if: $CI_PIPELINE_SOURCE == "merge_request_event"
      script:
        - bear plan
    ```

=== "Jenkins"

    ```groovy title="Jenkinsfile"
    pipeline {
        agent any
        stages {
            stage('Preview') {
                when {
                    changeRequest()
                }
                steps {
                    sh 'go install github.com/irevolve/bear@latest'
                    sh 'bear plan'
                }
            }
        }
    }
    ```

## Manual Approval for Production

Add a manual gate before deploying:

=== "GitHub Actions"

    ```yaml title=".github/workflows/deploy.yml"
    jobs:
      plan:
        runs-on: ubuntu-latest
        steps:
          - uses: actions/checkout@v4
            with:
              fetch-depth: 0
          - run: go install github.com/irevolve/bear@latest
          - run: bear plan
          - uses: actions/upload-artifact@v4
            with:
              name: plan
              path: .bear/plan.yml

      deploy:
        needs: plan
        runs-on: ubuntu-latest
        environment: production    # Requires approval
        steps:
          - uses: actions/checkout@v4
            with:
              fetch-depth: 0
          - run: go install github.com/irevolve/bear@latest
          - uses: actions/download-artifact@v4
            with:
              name: plan
              path: .bear
          - run: bear apply
    ```

=== "GitLab CI"

    ```yaml title=".gitlab-ci.yml"
    plan:
      stage: plan
      image: ghcr.io/irevolve/bear:alpine
      script:
        - bear plan
      artifacts:
        paths:
          - .bear/plan.yml

    deploy:
      stage: deploy
      image: ghcr.io/irevolve/bear:alpine
      needs: [plan]
      when: manual                 # Requires click
      script:
        - bear apply
    ```

=== "Jenkins"

    ```groovy title="Jenkinsfile"
    pipeline {
        agent any
        stages {
            stage('Plan') {
                steps {
                    sh 'bear plan'
                }
            }
            stage('Approve') {
                steps {
                    input message: 'Deploy to production?'
                }
            }
            stage('Apply') {
                steps {
                    sh 'bear apply'
                }
            }
        }
    }
    ```

## Lock File & Skip CI

After `bear apply`, the lock file (`bear.lock.yml`) is updated and committed with `[skip ci]`. This prevents infinite CI loops.

If your CI doesn't support `[skip ci]`, use path filters:

=== "GitHub Actions"

    ```yaml
    on:
      push:
        paths-ignore:
          - 'bear.lock.yml'
    ```

=== "GitLab CI"

    ```yaml
    workflow:
      rules:
        - changes:
            - bear.lock.yml
          when: never
        - when: always
    ```

=== "Jenkins"

    ```groovy
    pipeline {
        triggers {
            pollSCM('H/5 * * * *')
        }
        stages {
            stage('Check') {
                when {
                    not {
                        changeset 'bear.lock.yml'
                    }
                }
                steps {
                    sh 'bear plan && bear apply'
                }
            }
        }
    }
    ```

Or disable auto-commit entirely:

```bash
bear apply --no-commit
```

## Environment Variables

Pass deployment credentials via your CI's secret management:

=== "GitHub Actions"

    ```yaml
    env:
      DOCKER_USERNAME: ${{ secrets.DOCKER_USERNAME }}
      DOCKER_PASSWORD: ${{ secrets.DOCKER_PASSWORD }}
      PROJECT: ${{ secrets.GCP_PROJECT }}
      AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
      AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
    ```

=== "GitLab CI"

    ```yaml
    variables:
      DOCKER_USERNAME: $DOCKER_USERNAME
      DOCKER_PASSWORD: $DOCKER_PASSWORD
      PROJECT: $GCP_PROJECT
      AWS_ACCESS_KEY_ID: $AWS_ACCESS_KEY_ID
      AWS_SECRET_ACCESS_KEY: $AWS_SECRET_ACCESS_KEY
    ```

=== "Jenkins"

    ```groovy
    environment {
        DOCKER_USERNAME = credentials('docker-username')
        DOCKER_PASSWORD = credentials('docker-password')
        PROJECT = credentials('gcp-project')
        AWS_ACCESS_KEY_ID = credentials('aws-access-key')
        AWS_SECRET_ACCESS_KEY = credentials('aws-secret-key')
    }
    ```

## Concurrency

Bear runs validations and deployments in parallel (default: 10). Tune for your CI:

```bash
bear plan --concurrency 5     # Limit parallel validations
bear apply --concurrency 3    # Limit parallel deployments
```
