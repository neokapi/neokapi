# format-ops benchmarks — the prompt-edit regression gate

Generic "improvements" to prompts are not monotonic, so every ritual prompt
edit is **benchmark-gated** (format-ops.md §5.3):

- Each ritual owns a directory here: `benchmarks/<ritual>/`.
- A benchmark is **(a)** a frozen input snapshot — ledger state + captured
  signals (dashboard JSON, canned `gh`/GitLab responses, dossier watch
  states) under `input/` — and **(b)** `assertions.md`: *named* assertions on
  the ritual's proposed plan/output given exactly those inputs.
- **No prompt edit without a benchmark.** The first edit to an unbenchmarked
  prompt begins by freezing current behavior as its benchmark, committed
  before the edit.
- An edit lands only after a **paired run**: execute the ritual's reference
  prompt against the frozen inputs with the **old** prompt, then the **new**
  prompt; the new run must pass every named assertion the old one passed,
  plus whatever the edit set out to fix. Both paired runs are recorded in
  the ledger's `runs[]` (with the assertion outcomes as evidence).
- **Scoring-prompt edits additionally require golden-set calibration**
  (process-health phase 1) before landing.

## How to run one

1. Read `benchmarks/<ritual>/input/` — treat it as the *only* reality: the
   files stand in for the live artifacts/API responses the reference prompt
   names (the fixture filenames say which). Do not touch the network or the
   real ledger.
2. Execute `references/<ritual>.md` against those inputs, producing the
   ritual's plan/output (not committing anything).
3. Grade each named assertion in `assertions.md` pass/fail, by id.
4. Record `{check: "benchmark:<ritual>", exit: 0|1, output_sha:
   sha256(assertion outcomes)}` in the paired-run `runs[]` entries.

## Conventions

- Inputs are deliberately small and synthetic where possible — they freeze
  *behavior*, not data volume.
- Assertions are named (`A1`, `A2`, …) and state the expected behavior in
  one sentence each, with the planted condition they probe.
- Benchmarks are versioned artifacts: editing a benchmark is itself a
  process-health change (same commit discipline as prompt edits).

## Seed benchmarks

- `triage-score/` — planted floor regression (must FLOOR-FORCE demote) +
  planted uncited promotion (must suppress).
- `upstream-watch/` — planted spec version drift (must flag) + unchanged
  spec (must not flag).
