# bear apply

Execute the plan from `.bear/plan.yml`. For every deploying artifact, apply is the
only place anything ever runs: it runs the language's build steps
(`languages.<lang>.steps`) first, then the target's deploy steps, in one
continuous progress task per artifact. This is uniform for every deployment the
plan approved, pinned or not — there is a single code path, not a special case for
pins. Deploy in parallel, update lock file.

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
environment, are not deployed or recorded in the lock file. A plan with nothing to
deploy has nothing for apply to run.

## The Saved Plan Is the Authority

Apply deliberately never reads the current `bear.config.yml`. The saved plan is the
approved snapshot, so **no change to the project's declared environments can make
apply reject an approved plan on policy grounds**: a plan for `preprd` still applies
to `preprd` after the project stops declaring it, and the lock history it writes is
keyed under `preprd`. Apply also runs on a config that would not even load, which is
why upgrading Bear never strands an in-flight approved plan.

What apply does check is only the saved evidence: the saved environment name's
syntax, each artifact's **saved** allowlist, and each saved `ENVIRONMENT` variable.

```text
Error: unsafe saved plan: invalid environment name "Production": use 1 to 32 characters matching [a-z][a-z0-9-]*; run 'bear plan <environment>' again
Error: unsafe saved plan: artifact "api" has no saved deployment permission for environment preprd; run 'bear plan <environment>' again
Error: unsafe saved plan: artifact "api" ENVIRONMENT does not match selected environment preprd; run 'bear plan <environment>' again
```

The name check is syntax only — `[a-z][a-z0-9-]*`, 1 to 32 characters — so a plan
with no environment, or one saved as `Production`, is refused, while a well-formed
name the project no longer declares is not. Those checks are permission evidence,
not an instruction to reload policy, so none of them names a fixed set of
environments. Apply also refuses a plan whose source commit or fingerprint is
missing, and a duplicate artifact or mismatched pin. Every one of these preflight
failures happens before any subprocess, lock write, or Git command, and leaves the
plan and lock file untouched.

