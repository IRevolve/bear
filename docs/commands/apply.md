# bear apply

Execute the plan from `.bear/plan.yml`. Deploy in parallel, update lock file.

```bash
bear apply                     # Execute plan
bear apply --no-commit         # Don't auto-commit lock file
bear apply --concurrency 3     # Limit parallelism
bear apply --git-remote origin --git-branch main # Explicit CI push destination
```

Apply uses the environment, allowlists, and deploy/skip decisions snapshotted by
`bear plan`; it does not re-evaluate current artifact policy. A different `ENVIRONMENT`
job variable at apply time cannot change the saved selection. Replan after changing
policy or the intended environment. Old saved plans lacking allowlist snapshots
must be regenerated before apply.

Artifacts with absent or empty allowlists, or allowlists excluding the selected
environment, are not deployed or recorded in the lock file. A validation-only plan
has nothing to deploy.

## Flags

| Flag | Description |
|------|-------------|
| `--no-commit` | Skip commit and push; retain local lock history and remove the completed plan |
| `--concurrency <n>` | Max parallel deployments (default: `10`) |
| `--git-remote <remote>` | Configured remote for lock publication (default: `origin`) |
| `--git-branch <branch>` | Explicit existing branch, e.g. `main`, including detached HEAD |
| `--verbose` | Stream subprocess output while retaining bounded failure diagnostics |

A lower `--concurrency` is often appropriate for production. Ten simultaneous
deployments mean ten artifacts changing at the same instant, which is harder to
correlate with monitoring, and puts the whole batch's load on deployment APIs,
registries, and their rate limits at once. A smaller number spreads that out and
keeps the log readable.

It bounds only how many deployments are **in flight** at a time. It does not make
apply atomic or ordered, and a failed artifact does **not** stop the queue: the
remaining deployments still run, and a completed one stays deployed. Only a
checkpoint-persistence failure cancels the rest of the run. See
[Retries](../concepts/plan-apply.md#retries).

Restrict write credentials and apply to trusted branches;
validate the branch value rather than using untrusted PR metadata.

## Flow

Preflight the saved policy and paths, verify completed checkpoints against lock
history, verify source for pending deployments, deploy, persist each completion,
then commit/push if enabled. Normal apply requires the saved HEAD and fingerprint;
it does not rerun validation. Pinned apply recreates the saved revision in a private
worktree and reruns saved validation/setup steps before checking the fingerprint.

Each successful deployment atomically saves environment history, then checkpoints
completion in the plan. Failed runs retain the plan. Retries skip completed artifacts
only when history matches; pending artifacts still undergo source checks. Fully
completed retries run no source commands. Success removes the plan, including a
validation-only plan with nothing to deploy.

Persistence failures can leave external deployments successful without a completed
checkpoint. Inspect and reconcile deployment status before retrying; local atomic
writes do not form a transaction with an external service. See
[Retries](../concepts/plan-apply.md#retries) and
[Source Safety](../concepts/plan-apply.md#source-safety).

## Output

The plan is the review artifact; apply is the execution log. Everything apply would
recap — the environment, the deploy list, the skips — was already printed and
approved by `bear plan`, so apply does not repeat it. A successful run is the phase
heading, the job lines, and the closing sentence:

```text
Bear Apply
──────────

Deploying 2 artifacts to prd

  checkout-api:       Deploying...
  checkout-api:       Deploying... [1/3 Build image]
  kira-teams-adapter: Deploying...
  kira-teams-adapter: Deploying... [1/3 Build image]
  checkout-api:       Still deploying... [10s elapsed, 1/3 Build image]
  checkout-api:       Deployment complete after 15s
  kira-teams-adapter: Deployment complete after 15s

Apply complete: 2 deployed, 1 skipped in 15s
```

There is no rule, no `Environment:` block, and no `deploy (`/`skip (` section on
success. The phase heading names the environment — `Deploying 2 artifacts to prd`
— so the destination is still stated once, next to the work it describes, and the
per-job lines already report every artifact that ran. Unless `--no-commit` was
given, a dimmed `  Lock file committed with [skip ci]` line follows the sentence
once the lock file is published.

A failure brings the summary block back, holding only the failures:

```text
  kira-teams-adapter: Deployment failed after 11s: Push image: exit status 1
    denied-missing-registry-credentials

────────────────────────────────────────
Environment: prd

failed (2):
  - checkout-api (services/checkout-api): Push image: exit status 1
  - kira-teams-adapter (services/kira/teams-adapter): Push image: exit status 1

Apply failed: 0 deployed, 2 failed, 1 skipped in 11s
```

The rule, `Environment: <env>`, and the red `failed (N):` list are printed only
when a pending deployment did not complete, so failures are never buried in a recap
of things that went fine. Entries are one line each,
`  - <name> (<path>): <reason>`, with the failing step's error as the reason, sorted
by name so two runs of the same plan are comparable even though jobs finish in any
order. This list is an index into the log above it, not a replacement: the bounded
output tail stays indented four spaces under the job line that reported the
failure.

Apply closes with one sentence: `Apply complete: 2 deployed, 1 skipped in 15s`, or
`Apply failed: 0 deployed, 2 failed, 1 skipped in 11s` when a deployment did not
complete. The deployed count is always reported; failed and skipped counts appear
only when there are any. The skipped count covers both the plan's recorded skips
and artifacts already checkpointed by an earlier run.

On a retry, artifacts checkpointed by an earlier run are not redeployed, and they
are not listed anywhere — only the `skipped` count includes them. `bear.lock.yml`
and the retained plan are the record of what is already deployed. A plan with no
artifacts short-circuits before the `Bear Apply` header: it prints exactly one
line, `Plan for prd contains no artifacts to deploy.`, then removes the plan.

Pinned apply reruns saved validation before deploying, in its own
`Validating <n> artifacts` phase reported with `Validating...` /
`Still validating...` / `Validation complete` lines and closed by
`Validation complete: <n> artifacts in <time>`. Without a terminal, every job
reports one line per status change with names padded into a common column, plus a
`Still deploying...` line every 10 seconds carrying the remaining backlog as
`(1 job queued)` on the last running job; interactive terminals draw an animated
progress bar instead. Failure lines are followed by the bounded output tail
indented four spaces, and `--verbose` streams subprocess output as
`  <artifact> | <step> | <line>`. See [Live Output](../ci-cd.md#live-output) and
[plan output](plan.md#output).

## Git Publication

Bear commits only the lock file using a private Git index, leaving the caller's
index untouched. Existing staged lock content can appear as a reverse change
relative to the new HEAD. Normal Git hooks run; Bear does not configure identity,
disable hooks, reset, or force-push. Supply author/committer identity and temporary
authentication as in the [CI examples](../ci-cd.md#jenkins).

The remote must resolve to one push URL. The target branch must already exist and
its remote tip must equal local HEAD before Bear creates the lock commit. This
prevents publishing unrelated local commits. Without `--git-branch`, Bear uses the
current branch, or unambiguous non-PR Jenkins hints for a detached checkout. Other
detached or ambiguous checkouts need an explicit branch. Only HEAD is pushed to
that branch; tags and submodules are not automatically published.

Any Git failure returns nonzero and retains the plan and deployment history.
Fix identity/authentication issues before retrying. If the commit succeeded but
the push failed, the local commit is retained: **inspect HEAD and the remote and
retry the push manually**, as the helper's error directs. Rerunning apply alone
will refuse the now-local-ahead HEAD, not push it automatically. After safely
publishing that exact lock commit and reconciling state, rerun apply to finish;
matching completed checkpoints prevent redeployment. Never force-push or discard
the checkpoint just to make the job green.
