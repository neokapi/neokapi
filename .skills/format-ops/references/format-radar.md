# format-radar — emergence scan + adoption proposals

## Purpose

Scan the wider content world for format candidates, score them against the
adoption-evidence bar, and write accept/reject **proposals** into the pending
queue — never decisions. Output artifact:
`docs/internals/format-radar.yaml` (the ranked shortlist + decided log).

## Due when

90-day cadence only (no watermark term).

## Inputs

- Current radar: `docs/internals/format-radar.yaml` (do not re-litigate
  `rejected` entries before their `revisit_after`).
- Ledger `format-radar.decided` (accepted/rejected memory).
- The funnel + evidence bar: `docs/internals/format-ops.md` §6.

## Scan sources (the standing list — span the wider content world, not just text/code)

- GitHub Octoverse growth data
- Standards-body announcements: W3C (including timed-text/DAPT), OASIS,
  Unicode, Khronos glTF, AOUSD/USD
- TMS supported-format pages (the demand signal)
- CMS rich-text schemas
- Subtitle/caption and dubbing-exchange ecosystems
- Game-engine l10n table formats
- Design-tool format pages
- llms.txt / AGENTS.md adoption trackers

## Steps

1. Sweep the sources (network; offline → blocked). Collect candidate
   formats with raw evidence links.
2. Score each candidate against the **adoption-evidence bar** — all four
   required before an accept proposal:
   - real demand signal (TMS format pages, ecosystem growth data, user
     requests);
   - a harvestable or generatable corpus;
   - an identifiable spec source for the dossier;
   - a statement of what kapi uniquely adds (faithfulness / vocabulary /
     editor angle).
3. Classify three-valued: some candidates are **connectors** (Figma REST,
   cmi5/SCORM), some are **watch-only** (USD/glTF today). Prefer configuring
   existing readers (JSON/YAML) over new bespoke formats where faithfulness
   allows; bespoke is justified when the format has inline semantics (the
   Portable Text rule).
4. Update `docs/internals/format-radar.yaml`: refreshed emergence scan,
   scored candidates, and for each decision-ready candidate an
   accept/reject **proposal** appended to the ledger's `pending[]`:
   `{id, ritual: "format-radar", type: "radar-decision",
   proposal: {format, as: "format|connector|watch"} or
   {format, reject: true, revisit_after}, evidence, created}`.
5. **Engineering build-out work is not radar material** (runners, harnesses,
   safeio, migrations) — those are GitHub issues. Retirement proposals run
   the same funnel backwards via `tier-review`, not here.

## Verification (→ `runs[].evidence`)

`{check: "radar yaml validates + pending appended", exit,
output_sha: sha256(format-radar.yaml)}`.

## Ledger updates

`last_run`; `decided.accepted[]` / `decided.rejected{<format>:
revisit_after}` only **after** the maintainer approves the pending item (the
apply step records it); `runs[]` entry — same commit as the radar YAML.

## Outputs

Updated radar shortlist + pending radar-decision proposals. Approved accepts
flow to the `implement-format` skill (ladder: L1 floor → L2 specified), then
`triage-score` picks the format up, then `tier-review` promotes.

## Failure → blocked

Offline → `outcome: "blocked"` (`needs-network`); never fabricate demand
evidence from memory — every candidate row carries a fetchable link.
