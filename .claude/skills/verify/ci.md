# Verify a change to CI, a release workflow or a check

A workflow change is verified by a run of that workflow. Reading the YAML is
not a run.

## Lint first

```bash
make gha-lint                      # actionlint over every workflow
./scripts/check-local-actions.sh   # a local `uses: ./…` resolves where the job checked out
shellcheck <each script you changed>
```

A guard or scan script has a self-test. Run it, and add a fixture for the case
you changed:

```bash
./scripts/check-ts-license-boundary.sh --self-test
python3 scripts/govulncheck.py --self-test
```

## Prove a check can fail

A check that cannot fail verifies nothing. After changing a guard, break the
thing it guards and watch it fail, then restore it:

- For a script with a policy table or an allowlist, run a copy with one entry
  tightened or removed and confirm it reports the violation and exits non-zero.
- For a parser of a tool's output, feed it output it does not recognise and
  confirm it fails. An unrecognised input must never read as "nothing found".

## Run the workflow on the branch

Push the branch, then start the workflow against it. Every scheduled workflow
and the plugin release workflows accept a manual run:

```bash
gh workflow run nightly.yml --ref <branch>
gh workflow run release-sat.yml --ref <branch>      # builds every platform, publishes nothing
gh workflow run release-vision.yml --ref <branch>
gh run list --workflow <file> --branch <branch> --limit 1
gh run watch <run-id> --exit-status
```

The release workflows publish only on a tag, so a manual run is a build-only
dry run. `release-check.yml` has no manual trigger; its steps mirror
`release-sat.yml`, so change both and dry-run the sat workflow.

Read the job you changed, not the run's overall colour. A run can be green
because your job was skipped by a path filter or an `if:`.

```bash
gh run view <run-id> --json jobs --jq '.jobs[] | "\(.conclusion)\t\(.name)"'
gh run view --job <job-id> --log-failed
gh api repos/{owner}/{repo}/actions/jobs/<job-id> --jq '.steps[] | "\(.conclusion)\t\(.name)"'
```

## What to look for

- **A download with no checksum.** Every fetched binary, model or archive is
  checked against a recorded SHA-256, as `scripts/fetch-onnxruntime.sh` does.
- **A floating version.** `@latest`, `toolchain: stable` and an unpinned
  runner image drift without a commit. Pin what the job depends on, or the
  failure arrives on a day nobody changed anything.
- **A step that stops the ones after it.** One failing module must not hide
  the next. Collect failures and report them together.
- **A path filter narrower than what the job reads.** A job gated on paths
  runs for every path that can change its result, including the script and the
  Makefile that define it.
- **A scheduled job nobody watches.** A new scheduled workflow is added to
  `scheduled-failure-issue.yml`. A second job that repeats an existing check is
  removed instead of added.
- **Cost on every pull request.** Note the job's duration in the pull request.
  A job over three minutes on the pull request path needs a reason.
