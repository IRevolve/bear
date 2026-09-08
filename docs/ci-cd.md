# CI/CD Integration

## Environment Policy

Every plan requires `dev`, `int`, or `prd` as its first positional argument,
including validation-only plans and pinned or unchanged selections:

```bash
bear plan int
bear apply
```

Artifact filters follow the environment: `bear plan dev user-api`.
Artifacts must opt in with `environments: [dev, int]` in `bear.artifact.yml`.
Absent or empty allowlists deny deployment everywhere, but retain validation and
dependency propagation. Each dependent uses its own allowlist. Neither `--pin`
nor `--force` bypasses policy. Use `[dev, int, prd]` to permit all three.

Inherited or configured `ENVIRONMENT` cannot select policy. The positional value
is injected as `$ENVIRONMENT` into validation and deployment steps, overriding
inherited/configured values. Credentials and targets remain separately configured.
The selected environment, allowlists, and decisions are saved in the plan; apply
does not re-read policy. Replan and reapprove after changing policy or environment.
Disabled deployments do not update history.

After CLI argument parsing succeeds and the workspace lock is acquired, planning invalidates the previous saved plan
before validation. A missing argument (`bear plan`) is a syntax error and leaves
the old plan untouched; a supplied invalid environment (`bear plan qa`) starts
planning and clears it. Run apply only after a successful plan.

