# Anchored evidence development comparison

This six-session comparison tests evidence selection on three existing
candidates. Selection follows observed development failures, so these are
regression cases rather than held-out evidence:

- `museum-transfer-explicit`: supported action guidance, retained as a control
  for literal-quote failures.
- `editorial-correction-omitted`: a missing reader action, retained to check
  omission detection and unsupported duplicate conflict claims.
- `destination-guidance-faulty`: factual conflicts and a missing saved-file
  check, retained to check that genuine contradictions remain detectable.

Each candidate receives the existing `requirements` review protocol and the
`anchored` protocol with the same Sonnet 5 high configuration. Candidate text,
sources, variables, task requirements and shared instructions are identical
within each pair. The anchored protocol derives paragraph selection IDs and
requests an explicit conflicting claim. The preparer does not add answers or
alter the supplied content. Protocol order alternates between cases. The
existing runner's subscription-only execution, no-tool restriction and
persistent six-attempt ceiling apply.

Prepare the comparison without calling a model:

```sh
python3 scripts/checkeval/prepare_evidence.py --out harness/out/evidence-comparison
```

The preparer calls the existing action and document preparers, including their
source and schema checks. Repository documentation must still match its frozen
Git blob and excerpt. The museum and editorial source policies retain their
synthetic provenance. Selected inputs and labels are copied from the existing
preparations; their contents remain unchanged.

Open `harness/out/evidence-comparison/review.html` to read the context, sources,
candidates and provisional author labels. The `subject` directory contains
only the three scheduled inputs, shared instruction and runner manifest. The
labels, selection rationale and full provenance remain outside it. Provenance
records the corpus preparations and hashes of the preparation scripts. Existing
outputs cannot be overwritten. This casebook reports no new model judgments.

Run preparation checks offline:

```sh
python3 -m unittest discover -s scripts/checkeval -p 'test_prepare_evidence.py'
```

The checks verify unchanged selected evidence and labels, matched scheduling,
reproducible artifacts and continued rejection of changed repository sources.
They do not establish semantic accuracy, review benefit or the correctness of
an authored label. Keep response integrity and externally assessed factual
support separate when interpreting the eventual comparison.
