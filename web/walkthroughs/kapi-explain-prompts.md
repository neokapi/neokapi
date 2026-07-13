---
id: kapi-explain-prompts
audience: developer
target_doc: docs/framework/prompts.mdx
scenes:
  - id: explain-prompts
    kind: terminal
    binary: kapi
    duration_budget_seconds: 45
    fixtures:
      - messages.json
    smoke_contract:
      - kapi translate messages.json --source-lang en --target-lang fr --provider demo --explain-prompts
      - kapi translate messages.json --source-lang en --target-lang fr --provider demo --instruction "Informal register. Keep product names in English." --explain-prompts
      - kapi translate notice.html --source-lang en --target-lang fr --provider demo --explain-prompts
---

## Story

"What is kapi actually sending to Claude?" is a fair question, and until you can
answer it, every translation is an act of trust. This walkthrough answers it by
running the thing: `--explain-prompts` prints the exact text kapi sent, the model
it went to, and the reply that came back.

The scene builds a prompt up one layer at a time so the composition is watched
rather than asserted. A bare run shows the two sections the framework always
sends — the task, and the placeholder constraint — plus a user turn holding the
source text and nothing else. Adding `--instruction` makes a third section
appear, attributed to the flag that produced it. Translating a block that carries
inline markup makes a fourth appear: the tag-fidelity rule, which a plain string
never sees, because constraints are earned rather than constant.

Every step runs against `--provider demo`, the deterministic offline stub. That
is not a compromise — it is the feature. The prompt rendered is the prompt a paid
provider would receive, so you can inspect exactly what kapi would send before
spending anything, without an API key and without the content leaving the
machine.

The two sections the scene does not show — the glossary and the brand voice —
are project bindings rather than flags: they come from your termbase and your
voice profile, declared in the recipe. The [Prompts](/framework/prompts) page
shows them in place.

## Scene 1 — explain-prompts (terminal)

Translate a fixture bare and read the prompt; add an `--instruction` and watch
the steering section appear; translate a tag-bearing HTML block and watch the
inline-tag constraint appear. Three runs, no keys, no network.