History and pins are scoped per environment and artifact. Legacy artifact-only
history is retained but ignored, not automatically mapped to environments; replan
and review each environment. See [Source Safety](concepts/plan-apply.md#source-safety)
and [History And State](concepts/plan-apply.md#history-and-state).

## Agent Requirements

Bear is an orchestrator, not a toolchain bundle. Install the tools used by your
configured steps before planning. This repository's sample project root is
`examples/`, not the repository root. See [the example setup](https://github.com/irevolve/bear/blob/main/examples/README.md)
and the two checked-in Jenkinsfiles for full templates.

The Go `examples/functions/github-webhook` is an HTTP service for Cloud Run on
port `8080`, not a Lambda executable. The Python `stripe-webhook` stays on Lambda:
its ZIP has `handler.py` at the root and requires an existing Python function with
handler `handler.main`. The sample target updates code, not runtime or handler
configuration. See the example setup for credentials, packaging, and security limits.

| Image aliases | Contents | Limitations |
|---------------|----------|-------------|
| `latest` | Bear and CA certificates (scratch) | No shell or Git; not a CI job image |
| `latest-alpine`, `alpine` | Bear, Git, CA certificates, Alpine shell | No language or deployment toolchains; musl can also limit CI action compatibility |
| `latest-debian`, `debian` | Bear, Git, CA certificates, Debian shell | No language or deployment toolchains |

Stable version tags are `1.2.3`, `1.2.3-alpine`, and `1.2.3-debian`, with matching
`1.2` aliases. Prerelease tags are deliberately namespaced, e.g.
`scratch-1.2.3-rc.1`, `alpine-1.2.3-rc.1-alpine`, and
`debian-1.2.3-rc.1-debian`. This prevents a prerelease named `alpine` or `debian`
from colliding with a stable variant tag. Prereleases never update stable aliases.
Release refs must be strict SemVer with a `v` prefix: `v1.2.3-beta.1` is valid;
`v1.2.3a` is rejected. Build metadata uses `_` in Docker tags instead of `+`.
Pin a reviewed version/digest for reproducibility rather than relying on `latest`.

All images have a Bear entrypoint. Jenkins and GitLab need an explicit override
to execute their own shell scripts. Build `examples/Dockerfile.ci` to add the
sample validation tools, then add deployment CLIs for the targets you actually use.
Do not mount a host Docker socket into an untrusted PR job.

## Trusted Deployments

The templates use `--git-remote` and `--git-branch` to select the lock publication
destination explicitly. Bear reports Git failures as nonzero errors and retains
the plan and deployment history for recovery.
Only a protected, trusted `main` job receives write credentials or runs apply.
PR jobs can execute arbitrary repository commands and must not receive deployment
secrets. Configure protected branches/environments and CI trust policy separately.

### GitHub Actions

This template assumes an agent with Bear and the project's toolchains installed.
For this repository use `working-directory: examples` as shown; for another project
use the directory containing its `bear.config.yml`.

```yaml title=".github/workflows/deploy.yml"
name: Deploy
on:
  push:
    branches: [main]
permissions:
  contents: write
concurrency:
  group: bear-production
  cancel-in-progress: false
jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: production
    env:
      GIT_AUTHOR_NAME: Bear CI
      GIT_AUTHOR_EMAIL: bear-ci@example.com
      GIT_COMMITTER_NAME: Bear CI
      GIT_COMMITTER_EMAIL: bear-ci@example.com
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      # Provision a reviewed Bear release and required toolchains here.
      - run: bear plan prd
        working-directory: examples
      - run: bear apply --git-remote origin --git-branch main
        working-directory: examples
```

GitHub's checkout token is scoped to the job and cleaned up by the action.
Use least-privilege credentials and do not rewrite `origin` with a token URL.

### GitLab CI

Build the validation image and extend it with your deployment tools first. Set
`origin` to your repository's **token-free SSH URL** in the template. Configure a
protected file variable `DEPLOY_SSH_KEY` and a protected `SSH_KNOWN_HOSTS` file
whose host keys were verified out of band; install `openssh-client` in that image.
Do not obtain trusted host keys with an unchecked runtime `ssh-keyscan`.

```yaml title=".gitlab-ci.yml"
stages: [validate, deploy]
default:
  image:
    name: your-registry/bear-ci:reviewed
    entrypoint: [""]
variables:
  GIT_DEPTH: "0"
  GIT_AUTHOR_NAME: Bear CI
  GIT_AUTHOR_EMAIL: bear-ci@example.com
  GIT_COMMITTER_NAME: Bear CI
  GIT_COMMITTER_EMAIL: bear-ci@example.com
preview:
  stage: validate
  rules:
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
  script:
    - cd examples
    - bear plan dev
deploy:
  stage: deploy
  resource_group: bear-production
  environment: production
  rules:
    - if: '$CI_PIPELINE_SOURCE == "push" && $CI_COMMIT_BRANCH == "main" && $CI_COMMIT_REF_PROTECTED == "true"'
  before_script:
    - chmod 600 "$DEPLOY_SSH_KEY"
    - export GIT_SSH_COMMAND="ssh -i '$DEPLOY_SSH_KEY' -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile='$SSH_KNOWN_HOSTS'"
    - git remote set-url origin git@gitlab.com:your-org/your-repo.git
  script:
    - test "$CI_COMMIT_BRANCH" = main
    - git check-ref-format --branch "$CI_COMMIT_BRANCH"
    - cd examples
    - bear plan prd
    - bear apply --git-remote origin --git-branch "$CI_COMMIT_BRANCH"
```

### Jenkins

Use `examples/Jenkinsfile` or `examples/Jenkinsfile.docker` as the multibranch
pipeline script. The Docker template overrides the Bear entrypoint:

```groovy
agent {
    docker {
        image 'your-registry/bear-ci:reviewed'
        args '--entrypoint=""'
    }
}
```

Both templates set `GIT_AUTHOR_NAME`, `GIT_AUTHOR_EMAIL`, `GIT_COMMITTER_NAME`,
and `GIT_COMMITTER_EMAIL`. Jenkins checkout often supplies credentials without a
commit identity, causing "unable to auto-detect email address". Detached HEAD is
also normal: plain `git push` has no current branch. The trusted deployment stage
uses the Git plugin's `gitUsernamePassword` binding and invokes:

```bash
test -z "${CHANGE_ID:-}"
test "$BRANCH_NAME" = main
git check-ref-format --branch "$BRANCH_NAME"
bear apply --git-remote origin --git-branch "$BRANCH_NAME"
```

The exact `main` allowlist and Git's ref validation reject unsafe branch values;
do not transform an arbitrary PR branch into a deployment branch. Configure the
named Git tool (`Default` in these templates) in Jenkins. For SSH repositories,
use `sshagent` with a verified known-hosts setup instead. Never put passwords or
tokens in `git remote set-url`, interpolate them into Groovy strings, or persist
them in Git config. These templates do not change Git identity configuration.

## Approval And State

Plan for the intended environment, review, then approve apply in a protected job.
Normal plans with any deployments require clean source before validation. Validation
must not change HEAD or tracked source; generated nonignored untracked files are
saved in the post-validation fingerprint. If separate jobs transfer a normal plan,
check out the same revision and reproduce or transfer those generated files:
normal apply does not rerun validation. Paths in the plan are project-relative,
not tied to the original absolute checkout directory. Ignored dependencies and
outputs are excluded from the fingerprint, so provision them reproducibly yourself.
Pinned apply uses a private worktree and reruns saved validation/setup steps before
checking the fingerprint. See [Source Safety](concepts/plan-apply.md#source-safety).

Preserve required recovery state too. Plans can contain resolved secrets: use private,
short-lived artifacts, restrictive access, and restore mode `0600` after download
(artifact services may discard Unix permissions). Do not print or publicly upload
the plan. Never apply a PR-generated plan with production credentials.

Bear takes both a project-local workspace lock and a repository lock in the common
Git directory. The repository lock serializes cooperating Bear operations across
sibling/nested projects and linked worktrees sharing that directory. Independent
clones/runners do not share it: coordinate CI jobs publishing to the same remote
branch or deployment state. Git itself and other noncooperating writers do not
honor Bear's advisory locks. Do not delete `.bear/workspace.lock` or the common
Git directory's `bear.repository.lock`; file existence does not mean a stale lock.
Each deployment completion saves lock history and then checkpoints the plan. On
retry, completed artifacts are skipped only when lock history matches. Preserve
both files; a crash or failed checkpoint can still leave external status uncertain.

Git failures fail the job and retain completed checkpoints. If a push fails after
the local commit, inspect HEAD and the remote and retry the push **manually** as
the helper instructs. Rerunning apply alone cannot publish that local-ahead commit.
After recovery, matching checkpoints prevent redeployment. See
[Git Publication](commands/apply.md#git-publication). There is no transaction that
can roll back an external deployment on a state or Git failure.

For ephemeral runners, configure private recovery storage for the updated lock,
checkpointed plan, and any unpublished Git commit/objects before enabling real
deployments; a commit ID alone cannot recover an object deleted with the runner.
Do not put secret-bearing plans into public/general-purpose artifacts. The Jenkins
templates retain failed workspaces; restrict access and clean up after recovery.

## Skip CI Safely

Bear's lock update subject is `chore(bear): update lock file [skip ci]`. CI providers
may honor `[skip ci]` before evaluating any job. Do not add that marker to mixed
source commits, and configure provider/organization policy if such skips must be
prohibited. A workflow cannot override a provider-level skip after it occurs.

For custom skipping, `examples/ci/skip-lock-only.sh` is conservative: it verifies
one non-merge commit since the previous build, both bot identities, the exact
subject, and **only** `examples/bear.lock.yml` changed. Missing history runs CI.
Change the lock path if your project root differs. Bot email matching is not an
authentication boundary; protect the branch and credentials too.

Do not use GitLab `changes: [bear.lock.yml]` with `when: never` or Jenkins
`not { changeset 'bear.lock.yml' }`: both can suppress mixed source/lock changes.
Path-only filters also cannot establish bot identity. Running extra CI is safer
than skipping an uncertain commit. Use `bear apply --no-commit` if CI should own
state persistence explicitly, then retain the updated lock file durably.

## Live Output

Non-terminal progress uses newline-delimited updates with step and elapsed time.
Run Bear directly in Jenkins `sh`, without `returnStdout: true` or shell capture.
No TTY or ANSI plugin is required. `--verbose` streams subprocess output while
retaining bounded failure diagnostics. Without it, output is captured and failure
tails are reported. Treat logs as sensitive; bounded capture does not redact secrets.

```bash
bear plan dev --concurrency 5
bear apply --concurrency 3
bear plan dev --verbose
```
