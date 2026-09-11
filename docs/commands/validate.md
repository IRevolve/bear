# bear validate

Run the validation steps of every artifact and library against the working tree.
No environment, no change detection, no plan, no deployment.

```bash
bear validate [artifacts...]
```

```bash
bear validate                       # Validate every artifact and library
bear validate user-api              # Validate a single artifact
bear validate user-api order-api    # Validate multiple artifacts
bear validate --environment int     # Inject ENVIRONMENT=int into the steps
bear validate --concurrency 5       # Limit parallel validation jobs
bear validate -v                    # Stream step output while it runs
bear validate -d ./other-project    # Validate a different directory
```

Validate is the merge-request command in the chain
[`doctor`](doctor.md) → `validate` → [`plan`](plan.md) → [`apply`](apply.md).
`bear doctor` reads the configuration, `bear validate` runs the configured build
steps, `bear plan <environment>` produces the deployment a human reviews without
running anything, and `bear apply` builds and deploys it. `bear apply`'s build
phase runs the exact same `languages.<lang>.steps` validate does, immediately
before each deploying artifact's target steps — so validate is not a rehearsal
for a second, different run; it is the same check, available earlier and for
free of any environment or deployment context. Run it in the merge request for
fast feedback, and rely on apply to run it again, for real, right before every
deployment.

## Arguments

| Argument | Description |
|----------|-------------|
| `[artifacts...]` | Optional artifact and library names; omit to validate everything discovered |

Every positional argument is an artifact name. There is no environment argument:
`bear validate int` fails with `unknown artifact "int"`.

Without arguments, every discovered artifact **and** library in the current
working tree is validated. Naming artifacts validates exactly those: dependencies
are never added, so `bear validate checkout-api` does not also validate the
`shared` library it depends on. A repeated name is validated once.

A name that matches nothing is an error, so a typo fails the job instead of
quietly validating nothing:

```text
Error: unknown artifact "chekout-api"
```

Several misses are reported together, sorted:
`unknown artifact "other", "typo"`. An unmatched name rejects the whole
selection before the first step runs, so nothing is validated.

Selection ignores deployment policy entirely. An artifact with no `environments`
allowlist can never be deployed anywhere and is still validated.

