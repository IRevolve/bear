# CI/CD Integration

## Environment Policy

Every plan requires an environment declared in `bear.config.yml` as its first
positional argument, including a plan that ends up with nothing to deploy and
pinned or unchanged selections:

```bash
bear plan int
bear apply
```

The project declares its own environments; Bear has no built-in set. `dev`, `int`,
`prd` is only `bear init`'s default, so a pipeline for a project declaring
`[preprd, prd]` runs `bear plan preprd` and `bear plan prd` instead. Run
`bear doctor` in CI to print the declared list, and remember that declaration order
carries **no** promotion semantics — if your pipeline must reach `int` before `prd`,
express that in the pipeline, because Bear will not.

Artifact filters follow the environment: `bear plan dev user-api`.
Artifacts must opt in with `environments: [dev, int]` in `bear.artifact.yml`, and
every entry must be one the project declares. Both mistakes fail the job before any
step runs:

```text
Error: error creating plan: unknown environment "production": bear.config.yml declares dev, int, prd
Error: error creating plan: artifact "api" allows undeclared environment "preprd"; bear.config.yml declares dev, int, prd
```

Absent or empty allowlists deny deployment everywhere, but retain change
detection and dependency propagation. Each dependent uses its own allowlist. Neither `--pin`
nor `--force` bypasses policy.

