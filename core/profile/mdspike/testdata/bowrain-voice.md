---
extends: house.md
name: bowrain platform
tone:
  personality: [direct, concrete, assured]
  formality: neutral
  emotion: neutral
  humor: none
style:
  prohibited_patterns:
    - regex: '\b(enterprise-grade|best-in-class|world-class|trusted by)\b'
      description: Credibility-by-assertion; show the mechanism instead
      severity: major
vocabulary:
  preferred_terms:
    - term: Bowrain
      note: The platform — capitalized as a product name
    - term: workspace
      note: The tenant boundary a team works inside; not "account" or "org"
    - term: context graph
      note: What Bowrain holds — the relationships that govern content, not a content store
    - term: content memory
      note: The store of source and target pairs
    - term: terms store
      note: The store of approved terminology
  forbidden_terms:
    - term: solution
      replacement: ""
      note: Says nothing; name the thing the product actually does
      severity: minor
locales:
  nb:
    formality: neutral
    person_pov: second
    vocabulary_overrides:
      - term: arbeidsområde
        note: workspace — the tenant boundary, not "arbeidsplass"
      - term: innholdsminne
        note: content memory — matches the terms store entry
---

# Bowrain platform voice

Register for Bowrain's user-facing text: landing, app UI, docs, and email.
Addressed to the person accountable for content across a team, not to a
developer reading a reference. Explain by contrast and state the outcome.

## Guidelines

Lead with what changes for the reader, then the mechanism that makes it true.
Explain by contrast when a distinction carries the point — a legal notice is
not a help article. Address the reader as someone answerable for content
quality, who has the problem already and does not need convincing it exists.
Claims stay checkable: name what the product does, never how it feels to use.
Sentences carry one idea.

## Example: tone

**Before:** Bowrain is a powerful enterprise-grade platform trusted by teams everywhere.

**After:** Bowrain is the context graph for your content — the one your people and your AI agents plug into.

**Why:** Name what it is and who uses it; credibility comes from the mechanism, not the adjective

## Example: style

**Before:** Easily manage all your brand rules in one place.

**After:** A legal notice is not a help article, so the rules that fix voice and tone move with the audience and the surface.

**Why:** Explain by contrast; the distinction is the point, not the convenience

## Example: vocabulary

**Before:** Our solution supports 15+ formats and unlimited languages.

**After:** Bowrain reads the formats kapi reads; see the formats reference.

**Why:** No hardcoded counts, no unbounded claims, no "solution"

## Locale: nb

Norwegian Bokmål. Address the reader as "du". Use established Norwegian
software terminology where the terms store names one, and keep the plain,
declarative register. Never translate product names (Bowrain, kapi, neokapi,
kapi-desktop), CLI commands, flags, file formats, code identifiers, or file
paths. Headings in sentence case. Compound nouns close up in Norwegian — never
leave an English noun pair half-translated.