An allowlist is still checked for *validity*, though, because validate loads the
whole artifact graph before it runs anything. An entry naming an environment the
project does not
[declare](../configuration.md#deployment-environments) is a configuration error, and
it fails the run whatever you selected:

```text
Error: error loading artifacts: artifact "web" allows undeclared environment "preprd"; bear.config.yml declares dev, int, prd
```

## Flags

| Flag | Description |
|------|-------------|
| `--environment <name>` | Optional; must be declared in `bear.config.yml`. Injects `ENVIRONMENT` into the validation steps |
| `--concurrency <n>` | Max parallel validation jobs (default: `10`) |
| `-d, --dir <path>` | Project directory (default: `.`) |
| `-v, --verbose` | Stream subprocess output while retaining bounded failure diagnostics |

`--environment` only names the variable handed to the steps, with the same
precedence a plan's validation uses — language vars, then target vars, then
artifact vars, then the selected environment — so a step behaves identically in a
merge request and in a deployment. It selects no deployment policy, reads no
history, and changes no verdict. It must still name an environment the project
[declares](../configuration.md#deployment-environments), so a typo cannot silently
inject a value no deployment will ever use. Invalid values are rejected before the
workspace is touched: with a project declaring `dev`, `int`, `prd`,
`--environment production` fails with
`unknown environment "production": bear.config.yml declares dev, int, prd`.
Without the flag,
steps inherit whatever `ENVIRONMENT` the CI job already exports; Bear injects
nothing and does not treat an inherited value as a selection.

The deployment flags are not accepted: `--pin`, `--no-commit`, `--git-remote`,
and `--git-branch` fail with `unknown flag`. The global `-f, --force` is accepted
because it is a root flag, but it has nothing to override here and does nothing.

`--concurrency` defaults to `10`, the same default as `bear plan` and
`bear apply`. Lower it when steps contend for one shared resource — a Docker
daemon, a test database, a rate-limited registry — or when interleaved job output
is hard to read. It bounds only how many jobs run at once: a failing artifact does
not stop the others, and every selected artifact is attempted.

## What It Does Not Do

Validation has no deployment semantics:

- **No environment argument and no deployment policy.** `environments`
  allowlists gate nothing here, no artifact is enabled or blocked, and nothing is
  approved. They are still checked against the project's declared environments,
  because that is a configuration error, not a policy decision.
- **No change detection.** Every selected artifact runs whether or not it
  changed. See [Cost](#cost).
- **No deployment history.** `bear.lock.yml` is neither read nor written.
- **No plan.** `.bear/plan.yml` is neither read, written, nor removed. A plan
  saved by an earlier `bear plan` survives a validate run untouched, so validating
  cannot invalidate an approval.
- **No deploy or skip output.** There is no `Environment:` fact block, no
  `deploy (`/`skip (` section, and no `Run 'bear apply'` hint.
- **No Git.** Validate never shells out to Git. It works in a shallow clone, a
  single-commit CI checkout, or a plain export that is not a repository at all. It
  cannot require clean source, because it never looks at source control.

It does take the nonblocking advisory workspace lock on `.bear/workspace.lock`,
creating `.bear/` if needed, because it reads a working tree that a concurrent
plan or apply may be rewriting. Running it against a workspace another Bear
process holds fails immediately rather than waiting:

```text
Error: cannot lock workspace /src: cannot acquire lock /src/.bear/workspace.lock (another bear process may be running): resource temporarily unavailable
```

It takes no repository lock, because it runs no Git and publishes nothing. See
[History And State](../concepts/plan-apply.md#history-and-state) for what those
two locks do and do not guarantee.

## Cost

Validate deliberately does **not** scope work to the changed files. It validates
everything, or exactly the artifacts you name. In a large monorepo that costs
time on every pull request, and Bear offers exactly two levers: name the
artifacts a job should cover, and tune `--concurrency`. Deciding which artifacts
a merge request needs is the caller's job — Bear does not infer it from a base
ref, a diff, or CI metadata.

## Output

Validate brands itself like plan and apply, reports each job while it runs, and
closes with one sentence:

```text
Bear Validate
─────────────

Validating 4 artifacts

  shared:             Validating...
  generator:          Validating...
  generator:          Validating... [no validation steps]
  generator:          Validation complete after 0s
  checkout-api:       Validating...
  checkout-api:       Validating... [1/2 Test]
  kira-teams-adapter: Validating...
  kira-teams-adapter: Validating... [1/2 Test]
  shared:             Validating... [1/2 Test]
  shared:             Validating... [2/2 Build]
  kira-teams-adapter: Validating... [2/2 Build]
  checkout-api:       Validating... [2/2 Build]
  shared:             Validation complete after 3s
  checkout-api:       Validation complete after 3s
  kira-teams-adapter: Validation complete after 3s

Validation complete: 4 artifacts in 3s
```

Progress uses the same reporting as plan and apply: without a terminal, one plain
line per job on every status change, job names padded into a common column, a
`Still validating...` line every 10 seconds per running job, and the remaining
backlog appended to the last running job's heartbeat as `(1 job queued)`.
Interactive terminals draw an animated progress bar instead, and `--verbose`
selects the plain display even on a terminal while streaming step output as
`  <artifact> | <step> | <line>`. See [Live Output](../ci-cd.md#live-output).

An artifact whose language defines no validation steps is reported as
`no validation steps` and completes successfully. That keeps a silently
unvalidated artifact visible without turning a deliberate configuration into a
failure.

A failing run reopens the summary with the failures only, the way a failed apply
reports:

```text
  checkout-api:       Validation failed after 1s: Test: exit status 1
    FAIL Test/checkout-api: assertion failed
  shared:             Validation failed after 2s: Build: exit status 2
    FAIL Build/shared: compile error
  kira-teams-adapter: Validation complete after 4s

────────────────────────────────────────

failed (2):
  - checkout-api (services/checkout-api): Test: exit status 1
  - shared (libs/shared): Build: exit status 2

Validation failed: 2 passed, 2 failed in 4s
```

A successful run prints no rule and no summary block at all, so the block's
presence is itself the signal. It carries no `Environment:` line, because
validation selected none. Entries are one line each,
`  - <name> (<path>): <reason>`, with the failing step's error as the reason,
sorted by name so two runs are comparable even though jobs finish in any order.
The list indexes the log above it: the bounded output tail stays indented four
spaces under the job line that reported the failure. Treat those tails as
sensitive; capture is bounded, not redacted.

## Exit Code

`0` when every selected artifact passed, including artifacts with no validation
steps. `1` for any failing step, an unknown artifact name, an `--environment` the
project does not declare, a missing `bear.config.yml`, a configuration or discovery
error such as an undeclared allowlist entry,
or a workspace held by another Bear process. A failing run also prints the joined
step errors on stderr after the summary:

```text
Error: validation failed: Build: exit status 2
Test: exit status 1
```

## In CI

Run it in the merge/pull request job. It needs no deployment credentials, no
environment, and no Git history, and it must never be paired with `bear apply` in
the same job. See
[Merge Request Validation](../ci-cd.md#merge-request-validation) for GitHub
Actions, GitLab CI, and Jenkins templates.
