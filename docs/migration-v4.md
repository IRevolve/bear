# Migrate a Monorepo to v4

v4.0.0 changes deployment policy, history, and saved plans. Pause automated apply
jobs while updating configuration and reviewing the first plan for each environment.
See [release notes](releases/v4.0.0.md) for the significant fixes.

## Upgrade Checklist

1. Pin Bear to v4.0.0 and provision the tools used by your steps.
2. Update CI to `bear plan <environment> [artifacts...]` and add explicit deployment allowlists.
3. Check local YAML, discovery rules, and preset revisions with `bear check` and `bear list --tree`.
4. Preserve `bear.lock.yml`; decide whether to redeploy or manually map verified legacy history.
5. Commit intended source/configuration, recreate old plans, and review each environment before apply.
6. Configure Git publication and retain recovery state if deployment or publication fails.

## Pin the Installation

Use Go 1.25+ to install from the release tag, not a moving branch or `latest`:

```bash
git clone --branch v4.0.0 --depth 1 https://github.com/irevolve/bear.git bear-v4
go -C bear-v4 install -ldflags="-X github.com/irevolve/bear/commands.Version=4.0.0" .
bear --version
```

Put `$(go env GOPATH)/bin` on `PATH` (or your configured `GOBIN`). The tagged source
install avoids relying on a `/v4` Go module path; the module is still
`github.com/irevolve/bear`. Alternatively, use a platform binary from the
[v4.0.0 release](https://github.com/irevolve/bear/releases/tag/v4.0.0).

For container-based agents, pin `ghcr.io/irevolve/bear:4.0.0-debian` or
`ghcr.io/irevolve/bear:4.0.0-alpine`, ideally to a reviewed digest. These include
Git and a shell, but not your language/deployment tools. The scratch image
`ghcr.io/irevolve/bear:4.0.0` has neither Git nor a shell. Jenkins/GitLab agents
need an entrypoint override; see [agent requirements](ci-cd.md#agent-requirements).

## Select the Environment

Every plan requires case-sensitive `dev`, `int`, or `prd` as its **first positional
argument**, including validation-only, pinned, forced, and unchanged selections:

```bash
bear plan dev
bear plan int user-api order-api
bear apply
```

Apply takes no environment argument: it uses the saved selection. Add an allowlist
to each deployable artifact, choosing only the environments you intend to permit:

```yaml title="services/user-api/bear.artifact.yml"
name: user-api
target: cloudrun
language: go
environments: [dev, int]
```

The `cloudrun` target and `go` language must be configured/imported in your project.
Replace obsolete `disabled_environments` with the environments that are **allowed**,
not a copy of the old denied list. Absent `environments` or `[]` denies deployment
everywhere. Applicable validation and dependency propagation remain; each dependent
uses its own allowlist. Libraries never deploy. Neither `--pin` nor `--force`
bypasses policy, and skipped deployments do not update history.

Inherited or configured `ENVIRONMENT` cannot select policy or replace the positional
argument. Bear injects the selection as `ENVIRONMENT` into validation and deployment
steps, overriding those values. It does not select credentials or targets for you.
Policy is snapshotted in the plan, not reloaded at apply; replan and reapprove after
policy or environment changes.

## Review YAML and Discovery

- Project, artifact, and library configs accept exactly one YAML document and reject unknown/obsolete fields.
- Local languages use `detection`, `vars`, and `steps`; local targets use `vars` and `steps`. Names come from map keys. Convert remote-style `validation` phases to ordered `steps`, and target `defaults`/`deploy` to `vars`/`steps` when defining them locally.
- Every step needs nonblank `name` and `run`. Languages require explicit `steps` (`steps: []` opts out of validation); targets must resolve to nonempty deployment steps. Artifacts still require a valid target even when deployment is disabled.
- Multiple matching languages now fail detection. Set `language: <configured-name>` in the artifact or library to resolve ambiguity. No match remains `unknown`: check warns and no language validation runs.
- Discovery skips `.git`, `.bear`, `node_modules`, `vendor`, `generated`, `dist`, and `build`. `ignore_dirs` adds literal names at any depth or project-relative paths, not globs; entries cannot be absolute or contain `..` components or backslashes. Move real artifacts out of default skipped directories.

Review [configuration](configuration.md) for the full schema. `bear check` catches
duplicate names, missing dependencies, unknown targets, and cycles; it does not run
your validation commands. Check the discovered inventory before planning.

### Preset Revisions

Pin the supported built-in preset revision explicitly to keep the selection stable
across Bear upgrades:

```yaml title="bear.config.yml (excerpt)"
use:
  revision: dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a
  languages: [go, node]
  targets: [cloudrun]
```

`use.revision` and CLI `--revision` accept only an immutable 40-character lowercase
commit SHA, not branches, tags, or abbreviated SHAs. Preset index version `1` is
supported. Imports normalize supported remote schemas; complete local definitions
replace presets rather than merging fields. `bear preset` commands use their own
revision, not project config:

```bash
bear preset show language go --revision dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a
bear preset update --revision dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a
```

Update populates/repairs that revision's cache; it does not advance the project to
latest upstream. Valid cached files under `~/.bear/presets/<revision>/` are reusable
offline. Review the [exact-revision Python/Java corrections](commands/preset.md#maintained-corrections)
if previous CI relied on failure-masking fallbacks. `bear init` is now noninteractive;
it imports presets only when requested with `--lang`/`--target`.

## Migrate History Deliberately

Deployment history and pins now live under
`environments.<environment>.<artifact>` in `bear.lock.yml`. Legacy top-level
`artifacts` entries are retained but **ignored**, never automatically assigned to
an environment.

!!! warning "The first run can redeploy"
    Without a per-environment entry, an artifact is treated as new in that environment.
    The first v4 plan therefore schedules eligible artifacts even if legacy history
    says they were deployed. Legacy pins do not protect them either. Review the new
    plan explicitly before apply; do not interpret missing history as proof that the
    remote service has never been deployed.

The recommended path is to preserve legacy history, review a fresh plan, and allow
an intentional deployment to establish each environment's baseline. If redeployment
is inappropriate and you have reliable deployment records, you can **manually** map
an individual legacy entry to its known environment: copy its `commit`, `timestamp`,
`target`, and optional `version`/`pinned` fields from `artifacts.<name>` to
`environments.<known-environment>.<name>`. Verify that the commit is available in
Git and represents what is actually deployed there. Do not overwrite newer history,
copy one entry into every environment, or infer provenance from a branch name.
Back up and review this edit before planning; it asserts deployment state without
performing a deployment. If provenance is uncertain, do not map it.

## Recreate Plans From Clean Source

Do not reuse or hand-upgrade pre-v4 `.bear/plan.yml` files. Deployment plans now
need the environment, saved allowlists/variables, source commit and fingerprint,
and project-relative paths. Run a new successful plan and review it before apply.
Planning invalidates the previous plan once argument parsing succeeds and the
workspace lock is acquired, even if subsequent validation fails.

Normal plans with any deployments require clean Git source **before** validation,
across the whole repository, not just selected services. Commit intended changes
first. Validation must not change HEAD or tracked source; generated nonignored
untracked files are included in the post-validation fingerprint. Apply checks both
the saved commit and fingerprint before pending deployments. Normal apply does not
rerun validation, so separate CI jobs must preserve/reproduce the approved files.

The source footprint covers repository-wide tracked and nonignored untracked files,
including names, modes, contents, and symlink targets. `.bear` state and files named
`bear.lock.yml` are excluded at any depth. Discovery `ignore_dirs` is **not** a
fingerprint exclusion. Ignored untracked dependencies/build outputs are outside the
guarantee; pin and provision them reproducibly yourself. Submodules, nested
repositories encountered as source, special files, and index flags hiding changes
fail closed. Checks are not an atomic filesystem snapshot or a sandbox: keep source
quiescent and use only trusted steps. See [source safety](concepts/plan-apply.md#source-safety).

### Pins and Rollbacks

`bear plan dev user-api --pin <commit>` validates the selected source in a private
detached worktree. Current configuration supplies selection, policy, and steps;
dirty/ignored files from the caller's checkout are not copied. For pending pinned
deployments, apply creates a fresh worktree, reruns saved validation/setup, and
checks the resulting fingerprint before deploying. Setup must reproduce required
outputs. Pin state is environment-specific. `--force` deploys a selected pinned
artifact from current source and clears its pin only after successful deployment;
it is not a general redeploy-all switch. Neither flag enables denied deployments.
Rollback does not undo database migrations or other external effects.

## Update CI and Recovery

Jenkins checkout credentials do not supply a Git commit identity. Set
`GIT_AUTHOR_NAME`, `GIT_AUTHOR_EMAIL`, `GIT_COMMITTER_NAME`, and
`GIT_COMMITTER_EMAIL` in the trusted job. Use the Git plugin's `gitUsernamePassword`
binding (askpass) for HTTPS, or `sshagent` with verified host keys for SSH. Keep
tokens out of remote URLs, Groovy interpolation, and persisted Git config.

Use explicit publication flags in detached checkouts, for example on a protected
`main` job after planning and approval:

```bash
bear apply --git-remote origin --git-branch main
```

The existing remote branch must match local HEAD before the lock commit. Bear will
not publish unrelated local commits, guess ambiguous/PR branch hints, set identity,
disable hooks, or force-push. Give write/deployment credentials only to trusted
jobs, not PR validation. Coordinate independent runners; Bear's repository/workspace
locks serialize cooperating operations locally, not separate clones. See
[Jenkins](ci-cd.md#jenkins) for the bindings and branch guards.

Git commit/push failures now return **nonzero** after deployment and retain the
completed plan. Successful deployments save lock history and then checkpoint
`completed` in the plan. Retries skip completed artifacts only when lock history
matches; fully completed retries do not rerun source validation/deployment commands.
If a push fails after a local commit, inspect that commit and the remote, then
recover the push **manually**. Rerunning apply alone cannot publish a local-ahead
commit. After publication recovery, retry apply to finish the retained plan.

Preserve both the updated lock and checkpointed plan, plus unpublished Git objects
on ephemeral agents. Plans may contain secrets: keep recovery storage private and
restore plan permissions to `0600`. Checkpoint failures or crashes can leave external
deployment status uncertain; reconcile it before retrying or replanning. There is
no transaction or exactly-once guarantee across external deployments and local state.
`--no-commit` skips Git publication, not lock updates, and makes durable history
publication your responsibility. See [retry limits](concepts/plan-apply.md#retries).
