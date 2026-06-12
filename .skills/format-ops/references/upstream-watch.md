# upstream-watch — Okapi tracker, release tags, and spec feeds

## Purpose

Detect upstream movement — Okapi issues/tags, per-spec version drift, other
implementations — and turn it into dossier drift notes, un-xfail candidates,
a backport list, and (when the Okapi tag moved) a version-bump plan.

## Due when

- 30-day cadence elapsed; or any watermark family moved:
  - Okapi issues: any `updated_at` > `watermarks.okapi_last_issue_updated_at`;
  - Okapi release tag ≠ `watermarks.okapi_pinned` (currently `1.48.0`);
  - any dossier spec-source or implementation `watch` feed shows a new
    version vs `watermarks.per_spec` / `watermarks.per_implementation`.

## Inputs (watermark-source commands)

The Okapi tracker is **GitLab, not GitHub** (`gh` does not apply). Project
`okapiframework/Okapi` == id `62298414`; the local clone is pinned to
**1.48.0** (`/Users/asgeirf/src/okapi/Okapi`).

```bash
# Issues updated since the watermark (public, no auth):
curl -s "https://gitlab.com/api/v4/projects/62298414/issues?order_by=updated_at&sort=desc&state=all&per_page=50&updated_after=<okapi_last_issue_updated_at>"
# Issues mentioning one format:
curl -s "https://gitlab.com/api/v4/projects/62298414/issues?search=<id>&state=all&per_page=50"
# Open bugs, most-recently-updated first:
curl -s "https://gitlab.com/api/v4/projects/62298414/issues?labels=bug&state=opened&order_by=updated_at&per_page=50"
# Release tags:
curl -s "https://gitlab.com/api/v4/projects/62298414/repository/tags?per_page=5"
# Browse one issue:  https://gitlab.com/okapiframework/Okapi/-/issues/<iid>
# or:                glab issue list -R okapiframework/Okapi --search "<id>"
```

Cross-reference: issue numbers are embedded in the Okapi checkout's comments
and fixture names (`grep -rn "issue_" <okapi-clone>/okapi/filters/<format>`)
and in this repo's `parity-annotations.yaml`. Spec/implementation feeds come
from each `core/formats/<id>/dossier.yaml` `watch` entry (GitHub
`releases.atom`, W3C/OASIS/Unicode feeds, MS-* revision pages).

## Steps

1. **Okapi issues sweep**: fetch issues updated since the watermark; triage
   each to the affected format(s). Write per-format drift notes into
   `core/formats/<id>/dossier.yaml`. Collect:
   - **un-xfail candidates** — `okapi-bug` xfails now fixed upstream
     (relative to the pinned 1.48.0: "fixed upstream" lands when we bump);
   - **backport list** — new upstream `@Test` cases worth porting (harvest
     with `scripts/okapi-test-scan`).
2. **Tags**: if the latest tag ≠ pinned, write an **Okapi version-bump plan**
   enumerating every pin location: `Makefile`, the parity sandbox, fixtures
   regen (`make regen-okapi-fixtures`), contract-audit, the okapi-bridge
   build matrix. Execution is its own approved follow-up — record the plan
   as a `followups[]` item + a GitHub issue, not in `pending[]` (the pending
   type enum has no slot for it; the tracker owns implementation plans).
3. **Per-spec watch**: for every dossier spec source with a `watch` feed,
   fetch the feed and compare the latest version against
   `watermarks.per_spec[<spec-id>]`. New version → a dated drift note in the
   dossier (what changed, which clauses, whether `specs/` snapshots/sections
   need a new pinned version) + a `case-gen`/`remediate` followup when
   clauses we cite moved. **Unchanged feeds get no note and no edit.**
4. **Per-implementation watch** (`watermarks.per_implementation`): Pandoc /
   Tika / LibreOffice / translate-toolkit release feeds from the dossiers.
   GPL sources are *read-about*, never harvested.
5. **Calendar windows**: annually in the **Oct–Nov Unicode/CLDR/ICU release
   train**, do the deep pass (Unicode, CLDR, ICU versions across affected
   formats). Quarterly, sweep the slow movers: **MS-\* revision pages
   (MS-DOC/MS-XLS/…), OASIS (ODF), DTCG (design tokens), W3C timed-text
   (TTML/WebVTT/DAPT)**.
6. Update `per_format_last_swept` for every format touched.

## Verification (→ `runs[].evidence`)

`{check: "gitlab issues+tags fetch", exit, output_sha: sha256(concatenated
API responses)}` — the raw responses are the evidence the watermarks were
computed from.

## Ledger updates

- `watermarks.okapi_last_issue_iid` / `.okapi_last_issue_updated_at` from the
  newest issue consumed; `.okapi_latest_tag` from the tags response;
  `.per_spec` / `.per_implementation` per feed consumed;
  `per_format_last_swept[<id>]` = today for swept formats.
- `last_run`, `runs[]` entry; dossier edits + ledger in the same commit.

## Outputs

Dossier drift notes, un-xfail candidate list (→ `xfail-hygiene` followup),
backport list (→ `remediate` carryover or followup issue), version-bump plan
when the tag moved.

## Failure → blocked

Offline / GitLab unreachable → outcome `blocked` ("needs-network"), report
from the local clone only and say so. Never guess a feed state — an
unverified feed keeps its old watermark.
