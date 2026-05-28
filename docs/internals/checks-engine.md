# Content checks: the verification engine

This note describes how neokapi verifies content — the mechanism behind
"checks that act like tests for AI output." It is a framework concern; it makes
no reference to any platform built on top of it.

## What a check is

A **check** is a rule that reads a block and emits findings. A **checkset** (or
profile) is a named bundle of checks. Running a checkset over content is the
content analogue of running a test suite over code: the checks are deterministic
and repeatable even though the generation that produced the content was not. The
checks do not make generation deterministic; they make it accountable.

## One finding model

Every checker — a deterministic rule, a small ML model, or an LLM judge — emits
the same record, so one scoring, annotation, and review path serves terminology,
do-not-translate, placeholder integrity, register, and brand voice alike.

```go
// core/check
type Finding struct {
    Category     string            // "terminology", "do-not-translate", "placeholder", "register", a brand dimension …
    Severity     Severity          // neutral | minor | major | critical
    Message      string
    Suggestion   string            // optional remediation hint
    Position     model.RunRange    // run-anchored span in the source
    OriginalText string
    Metadata     map[string]string // checker-specific detail (model, confidence, rule id)
}
```

Severity carries MQM-inspired penalty weights (neutral 0, minor 1, major 5,
critical 25). Findings are the substantive output; the roll-up score is a
convenience and is honest only when calibrated (see *Scoring*, below).

A checker is any type that implements `check.Checker` and writes its findings
through `check.Annotate`, which attaches a single `FindingsAnnotation`
(`quality.findings`) to the block. Checkers are read-only **Annotate** tools
under the capability model (AD-006): they observe source and target and write
overlays/annotations only — never content.

## The deterministic kernel

The kernel is the set of checks a localization owner keeps enabled because they
are objective and high-confidence:

- **Terminology** — preferred/forbidden term usage, matched on whole words with
  Unicode word boundaries (`check.FindTerm`), so "use" never matches inside
  "user". This replaces substring matching, which is the usual source of false
  positives in lexical checks.
- **Do-not-translate** — product names, trademarks, and code identifiers that
  must survive verbatim into the target; a translated do-not-translate term is
  critical.
- **Placeholder and tag integrity** — every interpolation placeholder and
  numbered tag in the source must survive, by count, into the target
  (`{name}`, `{{name}}`, `${name}`, `%s`/`%d`/`%1$s`/`%@`, `%(name)s`,
  `<0>…</0>`). A dropped placeholder breaks a localized build at runtime.
- **Register** — formality requirements per locale (for example, formal forms in
  de/ja). A lexical layer covers the cheap cases; a small model covers the rest.

The value of the kernel compounds with volume, number of languages, and the
non-determinism of the producer: one person writing one language rarely needs
it; tens of thousands of strings translated by a machine into many languages do.

## Small models as checkers

Subjective-but-checkable dimensions (register, "does this read like the
reference examples?") are served by small, open, multilingual models run
**in-process** through the same plugin pattern the segmenter uses: an ONNX model
behind a build tag, driven over a line-delimited JSON protocol by a pure-Go
host, so the native runtime never enters the main binary. Such a model is a
read-only checker: it inspects output and emits findings; it does not generate,
and it does not compete with the generator. Quality tiers by language on a
multilingual backbone; uniform quality is not claimed.

## Scoring

`check.CalculateScore` aggregates findings into a 0–100 roll-up with a
per-category breakdown. By default it is `100 − Σ penalty`; passing
`WithWordCount` length-normalizes it (a single nit in a long paragraph should
not score like the same nit in a one-word string). A score is reported as
"trusted" only against a labeled evaluation set; absent that, the findings — not
the number — are the result a reader should act on.

## Running checks

Checks run wherever the pipeline runs: as tools inside a flow (`kapi run`), as a
quality gate in CI (a non-zero exit blocks the build), and exposed to an AI
assistant over MCP. The loop is the same one developers know from tests: produce
content, run the checks, read the findings, fix, re-run.
