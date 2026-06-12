# Format Demotions Ledger

This file records every support-tier demotion of a format — the durable
"why" behind each downgrade in `core/formats/support.yaml`. Demotion is a
normal, recorded outcome, never deletion ([format-maturity.md §1](./format-maturity.md)):
announce before the release, drop at most one tier at a time
(Supported → Maintained → Available → plugin/retired), and keep the corpus and
dossier intact. Entries are written by the `tier-review` ritual after
maintainer approval of a `pending[]` proposal in the ops ledger
([format-ops.md §3 #12, §4](./format-ops.md)); they are never re-litigated
from scratch.

## Entry format

```markdown
## <format-id>: <from-tier> → <to-tier> (YYYY-MM-DD)

- **Reason:** the gating-axis regression or product decision, with citations
  (test output, dashboard snapshot, issue link).
- **Evidence:** ledger run reference (`runs[]` date + commit) and the approved
  pending-item id.
- **Announced:** where/when users were told (release notes link).
- **Revisit:** condition or date for re-promotion, if any.
```

## Demotions

None recorded yet (ledger seeded 2026-06-12; all 49 formats entered the tier
registry via the bootstrap with `grandfathered: true`).
