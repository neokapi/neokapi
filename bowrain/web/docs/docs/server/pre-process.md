---
sidebar_position: 5
title: Pre-process
---

# Pre-process

The Pre-process surface runs file-wide source-prep before per-block translation
begins. It is a sibling of the [Translation Editor](/server/translation-editor),
scoped to the same file and reached from the surface switcher in the header.

Keeping these operations on their own surface keeps the Translate editor focused
on editing one block at a time, rather than mixing file-wide actions into the
per-block toolbar.

## Operations

### Pseudo-translate

Generate accented, length-expanded placeholder translations for the whole file.
Pseudo-translation surfaces truncation, layout, and encoding problems before any
real translation is done — see also the framework's pseudo-translate tool.

### Bulk TM leverage

Pre-fill targets from the [translation memory](/server/translation-memory) across
the whole file. Exact and high-confidence fuzzy matches land as drafts you can
then check in [Review](/server/review).

### AI bulk draft

Drafting every untranslated block with an AI provider requires a configured
provider. Configure one in project settings, then start the draft from the
Translate editor's AI actions.

Each operation reports how many blocks it filled. After pre-processing, switch to
the Translate editor to refine, or to Review to accept the results.
