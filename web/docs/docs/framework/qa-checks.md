---
sidebar_position: 13
title: QA Checks
description: Quality assurance in neokapi — deterministic rule-based checks and LLM-assisted review that annotate translated blocks with findings without modifying content, composable as pipeline stages.
keywords: [QA checks, quality assurance, translation QA, rule-based, LLM review, pipeline, localization]
---

import RunnableSnippet from "@site/src/components/KapiPlayground/RunnableSnippet";

# Quality Assurance Checks

Quality assurance in neokapi is a kind of [tool](/framework/tools): a QA check
reads translated [blocks](/framework/content-model), inspects each one against a
set of rules, and **reports findings without modifying the content**. Findings
are attached to the block — recorded in its properties and surfaced to the CLI,
an editor, or a downstream tool — so a QA pass slots into any
[flow](/framework/flows) as an ordinary stage. neokapi offers two complementary
approaches: fast, deterministic rule-based checks, and LLM-assisted review.

## Rule-based checks

The `qa-check` tool runs a battery of deterministic rules over each block,
comparing source and target. It records each finding as a structured issue with a
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

Each check is individually configurable — every rule has a flag, and length
checks have thresholds. Because the schema is declared on the tool's config
struct, the available options and their defaults are generated into the
[Tool Reference](/tools) rather than listed by hand here.

Run it from the CLI against a bilingual file. The command below parses an XLIFF
file and reports its findings as JSON:

<RunnableSnippet
  cmd="kapi qa-check app.xliff --target-lang fr --json"
  seed={["app.xliff"]}
/>

In a flow, `qa-check` is just another step after translation:

```yaml
steps:
  - tool: ai-translate
    config: { provider: anthropic }
  - tool: qa-check
    label: Quality checks
```

Related validation tools cover narrower jobs — `length-check` for length ratios,
`chars-check` for forbidden or corrupted characters, `pattern-check` for regex
patterns such as printf placeholders, `inconsistency-check` for the same source
translated differently across a batch, and the terminology validators
`term-enforce` and `term-check`. The full set is in the
[Tool Reference](/tools).

## LLM-assisted review

Where rule-based checks catch the mechanical errors, the `ai-qa` tool uses an
[LLM provider](/framework/ai-translation) to assess qualities a rule cannot
easily express — fluency, accuracy against the source, and terminology
appropriateness — and attaches its assessment to each block. It is the natural
companion to `ai-translate`: the built-in `ai-translate-qa` flow runs translation
and then this review in one pass.

```bash
kapi run ai-translate-qa -i app.xliff --target-lang fr
```

## Findings travel with the block

Both kinds of check use the [Block annotation system](/framework/content-model)
rather than rewriting text. Rule-based findings are recorded in block properties;
LLM findings are attached as an annotation. This is the same shared channel that
[TM matches](/framework/translation-memory),
[terminology](/framework/terminology), and [brand-voice](/framework/brand-voice)
results use, so a single downstream consumer — a report, an editor view, a CI
gate — can read every kind of finding from one place.

## Related reading

- [Tools](/framework/tools) — how a check fits the tool model.
- [Tool Reference](/tools) — the generated list of QA tools and their parameters.
- [Terminology](/framework/terminology) — terminology enforcement as a QA concern.
- [Implementing a Tool](/contribute/tools) — writing a custom check.
