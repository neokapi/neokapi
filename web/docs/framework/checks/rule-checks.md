---
sidebar_position: 2
title: Rule-based checks
description: "Rule-based checks in neokapi: deterministic rules and LLM-assisted review that annotate translated blocks with findings without modifying content, composable as pipeline stages."
keywords: [rule-based checks, translation checks, deterministic rules, findings, LLM review, pipeline, multilingual content]
---

import RunnableSnippet from "@site/src/components/KapiPlayground/RunnableSnippet";
import { ThemedVideo } from "@neokapi/docs-shared";

# Rule-based checks

A rule-based check is a kind of [tool](/framework/tools): it reads translated
[blocks](/framework/content-model), inspects each one against a set of rules,
and **reports findings without modifying the content**. Findings are recorded
as stand-off overlays on the block (see
[Checks](/framework/checks#one-model-findings)) and surfaced to the CLI, an
editor, or a downstream tool, so a check pass slots into any
[flow](/framework/flows) as an ordinary stage. neokapi offers two complementary
approaches: fast, deterministic rules, and LLM-assisted review.

Run as a gate, these checks behave like tests for AI output: deterministic and
repeatable, they read translated content against its source and report exactly
what broke: a dropped placeholder, a translated do-not-translate term, a term
that drifts from the project's terms store. `kapi check` runs them over a file or a
source/target pair and exits non-zero when the gate fails, so a regression is
caught in CI, or in an assistant's fix-loop, the same way a failing test is.

<ThemedVideo
  sources={{
    light: "/video/kapi/kapi-checks-guardrail-light.webm",
    dark: "/video/kapi/kapi-checks-guardrail-dark.webm",
  }}
  maxWidth="820px"
/>

## Deterministic rules

By default, with no `--provider`, the `qa` tool runs a battery of
deterministic rules over each block, comparing source and target. It needs no
API key. It records each finding as a structured issue with a
type and a severity (error or warning) and marks whether the block passed. The
checks span several concerns:

| Concern             | Examples of what it catches                                                      |
| ------------------- | -------------------------------------------------------------------------------- |
| **Whitespace**      | Leading/trailing whitespace mismatch, double spaces                              |
| **Completeness**    | Empty target where the source has content, target identical to source           |
| **Inline codes**    | Missing or extra inline codes, code order, non-deletable code dropped, non-cloneable code duplicated |
| **Patterns**        | Source patterns (placeholders, numbers) without the expected target counterpart  |
| **Characters**      | Corruption patterns (for example, UTF-8 text decoded as ISO-8859-1)              |
| **Length**          | Target length outside an allowed ratio of the source, or over an absolute limit  |
| **Repetition**      | Consecutive doubled words in the target                                          |

Each check is individually configurable: every rule has a flag, and length
checks have thresholds. Because the schema is declared on the tool's config
struct, the available options and their defaults are generated into the
[Tool Reference](/tools) rather than listed by hand here.

Run it from the CLI against a bilingual file. The command below parses an XLIFF
file and reports its findings as JSON:

<RunnableSnippet
  cmd="kapi exec qa app.xliff --target-lang fr --json"
  seed={["app.xliff"]}
/>

In a flow, `qa` is just another step after translation:

```yaml
steps:
  - tool: translate
    config: { provider: anthropic }
  - tool: qa
    label: Quality checks
```

These rule families are all options of the one `qa` tool: length ratios and
absolute character/word limits (`--check-max-char-length`,
`--check-absolute-max-char-length`, `--check-max-words`), forbidden or
corrupted characters and charset conformance (`--forbidden-chars`,
`--required-chars`, `--check-charset`), regex patterns such as printf
placeholders (pattern rules in flow config), and the same source translated
differently across a batch (`--check-target-inconsistency`). Terminology has
its own validator, `term-check`. The full set is in the
[Tool Reference](/tools).

## LLM-assisted review

Where the deterministic rules catch the mechanical errors, running `qa` with an
LLM `--provider` uses that [LLM provider](/framework/translation) to assess
qualities a rule expresses poorly (fluency, accuracy against the source,
and terminology appropriateness) and attaches its assessment to each block. It
is the natural companion to `translate`: the built-in `translate-qa` flow runs
translation and then this review in one pass.

```bash
kapi run translate-qa -i app.xliff --target-lang fr
```

## Findings travel with the block

Both kinds of check emit the same `core/check.Finding`, recorded as a stand-off
overlay anchored to the offending runs, as described under
[Checks](/framework/checks#one-model-findings). This is the same shared channel that
[memory matches](/framework/content-memory),
[terminology](/framework/terminology), and [voice](/framework/checks/voice)
results use, so a single downstream consumer (a report, an editor view, a CI
gate) can read every kind of finding from one place.

## Related reading

- [Tools](/framework/tools): how a check fits the tool model.
- [Tool Reference](/tools): the generated list of check tools and their parameters.
- [Terminology](/framework/terminology): terminology enforcement as its own check family.
- [Implementing a Tool](/contribute/tools): writing a custom check.