!!! warning "Editing the config still breaks the plan — as source, not as policy"
    `bear.config.yml` is ordinary tracked source, and only `bear.lock.yml` and
    `.bear/` are exempt from the
    [source fingerprint](../concepts/plan-apply.md#source-safety). Editing it in the
    approved checkout between plan and apply therefore fails with
    `source commit or fingerprint does not match saved plan; run 'bear plan
    <environment>' again` — the same result as editing any other file, and nothing to
    do with what the file says about environments. Apply an in-flight plan before
    changing the config, or replan afterwards.

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
keeps the log readable. This bounds every step apply runs — build and deploy
alike — not only the deploy half.

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
then commit/push if enabled.

For every pending artifact, "deploy" here means running its language's build
steps and then its target's deploy steps back to back, in the same subprocess
sequence, whether or not the plan was pinned. Apply requires the saved HEAD and
fingerprint to still match the source before it runs anything; unlike plan, it
never validated that source in the first place, so this check is the only
guarantee that what apply is about to build is what was approved. A pinned plan's
"source" is a fresh private worktree at the saved commit rather than the caller's
checkout, created and fingerprint-checked before build steps run, then cleaned up
after use — there is no separate replay phase, because apply always builds before
it deploys regardless of how the source was selected.

Each successful deployment atomically saves environment history, then checkpoints
completion in the plan. Failed runs retain the plan. Retries skip completed artifacts
only when history matches. Pending artifacts still undergo source checks, and
still run their full build-then-deploy sequence from the start — apply has no
notion of resuming a partially completed artifact's steps. Fully completed
retries run no source or step commands at all.

Persistence failures can leave external deployments successful without a completed
checkpoint. Inspect and reconcile deployment status before retrying; local atomic
writes do not form a transaction with an external service. See
[Retries](../concepts/plan-apply.md#retries) and
[Source Safety](../concepts/plan-apply.md#source-safety).

## Output

The plan is the review artifact; apply is the execution log. Everything apply would
recap — the environment, the deploy list, the skips — was already printed and
approved by `bear plan`, so apply does not repeat it. There is exactly one phase,
regardless of whether any artifact is pinned: `Deploying N artifact(s) to <env>`.
Each artifact's step counter spans build and deploy steps together — build steps
first, deploy steps after, as one numbered sequence. A successful run is the phase
heading, the job lines, and the closing sentence:

```text
Bear Apply
──────────

Deploying 1 artifact to prd

  api: Deploying...
  api: Deploying... [1/4 Test]
  api: Deploying... [2/4 Build]
  api: Deploying... [3/4 Build image]
  api: Deploying... [4/4 Push image]
  api: Deployment complete after 0s

Apply complete: 1 deployed, 1 skipped in 0s
```

Here `api`'s language defines two steps (`Test`, `Build`) and its target defines
two more (`Build image`, `Push image`); apply numbers all four as one sequence,
`[1/4]` through `[4/4]`, rather than resetting the counter when deployment starts.
There is no rule, no `Environment:` block, and no `deploy (`/`skip (` section on
success. The phase heading names the environment — `Deploying 1 artifact to prd`
— so the destination is still stated once, next to the work it describes, and the
per-job lines already report every artifact that ran. Unless `--no-commit` was
given, a dimmed `  Lock file committed with [skip ci]` line follows the sentence
once the lock file is published.

A failing build step fails apply exactly like a failing deploy step: the same
progress line, the same `failed (N):` entry, and the same nonzero exit. The
failing step's name is part of the reason either way, so the summary alone tells
you whether a deployment failed before or after it started actually deploying:

```text
Bear Apply
──────────

Deploying 1 artifact to prd

  api: Deploying...
  api: Deploying... [1/4 Test]
  api: Deployment failed after 0s: Test: exit status 1

────────────────────────────────────────
Environment: prd

failed (1):
  - api (services/api): Test: exit status 1

Apply failed: 0 deployed, 1 failed, 1 skipped in 0s
```

The rule, `Environment: <env>`, and the red `failed (N):` list are printed only
when a pending deployment did not complete, so failures are never buried in a recap
of things that went fine. Entries are one line each,
`  - <name> (<path>): <reason>`, with the failing step's error as the reason —
`Test: exit status 1` for a failing build step, `Push image: exit status 1` for a
failing deploy step, the same format either way — sorted by name so two runs of
the same plan are comparable even though jobs finish in any order. This list is an
index into the log above it, not a replacement: the bounded output tail stays
indented four spaces under the job line that reported the failure.

Apply closes with one sentence: `Apply complete: 1 deployed, 1 skipped in 0s`, or
`Apply failed: 0 deployed, 1 failed, 1 skipped in 0s` when a deployment did not
complete. The deployed count is always reported; failed and skipped counts appear
only when there are any. The skipped count covers both the plan's recorded skips
and artifacts already checkpointed by an earlier run.

On a retry, artifacts checkpointed by an earlier run are not redeployed, and they
are not listed anywhere — only the `skipped` count includes them. `bear.lock.yml`
and the retained plan are the record of what is already deployed. A plan with no
artifacts short-circuits before the `Bear Apply` header: it prints exactly one
line, `Plan for prd contains no artifacts to deploy.`, then removes the plan.

Without a terminal, every job reports one line per status change with names
padded into a common column, plus a `Still deploying...` line every 10 seconds
carrying the remaining backlog as `(1 job queued)` on the last running job;
interactive terminals draw an animated progress bar instead. Failure lines are
followed by the bounded output tail indented four spaces, and `--verbose` streams
subprocess output as `  <artifact> | <step> | <line>` for both build and deploy
steps alike. See [Live Output](../ci-cd.md#live-output) and
[plan output](plan.md#output).

## Plan File Schema

Each artifact apply runs comes from `.bear/plan.yml`'s `artifacts` list. The two
step lists it saves map directly onto this page's execution order:

| Field | Description |
|-------|-------------|
| `build_steps` | The language's steps (`languages.<lang>.steps`), run first |
| `steps` | The target's deploy steps, run after `build_steps` |

Both are ordinary saved `Step` lists — apply does not distinguish them once it
starts running; they are simply concatenated into one sequence per artifact for
numbering and execution. See [Plan/Apply Workflow](../concepts/plan-apply.md) for
the rest of the schema and [Configuration](../configuration.md) for how
`build_steps` and `steps` are populated from `languages` and `targets`.

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
</content>
