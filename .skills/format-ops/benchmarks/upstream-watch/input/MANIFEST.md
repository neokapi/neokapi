# upstream-watch benchmark — frozen inputs

Canned network responses + watch state. These files stand in for the live
signals named by `references/upstream-watch.md`; treat them as the only
reality — no network calls.

| Fixture | Stands in for |
|---|---|
| `gitlab-issues.json` | `curl …/projects/62298414/issues?order_by=updated_at…` (canned response) |
| `gitlab-tags.json` | `curl …/projects/62298414/repository/tags…` (canned response) |
| `dossier-watch-state.json` | the dossier `watch` feeds' current versions (per spec source) + the ledger's `upstream-watch.watermarks` snapshot |

Planted conditions:

1. **designtokens / DTCG — version drift.** The watch feed shows draft
   `2026-05` while the watermark recorded `2025-10`: MUST be flagged as
   spec drift (dossier drift note proposed).
2. **ttml / W3C TTML2 — unchanged.** Feed version equals the watermark
   (`ttml2-2e-2021`): MUST NOT be flagged, no dossier edit proposed.
3. **Okapi tag unchanged.** Latest tag `v1.48.0` == pinned `1.48.0`: no
   version-bump plan.
4. **One new Okapi issue** (iid 1502, po-related, `updated_at` newer than
   the watermark): must be triaged into the sweep notes and advance the
   issue watermark.
