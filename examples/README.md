# Example Project

The Bear project root is **`examples/`**, inside this Git repository. Jenkinsfiles
use `dir('examples')`; locally run commands from that directory:

```bash
cd examples
bear doctor
bear validate
bear plan dev
```

These are demonstration services, not production-ready applications. The lock
file's placeholder commits illustrate legacy history, not real deployable source.
Use a disposable checkout and replan for each intended environment. Legacy entries
are retained but ignored, not automatically mapped to environment history.
Never treat placeholder or legacy history as evidence of a deployment.

Normal plans with deployments require the whole Git repository to be clean before
planning, except Bear state. Commit intended configuration/source edits first.
Plan itself never modifies tracked source or HEAD — it runs no commands at all.
`bear apply` verifies the saved commit and fingerprint match before it builds or
deploys anything, and since apply always builds fresh immediately before it
deploys, a receiving checkout does not need to already contain build output from
wherever the plan was created — only the same tracked commit and nonignored
untracked files. Ignored dependencies and outputs are excluded from the
fingerprint regardless, so provision them the same way you would for any fresh
build.

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

## Environments

`examples/bear.config.yml` declares `environments: [dev, int, prd]`. That list is
this project's choice, not a Bear built-in: rename it, cut it to one entry, or use
`[preprd, prd]` instead, and every command follows. All six demonstration artifacts
allow exactly those three environments, and an allowlist entry that the config does
not declare fails `bear doctor`, `bear validate` and `bear plan`. Remove the
environments you do not intend to deploy from both places. Missing or empty
allowlists deny deployment while retaining change detection.

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
`examples/Jenkinsfile.docker`; see [Jenkins Freeze/Unfreeze
Template](#jenkins-freezeunfreeze-template) below for the separate,
parameter-driven `examples/Jenkinsfile.freeze`. For the host-agent template,
provide `BEAR_VERSION` as a reviewed release tag such as `v5.0.1`,
and put `$(go env GOPATH)/bin` on the agent PATH. Its `Install` stage installs from
a tagged checkout outside the workspace because the Go module has no `/v4` suffix.
The Docker template expects the
custom image above, extended with deployment tools.

Both templates run a `Validate` stage guarded by `changeRequest()`: a change
request runs `bear validate` from `examples/` and nothing else. That stage takes
no credentials, needs no environment argument and no Git history, writes no plan
and no lock file, and never runs apply. The `Plan` stage carries the complementary
`not { changeRequest() }` guard, so a change request cannot produce a production
plan it is not allowed to apply.

Both templates run Bear from `examples/`. Apply runs only on trusted `main`,
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

## Jenkins Freeze/Unfreeze Template

`examples/Jenkinsfile.freeze` is a separate, manually-triggered ("Build with
Parameters") pipeline for freezing one or all deployables to a fixed commit, or
explicitly unfreezing one. It exposes `ENVIRONMENT`, `DEPLOYABLE`, `REF`, and
`FORCE` as build parameters around `bear plan`'s existing `--pin` and `--force`
flags — there is no dedicated Bear "freeze" command. Use it instead of, not in
addition to, `examples/Jenkinsfile`/`examples/Jenkinsfile.docker` for a given
deployment job:

- Reach for the plain commit-triggered templates when every push to `main`
  should simply deploy whatever changed.
- Reach for `Jenkinsfile.freeze` when an operator needs to pin a specific
  deployable to a known-good commit ahead of a release window, or unfreeze it
  again on demand, without waiting for a new commit.

It reuses the same identity variables, `gitUsernamePassword` credential binding,
trusted-`main`/`changeRequest()` guards, `--git-remote`/`--git-branch` flags,
install-from-tag block, and `disableConcurrentBuilds()` as the other two
templates. A scripted guard rejects `FORCE=true` with an empty `DEPLOYABLE`
before Bear ever runs, because `--force` without an artifact filter would
otherwise unfreeze every artifact in the selection. See
[Freeze & Unfreeze (Jenkins)](https://github.com/irevolve/bear/blob/main/docs/concepts/freeze-unfreeze.md)
for the full explanation, including why `REF` must be fetched first (Bear does
not fetch remote refs itself) and why an unknown `DEPLOYABLE` name fails the
build instead of silently doing nothing.
