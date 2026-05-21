---
name: bowrain-terminology
description: Use a team's SHARED, governed termbase on a Bowrain server — search approved terms and propose new ones for review. Use when terminology must be consistent across a whole organization with an approval workflow, rather than a local glossary. Triggers on "shared termbase", "approved org terminology", "propose a term", "term review queue".
---

# bowrain-terminology

The governed counterpart to the local `kapi-terminology` skill. Terminology lives on a Bowrain server with a review queue (term candidates are proposed, then approved/rejected by reviewers), so the whole org draws on one approved glossary.

## When to use

A team needs consistent terminology across many projects and people, with governance (approval) over what becomes canonical. For local/offline glossaries, use `kapi-terminology`.

## Prerequisites

- `kapi-bowrain` plugin installed and authenticated (see `bowrain-project`).
- The Bowrain MCP server configured for your assistant.

## Tools (via the Bowrain MCP server)

- `term_search` — search the workspace termbase for an approved term and its translations.
- `term_add` — propose a new term; it enters the review queue rather than being applied immediately.

## How to apply

1. `term_search` before writing or translating, to use the approved org term.
2. When you encounter an important term that's missing, `term_add` to propose it for review.
3. Reviewers approve/reject in the Bowrain app; approved terms become available to everyone and feed translation/QA.
4. Pair with `bowrain-brand-governance` (brand vocabulary) and `bowrain-project` (sync) for the full governed loop.
