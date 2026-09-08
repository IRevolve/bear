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
