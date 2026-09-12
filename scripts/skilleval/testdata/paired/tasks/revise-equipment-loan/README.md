# Equipment policy case

Alder Library is a fictional equipment-lending service. The approved camera
loan extension affects a borrowing page, a reminder template and a desk card.
The reminder dispatch day depends on the new loan period. Telescope loans are
an approved exception; historical copy and external identifiers are protected.

The task uses fixed display strings and scheduling values so completion can be
scored without asking a person to rank near-identical rewrites. Existing
`json_equals` criteria check every editable field. The evaluator also compares
protected files with the originals and checks document structure. Reference
repairs, fault fixtures, criteria and this note stay outside agent workspaces.

| Offline repair | Independent acceptance | Voice pattern checks |
| --- | --- | --- |
| Correct repair | Pass | Updated wording passes |
| Desk card omitted | Fails the card's label and duration | Obsolete label produces a finding |
| Reminder dispatch stays at day 5 | Fails the schedule calculation | No finding; a numeric dependency is outside pattern coverage |
| Telescope extended to 14 days | Fails the protected-file comparison | Telescope-specific rule produces a finding |
| Stable camera identifier renamed | Fails identifier preservation | No finding; identifier preservation is checked separately |

Fault files under `testdata/paired/faults/revise-equipment-loan` overlay the
correct repair. Tests assert the exact failed acceptance criteria. Separate
checker tests resolve each file's recipe channel and exercise the real profile
checker, including the telescope exception and its asserted provenance.

Use `testdata/paired-equipment-study.json` for the next six-condition run. It
selects this task only, with one repetition per host and integration. The
original manifest and its smoke task are unchanged. Input hashes require a
fresh study directory when changing the corpus.

Compare missed updates, over-broad updates, context and check scope, repair
attempts and elapsed time. A zero-finding check can coexist with a wrong reminder
schedule. Passing this case establishes the authored acceptance conditions;
it does not establish general semantic checking, better prose, or lower review
effort. The evaluator retains its human-review status for those broader claims;
there is no prose preference ballot for this case.
