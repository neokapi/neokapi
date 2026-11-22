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

## Add a repo guard

A repo guard is one file, `scripts/guards/<target>.mk`, and no other edit.
The file holds the target, its recipe (self-test first), `GUARD_LANES` and,
for a guard pre-push should gate, `GUARD_PATHS`. `scripts/lib/guards.mk`
shows the shape. `make lint`, `make pre-push`, `make help` and the Repo guards
job read the directory, and `make check-guard-registry` fails on a guard wired
anywhere else.

```bash
make check-<target>                      # one guard
./scripts/run-guards.sh ci               # every guard the Repo guards job runs
./scripts/run-guards.sh --list pre-push  # what pre-push considers, with its paths
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
dry run. Each one ends its build in a `verify-<plugin>-platforms` job that
fails when a platform `scripts/lib/release-assets.json` registers was not
built. Read that job: it is the dry run's answer to "would this release
register every platform".

`release.yml` and `release-bowrain.yml` have no build-only trigger. Their
`verify-release-assets` job compares the release with the same file, and only
a tag runs it. `python3 scripts/release-assets.py check-release <track> <tag>`
runs the same comparison locally against any published release; add
`--complete` once the signed Windows assets are uploaded.

Read the job you changed, not the run's overall colour. A run can be green
because your job was skipped by a path filter or an `if:`.

```bash
gh run view <run-id> --json jobs --jq '.jobs[] | "\(.conclusion)\t\(.name)"'
gh run view --job <job-id> --log-failed
gh api repos/{owner}/{repo}/actions/jobs/<job-id> --jq '.steps[] | "\(.conclusion)\t\(.name)"'
```

## Merging: one required check

The main branch requires one status check, `CI passed`, the last job of
`ci.yml`. It fails unless every job of that workflow passed or was skipped,
and on a pull request it also waits for every other workflow on the same
commit (E2E, the docs builds, format acceptance and the rest) and fails when
one of them does. Queue the merge and carry on with other work:

```bash
gh pr merge <n> --auto --squash
gh pr view <n> --json state,mergeStateStatus,autoMergeRequest   # MERGED, or what holds it
```

- **A new job in `ci.yml` goes in the `needs` of `ci-passed`.** `make
  check-ci-passed` fails until it is there.
- **After you rerun a failed workflow, rerun `CI passed` too.** It failed when
  the other run did and does not notice the rerun: `gh run rerun <ci-run-id>
  --failed`.
- **A commit a workflow pushes with the workflow token starts no CI**, so the
  pull request never reports `CI passed` and stays queued. This covers the
  catalog commit a workflow adds to your pull request and the pull requests a
  workflow opens itself. Push a commit of your own to start CI. A pull request a bot opened needs the
  person, who can merge with `--admin`.
- **Never pass `--admin` yourself.** It merges over a red or missing check.

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

## The Actions cache budget

The repository may keep 10 GiB of Actions caches. `scripts/ci-cache-budget.py`
deletes superseded copies after each main run and fails the daily Cache Budget
run past 9 GiB; `scripts/check-ci-caches.py` fails a pull request step that can
save a cache. Before adding a cache family, measure it and add it to this
table. A main-only job that saves a large family saves it with
`actions/cache/save` and prunes in the next step, so the old and new copies
overlap for seconds.

Measured 2026-11-22, after the first prune: 5.91 GiB in 91 caches. The pull
request build cache (`go-build-…-pr-trimpath-<sha>`) is 2.09 GiB, vite-plus
0.94 GiB, BuildKit blobs 0.93 GiB, go-mod 0.78 GiB, the race build cache
0.56 GiB. The lint-leg flavors of #3729 add about 1.05 GiB.

**Not built: one build cache per job group.** Every Go job restores the whole
2.09 GiB pr cache. Separate caches per group would restore less, at 15 to 20
seconds a job. Each group's cache, measured from an empty GOCACHE with
`go test -tags fts5 -count=1 -run '^$' ./...` in its module (macOS, before
compression): framework 873 MiB, cli 730 MiB, kapi 807 MiB, bowrain 1,536 MiB,
bowrain/plugin 753 MiB. Each holds the framework again, so the split adds
about 1.5 GiB compressed. With the lint flavors that leaves too little of the
9 GiB budget, so the single pr cache stays.

## Traps that pass on a rerun

- **A test that moves `XDG_DATA_HOME` and runs a Node tool.** On CI, and on
  any machine set up with setup-vp, `node` is the vite-plus shim, which keeps
  its Node.js runtimes under `XDG_DATA_HOME`. Point that at a throwaway
  directory and the shim downloads a whole Node.js release on every call,
  which is how oxfmt ran into its one-minute limit. `host/main_test.go`
  (`pinNode`) puts the real binary on PATH first.
- **A plugin daemon that installs its signal handler after the handshake.**
  A host may send SIGTERM as soon as it reads the handshake line, and a
  signal that arrives before `signal.Notify` kills the daemon before it
  removes its socket. Call `signal.Notify` first, as
  `host/pluginhost/testdata/fakedaemon/main.go` does (#3713).
