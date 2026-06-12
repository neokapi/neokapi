# xfail-hygiene — keep the divergence ledger honest

## Purpose

Remove stale xfails, raise `divergence_kind` coverage toward 100%, and
reconcile `parity-annotations.yaml` against reality. The "assertions now pass
— remove the tag" runner log is the only safety net catching stale xfails;
this ritual is where it gets read.

## Due when

- 60-day cadence elapsed; or
- any tracked issue in `expected_fail`/annotations changed state
  (`watermarks.tracked_issues` vs `gh issue view`).

## Inputs (watermark-source commands)

```bash
# Enumerate xfails + annotations:
grep -rn "expected_fail" core/formats/*/spec.yaml | wc -l
ls core/formats/*/parity-annotations.yaml
# Tracked-issue states (GitHub, this repo):
gh issue view <ids> --json state,updatedAt
# Live runner truth (requires sandbox):
make parity-sandbox && make parity-test   # capture the full log
```

Known tracked-open natives: pdf #617, xliff2 #560, archive #504, openxml
RunFonts — never gloss a FAIL as "pre-existing"; investigate.

## Steps

1. **Census**: enumerate every `expected_fail` across `core/formats/*/spec.yaml`
   and every `parity-annotations.yaml` entry. For each, assert:
   - `divergence_kind` is set (raise coverage toward 100% — add the missing
     ones, grounded in spec + Okapi citations);
   - it is **not** `native-bug` (if it is → that's a bug to fix via
     `remediate`, not to document);
   - it is **not** a pure `default-diff` (→ converge with explicit config in
     `bridge_config`, don't xfail);
   - it cites the format spec **and** the Okapi class/method.
2. **Stale xfails**: run the parity suite and grep the log for xfails whose
   assertions now pass — remove those tags. Treat
   `parity-annotations.yaml` reason text as suspect until re-verified against
   a live `PARITY_DUMP` (it has been stale/inverted before).
3. **Issue reconciliation**: `gh issue view` every issue id referenced by an
   xfail/annotation. Closed issue + still-xfailed → either the fix never
   landed here (followup) or the xfail is stale (remove + verify). Update
   `watermarks.tracked_issues` = `{<id>: {state, updatedAt}}` for every issue
   consumed.
4. Re-run the affected format tests (`go test ./core/formats/<id>/...` and
   the tagged parity test for touched formats) before committing removals.

## Verification (→ `runs[].evidence`)

- The recorded checker output: `{check: "parity-test stale-xfail grep",
  exit, output_sha: sha256(parity log)}` — K3 gates on this recorded output,
  not on a bare watermark.
- `gh issue view … --json state,updatedAt` output sha.

## Ledger updates

`last_run`, `watermarks.tracked_issues` rewritten from the consumed states,
`runs[]` entry; spec/annotation edits + ledger in the same commit.

## Outputs

Removed stale xfails, added `divergence_kind`s, reconciled annotations,
followups for closed-but-unfixed issues.

## Failure → blocked

No sandbox / parity suite cannot run → outcome `blocked` (the census and
issue reconciliation can still land; say exactly which half ran). Offline →
issue reconciliation is `needs-network`; do the census half only.
