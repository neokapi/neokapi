# upstream-watch benchmark — named assertions

Execute `references/upstream-watch.md` against `input/` as the only reality
(the canned JSON files ARE the API responses; make no network calls). Grade
each assertion pass/fail on the run's proposed drift notes, followups, and
watermark updates.

## A1 — spec-drift-flagged (planted: DTCG moved 2025-10 → 2026-05)

`designtokens/dtcg` shows `draft-2026-05` against watermark
`draft-2025-10`. The run MUST flag it: a dated drift note proposed for
`core/formats/designtokens/dossier.yaml` (what moved, that pinned
`specs/` snapshots/sections may need a new version) and
`watermarks.per_spec["designtokens/dtcg"]` advanced to `draft-2026-05`.
Missing the drift is a FAIL.

## A2 — unchanged-spec-not-flagged (ttml2)

`ttml/ttml2` equals its watermark (`ttml2-2e-2021`). The run MUST NOT
propose any drift note or dossier edit for ttml, and MUST NOT touch its
watermark value. A spurious finding here is a FAIL (false positives erode
the watch).

## A3 — no-bump-plan-when-tag-unchanged

Latest tag `v1.48.0` matches pinned `1.48.0`: the run MUST NOT produce an
Okapi version-bump plan, and MUST NOT propose changing any pin location.

## A4 — new-issue-triaged-and-watermark-advanced

Issue iid 1502 (`updated_at 2026-06-02` > watermark `2026-04-19`) MUST be
triaged to the `po` format (a drift/backport note referencing the obsolete-
entry comment loss), and the run's proposed watermarks MUST advance to
`okapi_last_issue_iid: 1502` /
`okapi_last_issue_updated_at: "2026-06-02T09:14:33.000Z"`. The already-
consumed issue 1495 MUST NOT generate a new note.
