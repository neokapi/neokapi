---
name: kapi-brand-context
description: Load a brand's voice guide (personality, tone, style rules, preferred/forbidden/competitor terminology, examples) into context BEFORE writing or translating content, so the output is on-brand from the first draft. Use at the start of any copywriting, content, docs, or UI-string task that must match a brand voice. Triggers on "write this on brand", "what's our voice/tone", "use our brand guidelines".
---

# kapi-brand-context

Prints a brand voice guide as markdown so you (the assistant) can apply it while generating content — getting it right at generation time instead of fixing it afterwards.

## When to use

Before producing user-facing text for a user who has a brand: marketing copy, landing pages, slide decks, README/docs, release notes, UI strings, support replies. Pull the guide first, then write to it. Pair with `kapi-brand-check` to verify the result.

## How to run

```bash
kapi brand guide --pack marketing-blog
kapi brand guide --profile-file ./brand.yaml
kapi brand guide --profile acme-marketing      # from the local store
```

List available profiles and packs:

```bash
kapi brand profiles --json
```

Built-in starter packs: `professional-b2b`, `friendly-dtc`, `technical-docs`, `marketing-blog`, `customer-support`.

## Output

Markdown covering Tone (personality, formality, emotion, humor), Style Rules (active voice, sentence length, POV, contractions, prohibited patterns), Vocabulary (preferred / forbidden / competitor terms with replacements), and before/after Examples.

Add `--json` to get `{ "profile": "...", "guide": "..." }` instead of raw markdown.

## How to apply

Read the guide, then keep these rules in mind as you write: match the tone and style, prefer the preferred terms, never use the forbidden or competitor terms (use the listed replacements), and follow the examples' before→after pattern. After drafting, run `kapi-brand-check` to confirm.
