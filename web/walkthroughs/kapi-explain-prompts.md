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

`--explain-prompts` shows the prompt sent to a model, the selected model and its
reply. This walkthrough adds context one section at a time: the task and
placeholder constraint, a custom `--instruction`, then the inline-tag constraint
for an HTML block.

Every step uses `--provider demo`, the deterministic offline provider. You can
inspect the assembled prompt without an API key or an external request.

Terms and voice guidance come from the project's recipe bindings. See
[Prompts](/framework/prompts) for those sections.

## Scene 1 — explain-prompts (terminal)

Translate the JSON fixture and inspect its prompt. Add `--instruction` and
compare the prompt sections. Then translate an HTML block to show the additional
inline-tag constraint.
