# Review instructions

Most changes in this repository are written and merged by an agent on green CI,
so a review is the one reading that is independent of the author. Spend it on
what CI cannot see. `CLAUDE.md` holds the repository rules; this file says where
defects have actually come from.

## Tag every finding

- **Important**: the change breaks behaviour, loses or leaks data, breaks a
  client that is already installed, or weakens a check. Give the file and line,
  the input or state that triggers it, and how you confirmed it.
- **Nit**: correct but worth improving. Keep these few.
- **Pre-existing**: the defect is on the base branch too. Report it, and say
  that the author did not introduce it. A pre-existing leak or data loss is
  still worth fixing in the same pull request.

Say whether each finding is confirmed (you ran it or traced it) or plausible. A
finding you could not confirm is reported as plausible, never as fact.

## What pull request CI does not run

A green pull request has not run any of these. When the diff touches them, the
author must show a local run, and you check that the run covers the change.

- **Suites that start their own containers.** Pull request CI passes
  `-short`. The PostgreSQL-backed suites still run, on the job's database
  service, but a suite that starts another container (the event bus, object
  storage) is skipped until the change reaches main.
- **The integration lane** (`//go:build integration`) and the Docker E2E
  stack. Both run nightly.
- **The race detector**, unless the pull request carries the `race` label. Ask
  for it when the diff adds a goroutine, a lock, a shared map or a channel.
- **Release workflows.** A change to a `release-*.yml` file, a package script
  or a pinned download is exercised only by a manual run of that workflow.

## Where defects come from here

Read these first when the diff touches them.

1. **Migrations against live data.** The server runs in production on
   PostgreSQL and migrates on deploy. For each new or changed migration: can it
   fail on a row that exists today (NULL, empty string, malformed JSON, a
   duplicate under a new unique index)? Does it lock a table that requests
   write to? Is it safe to run twice? Does the SQLite side stay in step? A test
   that rewinds the migration ledger must rewind every later version too.
2. **Visibility on every route of a surface.** When one route of a family
   filters by project visibility or workspace membership, every route of the
   family must: the list, the detail, the feed, the counts and any query
   parameter that names a project. An id the caller may not see answers
   exactly as an id that does not exist.
3. **Clients that ship separately.** The mobile app, the `kapi-bowrain` plugin
   and the desktop apps are installed at versions older than the server. A
   removed, renamed or retyped JSON field, route or status code breaks them.
   Responses may gain fields. A persisted client cache outlives an app update.
4. **Two clocks.** A comparison between a timestamp the server process wrote
   and one the database wrote is wrong under clock skew. Both sides of a
   comparison come from one clock.
5. **Unbounded reads.** A query with no `LIMIT`, or a loop that loads every
   block, term or activity of a project, has taken production down before.
   Check what bounds each new read.
6. **Run sequences.** Code that walks a `Run[]` and keeps only text deletes
   placeholders, paired codes and plurals without failing anything. A writer
   uses `model.RenderRunsWithData`; a TypeScript projection declares a
   `RunSpec`.
7. **Format writers.** An unedited round trip is byte-identical. An edit
   changes only the bytes of the edited value: line endings, indentation,
   comments and encodings survive.
8. **The licence line.** No Apache module or package reaches into `bowrain/`.
   A file moved across the line is relicensed.
9. **Per-tab and per-project state in the desktop.** An event, cancel or save
   applies to the tab that started it. Look for a closure over a tab id that
   can go stale, and a response applied after the project changed.

## Changes to the checks themselves

A change that edits a check which judges it gets an **Important** finding that
says so, whatever else you conclude, so a person looks at it. This covers:

- `scripts/check-*`, the guards under `scripts/*guard*/` and every allowlist
  or exemption list inside them;
- `scripts/govulncheck-accepted.txt`, where each new or extended entry is a
  security decision;
- `.github/workflows/`, `scripts/pre-push-check.sh` and the check targets in
  the Makefiles;
- this file, `CLAUDE.md` and `docs/internals/reliability-checklist.md`;
- a test that is deleted, skipped, given a sleep or a retry, or whose
  assertion is loosened.

State what the change lets through that was caught before. A new allowlist
entry needs a reason that would hold for a reader with no context.

## The description is a claim

Check each behaviour the description claims against the code. Report a claim
with no code behind it as Important: the next reader will trust it. The
"Verification" section lists what was run; a check that was skipped, or that
could not run, is named there and not left out.

## Do not report

- Formatting, import order and naming that `make check` already enforces.
- Vocabulary and prose rules. `kapi check` and the repository guards own them.
- Target-language catalogs falling behind a source change. That is expected.
- Missing tests for code the diff does not touch.
