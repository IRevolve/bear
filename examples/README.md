# Example Project

The Bear project root is **`examples/`**, inside this Git repository. Jenkinsfiles
use `dir('examples')`; locally run commands from that directory:

```bash
cd examples
bear check
bear plan dev
```

These are demonstration services, not production-ready applications. The lock
file's placeholder commits illustrate legacy history, not real deployable source.
Use a disposable checkout and replan for each intended environment. Legacy entries
are retained but ignored, not automatically mapped to environment history.
Never treat placeholder or legacy history as evidence of a deployment.

Normal plans with deployments require the whole Git repository to be clean before
validation, except Bear state. Commit intended configuration/source edits first.
Validation cannot change tracked source or HEAD. Nonignored generated files enter
the saved fingerprint; normal apply needs those same files and does not rerun
validation. Ignored dependencies and outputs are excluded, so their immutability
and reproducible provisioning are your responsibility.

## Toolchains

Validation needs Go 1.25+, a C compiler for `go test -race`, Node 18+/npm, and
Python 3.11+. The Node dashboard uses npm's `file:../../libs/ui` dependency, not
the unsupported npm `workspace:*` protocol. Its Vite entrypoint is included;
there are no Node lint/test scripts until actual lint configuration/tests exist.
Python uses the standard-library unittest runner and a real test; failures are
not swallowed or reported as missing pytest.

Build the validation agent from the repository root:

```bash
docker build -f examples/Dockerfile.ci --build-arg BEAR_IMAGE=ghcr.io/irevolve/bear:debian -t your-registry/bear-ci:reviewed .
```

Replace the moving Bear image with a reviewed version or digest in real CI.
The official Alpine/Debian images alone do **not** provide validation toolchains.
The scratch image has neither shell nor Git. Jenkins must pass
`args '--entrypoint=""'`; GitLab must use `entrypoint: [""]`.

Apply also needs target-specific tools and credentials: Docker plus a daemon and
gcloud for Cloud Run, AWS CLI and zip for Lambda, AWS CLI for S3, or Podman for
`docker-local`. The validation image does not supply these. Provision only what
you need; do not expose deployment secrets or a host Docker socket to PRs.
All demonstration artifacts explicitly allow `dev`, `int`, and `prd`. Remove
environments you do not intend to deploy. Missing/empty allowlists deny deployment.

## Webhook Targets

`functions/github-webhook` is a Go HTTP service deployed to **Cloud Run**, despite
the `functions/` directory name. It serves `/webhook` on hardcoded port `8080`;
the Cloud Run target explicitly selects that port. Its multi-stage Dockerfile
builds a static server, like the API examples. It is not a Lambda runtime and
does not provide a `bootstrap` executable. Set `PROJECT`, authenticate Docker and
gcloud, and configure service access deliberately. CI builds and smoke-tests the
container with a JSON request without publishing it.

`functions/stripe-webhook` remains a **Python Lambda** example. Bear runs packaging
from that artifact directory: `zip -r dist.zip .` puts `handler.py` at the archive
root, and `fileb://dist.zip` uploads that same local archive. Provision an existing
Lambda named `stripe-webhook` with a supported Python 3.11+ runtime and handler
`handler.main`. The target only updates code; the artifact's `HANDLER` variable
documents the required handler but does not configure Lambda. The example uses
only standard-library imports; package any dependencies you add yourself.

Both handlers are demonstrations, not production webhook security implementations:
the Go handler does not verify GitHub signatures, and the Python handler uses a
placeholder secret and a simplified signature format, not Stripe's timestamped
signature protocol. Add proper verification and secret management before exposing
either to real webhook traffic.

## Jenkins Setup

Configure the multibranch script path as `examples/Jenkinsfile` or
`examples/Jenkinsfile.docker`. For the host-agent template, provide `BEAR_VERSION`
as a reviewed release tag such as `v4.0.0`,
and put `$(go env GOPATH)/bin` on the agent PATH. The template installs from a tagged
checkout outside the workspace because the Go module has no `/v4` suffix.
The Docker template expects the
custom image above, extended with deployment tools.

Both templates validate from `examples/`. Apply runs only on trusted `main`,
rejects change requests, validates the branch, sets author/committer identity via
environment variables, and uses `gitUsernamePassword` for temporary authentication.
Configure the Jenkins Git tool named `Default` and credential `git-credentials`;
keep `origin` token-free. Protect main and configure fork/PR trust in Jenkins.
For SSH remotes use `sshagent` and verified known hosts instead.

The apply commands use `--git-remote origin --git-branch main` (via the validated
`BRANCH_NAME`). A detached checkout is expected; plain `git push` is not sufficient.
The target branch must already exist and its remote tip must equal local HEAD
before Bear creates a lock commit. Git failure returns nonzero and retains the
plan and history. Matching completed checkpoints prevent redeployment on retry.
If the push fails after a local commit, inspect HEAD and the remote and retry the
push manually as the error directs; rerunning apply alone refuses local-ahead HEAD.
Persistence failures can leave external status uncertain, so reconcile real
deployment status before retrying or replanning.
Failed workspaces are deliberately retained for recovery. Restrict their access
because saved plans may contain secrets, and clean them after investigating.

The skip helper skips only a single bot lock-only update since the prior build.
Missing history, non-bot identity, merge commits, and mixed file changes run CI.
Do not add `[skip ci]` to source commits: provider-level skip handling can bypass
the pipeline before this helper gets a chance to run.