Inherited or configured `ENVIRONMENT` cannot select policy. The positional value
is injected as `$ENVIRONMENT` into every step `bear apply` runs for a deploying
artifact, overriding
inherited/configured values. Credentials and targets remain separately configured.
The selected environment, allowlists, and decisions are saved in the plan; apply
does not re-read policy, and never reads `bear.config.yml` at all, so no change to
the declared environments can make it reject a plan an earlier job already approved.
Editing that file still invalidates an in-flight plan, but as tracked source through
the [fingerprint check](concepts/plan-apply.md#source-safety), not as policy.
Replan and reapprove after changing policy or environment.
Disabled deployments do not update history.

After CLI argument parsing succeeds and the workspace lock is acquired, planning
invalidates the previous saved plan before loading configuration.
A missing argument (`bear plan`) is a syntax error and leaves
the old plan untouched; a rejected environment (`bear plan qa` where the project
does not declare `qa`) starts
planning and clears it. Run apply only after a successful plan.

History and pins are scoped per environment and artifact. Legacy artifact-only
history is retained but ignored, not automatically mapped to environments; replan
and review each environment. History under a retired environment is kept and is
neither an error nor a warning; `bear list` just stops showing it. See
[Source Safety](concepts/plan-apply.md#source-safety)
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

## Merge Request Validation

`bear validate` is the merge-request command in the chain `doctor` → `validate` →
`plan` → `apply`. It runs every artifact's and library's build steps against
the checked-out working tree and reports which ones pass. It takes no environment
argument, selects no deployment policy, detects no changes, reads no deployment
history, neither reads nor writes `.bear/plan.yml` or `bear.lock.yml`, and runs no
Git at all.

A validation job therefore needs **no deployment credentials, no environment, and
no Git history**. Give it a read-only shallow checkout and the language toolchains
its steps use, and nothing else. It must never run `bear apply`: a change request
can execute arbitrary repository commands, so the job that runs untrusted code is
exactly the job that must hold no deployment secrets. See
[Trusted Deployments](#trusted-deployments).

`bear plan` runs no commands at all — it is a pure decision, not a build. It
never risks running untrusted steps just to show a diff, but that also means a
plan proves nothing about whether the code builds. `bear apply` is where building
actually happens: for every artifact it deploys, it runs the exact same
`languages.<lang>.steps` `bear validate` runs, immediately before that artifact's
deploy steps. Validate does not duplicate a check plan performs — plan performs
none — it moves that same check earlier, into the merge request, where there is
no environment to choose and nothing to approve. If a merge-to-main pipeline
wants build/test assurance before it even creates a deployment plan, run
`bear validate` as its own step; see [Trusted Deployments](#trusted-deployments).

Validate does not scope work to the changed files. It validates everything, or
exactly the artifacts you name, so in a large monorepo the artifact filter and
`--concurrency` are the two levers on how long a merge request waits. Bear does
not infer the affected set from a base ref, a diff, or CI metadata.

### GitHub Actions Merge Request Job

```yaml title=".github/workflows/validate.yml"
name: Validate
on:
  pull_request:
permissions:
  contents: read
jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      # Provision a reviewed Bear release and the toolchains your steps use.
      - run: bear validate
        working-directory: examples
```

There is no `fetch-depth: 0`, no `environment:`, and no secret. The default
shallow checkout is enough because validate never runs Git, and the read-only
`contents` permission is all it needs. Narrow a large monorepo by naming
artifacts, as in `bear validate user-api order-api`, and lower `--concurrency`
when steps contend for one shared resource.

### GitLab CI Merge Request Job

```yaml title=".gitlab-ci.yml"
validate:
  stage: validate
  image:
    name: your-registry/bear-ci:reviewed
    entrypoint: [""]
  rules:
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
  script:
    - cd examples
    - bear validate
```

The job inherits no `DEPLOY_SSH_KEY`, needs no `GIT_DEPTH`, and declares no
`environment:`. Do not copy `resource_group` from the deployment job: separate
merge requests validate in separate runner checkouts and do not contend. Two Bear
processes cannot validate the *same* workspace at once — validate takes the
nonblocking `.bear/workspace.lock` and fails rather than waiting — but that is a
local guard, not a pipeline-wide one.

### Jenkins Merge Request Stage

`examples/Jenkinsfile` and `examples/Jenkinsfile.docker` both carry a validation
stage guarded by `changeRequest()`:

```groovy
stage('Validate') {
    when {
        allOf {
            changeRequest()
            expression { env.LOCK_ONLY != 'true' }
        }
    }
    steps {
        dir('examples') { sh 'bear validate' }
    }
}
```

No `withCredentials` block wraps it, so a change-request build never receives the
`git-credentials` binding the deployment stage uses, and the `Deploy` stage's
`not { changeRequest() }` guard keeps the two mutually exclusive. In both
templates the `Plan` stage carries that same complementary guard, so a change
request validates instead of producing a production plan it can never apply. Do
not mount a host Docker socket into an untrusted change-request job.

## Trusted Deployments

The templates use `--git-remote` and `--git-branch` to select the lock publication
destination explicitly. Bear reports Git failures as nonzero errors and retains
the plan and deployment history for recovery.
Only a protected, trusted `main` job receives write credentials or runs apply.
PR jobs can execute arbitrary repository commands and must not receive deployment
secrets. Configure protected branches/environments and CI trust policy separately.

`bear plan` here only decides what would deploy; `bear apply` is what actually
builds and deploys it, running each deploying artifact's language steps and then
its target steps together. If a merge-to-main pipeline wants build/test feedback
before it even creates a production plan — for example to fail fast on a merge
commit without waiting for `bear apply` to reach that artifact — add an explicit
`bear validate` step ahead of `bear plan`, in the same trusted job or an earlier
one. It costs an extra run of the same steps `bear apply` runs anyway, in
exchange for finding a broken build before a plan is even written.

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
validate:
  stage: validate
  rules:
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
  script:
    - cd examples
    - bear validate
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

The merge-request `validate` job is deliberately credential-free: it inherits the
identity variables but never `DEPLOY_SSH_KEY`, and it runs no Git, so the pipeline's
`GIT_DEPTH: "0"` exists only for the deployment job's change detection. Keep
`bear apply` in the protected-branch job alone. See
[Merge Request Validation](#merge-request-validation).

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

A change request instead runs the credential-free `Validate` stage described in
[Merge Request Validation](#merge-request-validation), and its `changeRequest()`
guard is the complement of the `Plan` and `Deploy` stages'
`not { changeRequest() }`, so a change request never produces or applies a plan.

For a parameterized pipeline that can also freeze a deployable to a fixed commit
or explicitly unfreeze one, see
[`examples/Jenkinsfile.freeze`](https://github.com/irevolve/bear/blob/main/examples/Jenkinsfile.freeze)
and [Freeze & Unfreeze (Jenkins)](concepts/freeze-unfreeze.md). It reuses the same
identity variables, `gitUsernamePassword` binding, and trusted-branch guards as
the two templates above, adding `REF`/`DEPLOYABLE`/`FORCE` build parameters around
`--pin` and `--force`.

## Approval And State

Plan for the intended environment, review, then approve apply in a protected job.
A normal plan with any deployments requires clean source before it runs; plan
itself never modifies HEAD or tracked source, since it runs no commands at all.
If separate jobs transfer a plan, check out the exact same revision: apply
verifies the saved commit and fingerprint match before it builds or deploys
anything, and since apply always builds fresh right before it deploys, the
receiving checkout does not need to already contain build output from wherever
the plan was created — only the same tracked commit and nonignored untracked
files. Paths in the plan are project-relative, not tied to the original absolute
checkout directory. Ignored dependencies and outputs are excluded from the
fingerprint regardless, so provision them the same way you would for any fresh
build. A pinned apply checks the fingerprint of a fresh private worktree before
it runs any step, then builds and deploys from that worktree exactly like a
normal deployment. See [Source Safety](concepts/plan-apply.md#source-safety).

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

Without a terminal — Jenkins `sh`, GitLab job logs, GitHub Actions — progress is
Terraform-style: one plain line per job on every status change. There is no ASCII
progress bar, spinner, cursor movement, or ANSI colour, so each line is appended
once and stays readable in a scrollback log. Run Bear directly in the job's shell,
without `returnStdout: true` or other capture. No TTY or ANSI plugin is required.

```text
Bear Apply
──────────

Deploying 2 artifacts to prd

  checkout-api:       Deploying...
  checkout-api:       Deploying... [1/3 Build image]
  checkout-api:       Still deploying... [10s elapsed, 1/3 Build image] (1 job queued)
  checkout-api:       Deploying... [2/3 Push image]
  checkout-api:       Deploying... [3/3 Deploy revision]
  checkout-api:       Deployment complete after 15s
  kira-teams-adapter: Deploying...
  kira-teams-adapter: Deploying... [1/3 Build image]
  kira-teams-adapter: Deploying... [2/3 Push image]
  kira-teams-adapter: Deployment failed after 11s: Push image: exit status 1
    denied-missing-registry-credentials
```

Each command opens with its own branding header — `Bear Plan` or `Bear Apply` —
and each phase is a plain bold heading. Apply's heading names its destination,
`Deploying 2 artifacts to prd`, so the environment is stated once beside the work.
Phases carry no banner rules, so the log reads as a single column of text.

A job reports when it starts, when it enters a step, and when it finishes. Job
names are padded to a common width, so the status text of every line starts in the
same column. The bracketed detail is the current step, numbered `<n>/<total>`
when there is more than one. For apply, `<total>` counts the deploying artifact's
language build steps and target deploy steps together as one sequence — build
steps first, deploy steps after — not the target's steps alone; the example above
shows `checkout-api` with three target steps and no language build steps
configured, so its count happens to be the target's alone. Failure lines end with
the error and are followed by the bounded output tail, indented four spaces under
the job line that reported it.

Every running job additionally repeats a `Still ...` line every 10 seconds with
its total elapsed time and current step, so a silent long-running command never
looks hung and log-timeout watchdogs keep seeing output. Jobs still waiting for a
`--concurrency` slot are appended to the last running job's heartbeat as
`(1 job queued)` or `(2 jobs queued)`; the backlog never takes a line of its own.
The example above ran with `--concurrency 1`, which is why its second artifact
waited.

Each command names its own lifecycle: apply uses `Deploying` / `Still deploying` /
`Deployment complete` / `Deployment failed` for every step it runs — build steps
and deploy steps alike, there is no separate phase for either. `bear validate`
uses `Validating` / `Still validating` / `Validation complete` /
`Validation failed`. `bear plan` prints no progress lines at all: it runs no
commands, so it goes straight from its branding header to the summary. `bear
doctor` uses `Checking` / `Still checking` / `Check complete` / `Check failed`.

Interactive terminals keep the animated progress bar and per-task spinner instead
of these lines. Those per-task lines use the same `<n>/<total> <step>` labels, and
an apply task is named by its artifact alone rather than `<artifact> -> <target>`.
`--verbose` selects the same plain, line-per-status reporting even on a terminal,
and streams subprocess output as `  <artifact> | <step> | <line>` while retaining
bounded failure diagnostics. Without it, step output is captured and only failure
tails are reported. Treat logs as sensitive; bounded capture does not redact secrets.

```bash
bear validate --concurrency 5
bear apply --concurrency 3
bear plan dev --verbose  # accepted, but plan runs nothing to stream
```

`--concurrency` defaults to `10` for validate and apply, the two commands that
actually run steps. `bear plan` has no `--concurrency` flag at all — it never ran
anything in parallel after the plan/apply redesign, so the flag was removed
rather than kept as a silent no-op. `-v/--verbose` is still accepted, since it is
the global root flag, but plan runs no commands, so it has no visible effect
there. A lower `--concurrency` is often appropriate for production
apply: it limits how many artifacts change at the same instant, spreads load on
deployment APIs and registry rate limits, and keeps the log readable. It bounds
only how many jobs run in parallel — a failed deployment does not stop the
remaining ones, and apply is not atomic or ordered. See
[apply flags](commands/apply.md#flags) and
[validate flags](commands/validate.md#flags).

The two commands deliberately print different endings. `bear plan` prints its
branding header, then immediately the rule, its `Environment:`/`Commit:` facts,
its `deploy`/`skip` lists, and
`Plan complete: 3 changed, 1 to deploy, 2 skipped`: that block is the artifact a
reviewer approves, produced without running anything. `bear apply` does not
repeat it. A successful apply is the phase
heading, the job lines, and `Apply complete: 1 deployed, 1 skipped in 0s`; only a
failing apply prints a rule, `Environment: <env>`, and a `failed (N):` list, so a
failure is never buried in a recap of the plan. Either way the last sentence lets a
long CI log be read from the bottom up. See [plan output](commands/plan.md#output)
and [apply output](commands/apply.md#output).
