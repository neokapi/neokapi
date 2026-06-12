# corpus-sweep — execute the wild corpus (read→write→read)

> **BLOCKED ON [#848](https://github.com/neokapi/neokapi/issues/848) — the
> corpus-sweep harness.** Until that issue closes: **do nothing.** Report
> `outcome: "blocked"` with the issue URL as evidence and move on. Do not
> hand-roll a partial sweep; the per-file subprocess isolation and resource
> caps are the safety property, not an optimization. The ritual unblocks
> itself when the issue lands (`due.mjs` re-checks
> `gh issue view 848 --json state`).

## Purpose (once unblocked)

Wild files that are never executed validate nothing. Run every Tier B corpus
through read→write→read and classify each file; diff the counts against the
previous sweep; promote crashers into the fuzz/bug flywheel. This is the
green-sweep record the C3 floor consumes.

## Due when

- 60-day cadence elapsed (stays due while blocked); or
- any `watermarks.per_format_counts` drift left unexplained by the previous
  run.

## Inputs

- The sweep harness from #848 (one file = one subprocess, wall-clock + RSS
  caps).
- Fetched Tier B corpora: `make fetch-corpus [FORMAT=<id>]` →
  `corpus/<version>/<id>/`.
- Previous counts: `corpus-sweep.watermarks.per_format_counts`.

## Steps

1. Fetch the corpora for every format with Tier B entries.
2. Run the harness per format: each file read→write→read in its own
   subprocess with wall-clock/RSS caps; classify each file exactly one of
   `OK / OK_ROUNDTRIP / EXPECTED_REJECT / CRASH / HANG / OOM /
   ROUNDTRIP_DRIFT`.
3. **Diff the per-format counts against the previous sweep.** An unexplained
   delta (e.g. OK_ROUNDTRIP dropped, CRASH appeared) is a **due-fail**: the
   ritual's outcome is a finding, not a pass — attribute the delta to a
   commit (`git diff <last runs[].commit>..HEAD -- core/formats/<id>`) or
   record it as an open regression followup.
4. For every `CRASH / HANG / OOM / ROUNDTRIP_DRIFT` file: minimize it, then
   auto-promote — the minimized file lands in
   `core/formats/<id>/testdata/fuzz/` **and** gets a `corpus.yaml` entry with
   `origin: bug` (the bug→corpus flywheel is mandatory).
5. File an issue per crasher class; reference it from the corpus entry.

## Verification (→ `runs[].evidence`)

`{check: "corpus-sweep <format-set>", exit, output_sha:
sha256(per-file classification report)}` — the report is the artifact the C3
floor reads.

## Ledger updates

`last_run`; `watermarks.per_format_counts = {<format>: {OK: n,
OK_ROUNDTRIP: n, EXPECTED_REJECT: n, CRASH: n, HANG: n, OOM: n,
ROUNDTRIP_DRIFT: n}}`; `runs[]` entry (same commit as promoted fixtures +
manifest entries). Remove `blocked_on` only when #848 is closed and the
harness exists on HEAD.

## Outputs

Classification report, count diffs, minimized fixtures promoted to
`testdata/fuzz/` + `origin: bug` manifest entries, regression followups.

## Failure → blocked

Harness absent (today's state) or corpora unfetchable → `outcome: "blocked"`
with the issue URL / fetch command as evidence. Partial sweeps are recorded
as partial — never extrapolate counts.
