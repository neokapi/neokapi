# Anthropic's published guidance on self-improving, AI-driven engineering processes

Researched 2026-06-11. Scope: Anthropic engineering posts, Claude Code docs, Agent Skills docs/repo, Console prompt tooling, cookbook patterns — distilled for a solo maintainer running a recurring, self-improving format-maintenance process (neokapi: 49 formats, L0–L4 maturity rubric, Okapi parity harness, executable spec.yaml files).

## Findings

### 1. Building Effective Agents: simplicity-first, workflows before agents (Dec 2024)

Source: https://www.anthropic.com/engineering/building-effective-agents (published 2024-12-19).

- Core distinction: **workflows** = "LLMs and tools orchestrated through predefined code paths"; **agents** = "LLMs dynamically direct their own processes and tool usage." Use agents only for "open-ended problems where it's difficult or impossible to predict the required number of steps" and only when "you must have some level of trust in its decision-making."
- Five workflow patterns with selection criteria: **prompt chaining** ("task can be easily and cleanly decomposed into fixed subtasks"), **routing** (distinct categories, accurate classification), **parallelization** (sectioning + voting), **orchestrator-workers** ("subtasks aren't pre-defined, but determined by the orchestrator"), **evaluator-optimizer** (use when "clear evaluation criteria exist; iterative refinement demonstrably improves results" — "analogous to the iterative writing process a human writer might go through").
- Three principles: simplicity, transparency (show planning steps), and investing in the **agent-computer interface (ACI)** as much as a human UI: tool docs with "example usage, edge cases, input format requirements, and clear boundaries"; "Run many example inputs … see what mistakes the model makes, and iterate"; for SWE-bench they "actually spent more time optimizing our tools than the overall prompt."
- Iteration rule: measure and "add complexity *only* when it demonstrably improves outcomes."
- Runnable reference implementations of all five patterns live in the cookbook: https://github.com/anthropics/anthropic-cookbook/tree/main/patterns/agents (basic_workflows, evaluator_optimizer.ipynb, orchestrator_workers.ipynb).

### 2. Claude Code best practices: verification loops are the unit of unattended work

Source: https://code.claude.com/docs/en/best-practices (current as of June 2026).

- The single most load-bearing idea: **"Give Claude a check it can run… It's the difference between a session you watch and one you walk away from."** Gating options escalate: ask-in-prompt → `/goal` condition (re-checked every turn) → **Stop hook** ("a deterministic gate… blocks the turn from ending until it passes") → adversarial **verification subagent** in fresh context ("a fresh model try to refute the result, so the agent doing the work isn't the one grading it").
- **Evidence, not assertions**: "Have Claude show evidence rather than asserting success: the test output, the command it ran and what it returned… it works for sessions you weren't watching."
- **CLAUDE.md hygiene** is treated like code: "review it when things go wrong, prune it regularly, and test changes by observing whether Claude's behavior actually shifts"; "Bloated CLAUDE.md files cause Claude to ignore your actual instructions!" Per-line test: "Would removing this cause Claude to make mistakes?" If Claude already behaves correctly without a rule, "delete it or convert it to a hook."
- **CLAUDE.md vs skills boundary**: "CLAUDE.md is loaded every session, so only include things that apply broadly. For domain knowledge or workflows that are only relevant sometimes, use skills instead."
- **Skills as invocable runbooks**: a `fix-issue` skill with `disable-model-invocation: true` and `$ARGUMENTS` is the documented pattern for side-effectful, manually-triggered workflows (`/fix-issue 1234`).
- **Hooks** for non-negotiables: "Unlike CLAUDE.md instructions which are advisory, hooks are deterministic and guarantee the action happens."
- **Headless fan-out** for batch maintenance: generate a task list, then loop `claude -p "Migrate $file… Return OK or FAIL." --allowedTools "Edit,Bash(git commit *)"`; "Refine your prompt based on what goes wrong with the first 2-3 files, then run on the full set."
- **Adversarial review caveat** (over-engineering risk): "A reviewer prompted to find gaps will usually report some, even when the work is sound… Tell the reviewer to flag only gaps that affect correctness or the stated requirements."
- Failure patterns named: kitchen-sink sessions, correcting-over-and-over (after 2 failed corrections, `/clear` and write a better prompt "incorporating what you learned" — i.e., the lesson goes into the prompt, not the chat), over-specified CLAUDE.md, trust-then-verify gap, infinite exploration (scope or use subagents).
- Spec-first pattern: "have Claude interview you… write a complete spec to SPEC.md," then execute in a fresh session — "Time spent making the spec precise pays off more than time spent watching the implementation."

### 3. Agent Skills: progressive disclosure + the Claude-A/Claude-B authoring loop

Sources: https://www.anthropic.com/engineering/equipping-agents-for-the-real-world-with-agent-skills (Oct 2025); https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices; https://github.com/anthropics/skills.

- **Structure**: a skill is a directory with `SKILL.md` (YAML frontmatter: `name` ≤64 chars kebab-case, `description` ≤1024 chars) plus optional `scripts/` (executed, not loaded), `references/` (loaded on demand), `assets/`. Three levels of progressive disclosure: (1) name+description preloaded at startup, (2) SKILL.md body on trigger, (3) bundled files on demand — "the amount of context that can be bundled into a skill is effectively unbounded."
- **Conciseness doctrine**: "The context window is a public good." "Default assumption: Claude is already very smart" — challenge each paragraph's token cost. Keep SKILL.md body under 500 lines; references one level deep (deeper nesting causes partial `head -100` reads); files >100 lines need a table of contents.
- **Degrees of freedom** is the key authoring dial: high freedom (text heuristics) for context-dependent work; low freedom ("Run exactly this script… Do not modify the command") for fragile sequences. Analogy: narrow bridge vs open field.
- **Descriptions are the routing layer**: third person, "what the Skill does and when to use it," with key trigger terms — "Claude uses it to choose the right Skill from potentially 100+ available Skills."
- **Eval-first skill development**: "Create evaluations BEFORE writing extensive documentation… 1. Identify gaps: Run Claude on representative tasks without a Skill… 3. Establish baseline… 4. Write minimal instructions… 5. Iterate." Evaluations are "your source of truth for measuring Skill effectiveness."
- **Claude A / Claude B loop** (the documented self-improving process): work with Claude A to author the skill, test with fresh Claude B on real tasks, observe, return observations to Claude A ("I noticed Claude B forgot to filter test accounts… maybe it's not prominent enough?"), apply, re-test. "Each iteration improves the Skill based on real agent behavior, not assumptions."
- **Iterate-with-Claude capture**: "As you work on a task with Claude, ask Claude to capture its successful approaches and common mistakes into reusable context and code within a skill." When a skill underperforms, "ask it to self-reflect on what went wrong. This process will help you discover what context Claude actually needs, instead of trying to anticipate it upfront."
- **Script-bundling signal**: "If all 3 test cases resulted in the subagent writing a `create_docx.py`… that's a strong signal the skill should bundle that script. Write it once, put it in `scripts/`."
- **Plan-validate-execute** for risky batch work: emit a `changes.json` plan, validate with a script, then execute — "Catches errors early… Machine-verifiable… Make validation scripts verbose with specific error messages."
- **Durability**: "Avoid time-sensitive information" — keep a collapsed "Old patterns" section instead of date-conditional instructions; consistent terminology; provide a default tool with an escape hatch rather than option lists.
- **Model-portability**: "Test your Skill with all the models you plan to use it with… What works perfectly for Opus might need more detail for Haiku."
- **Anti-MUST guidance** (from skill-creator): "Try hard to explain the **why** behind everything you're asking the model to do… If you find yourself writing ALWAYS or NEVER in all caps… that's a yellow flag — reframe and explain the reasoning instead."
- Direction of travel: Anthropic states future work is "enabling agents to create, edit, and evaluate Skills on their own, letting them codify their own patterns of behavior into reusable capabilities."

### 4. skill-creator: a concrete, reusable skill-benchmarking harness

Source: https://github.com/anthropics/skills/blob/main/skills/skill-creator/SKILL.md (Anthropic's own meta-skill; also https://resources.anthropic.com/hubfs/The-Complete-Guide-to-Building-Skill-for-Claude.pdf).

- Five-stage loop: capture intent → interview & research → write SKILL.md → run & evaluate test cases → iterate.
- **Paired baseline runs**: "For each test case, spawn two subagents in the same turn — one with the skill, one without." For improvements, baseline = snapshot of the old skill version (`cp -r <skill-path> <workspace>/skill-snapshot/`). Workspace layout: `iteration-N/eval-K-name/{with_skill,without_skill}/outputs/` + `eval_metadata.json` + `timing.json`.
- **Quantified benchmarking**: per-run pass-rate over named assertions, tokens, duration (mean ± stddev, delta vs baseline) aggregated into `benchmark.json`; an "analyst pass" surfaces "assertions that always pass regardless of skill (non-discriminating), high-variance evals (possibly flaky), and time/token tradeoffs."
- **Human-in-the-loop before self-revision**: "GENERATE THE EVAL VIEWER _BEFORE_ evaluating inputs yourself" — the human reviews outputs in an HTML viewer and leaves per-run feedback; "Empty feedback means the user thought it was fine."
- **Description optimization as a train/test loop**: generate ~20 trigger queries (8–10 should-trigger, 8–10 near-miss should-NOT-trigger), then `scripts/run_loop.py` "splits the eval set into 60% train and 40% held-out test… running each query 3 times to get a reliable trigger rate… returns `best_description` — selected by test score rather than train score to avoid overfitting."
- North star: "we're trying to create skills that can be used a million times across many different prompts" — generalize from feedback, keep the prompt lean, explain the why, bundle repeated work.
- **Versioning** (enterprise skills doc, https://platform.claude.com/docs/en/agents-and-tools/agent-skills/enterprise): pin skills to specific versions; "run the full evaluation suite before promoting a new version"; treat every update as a deployment.

### 5. Eval-driven development: capability evals graduate into regression suites (Jan 2026)

Source: https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents (published 2026-01-09).

- Definition: "build evals to define planned capabilities before agents can fulfill them, then iterate until the agent performs well."
- Start small: "20-50 simple tasks drawn from real failures is a great start" — sourced from manual pre-release checks, bug tracker/support queue, common failing user tasks. Early changes have large effects, so small N suffices.
- Grader hierarchy: deterministic/code graders first (cheap, objective: string match, unit tests, lint/static analysis); LLM judges with structured rubrics, one isolated judge call per dimension, calibrated against humans, with an out ("return 'Unknown'"); humans as gold standard for calibration and subjective domains.
- **Trajectory vs outcome**: "grade what the agent produced, not the path it took"; outcome = state in the environment, not the agent's claim ("the outcome is whether a reservation exists in the environment's SQL database").
- **pass@k vs pass^k**: at-least-one-of-k vs all-k-succeed; pick by product need (pass^k for customer-facing reliability).
- **Lifecycle**: regression evals "should maintain ~100% pass rate"; capability evals "start at low pass rates… giving teams a hill to climb. As agents improve and capability evals saturate, they graduate to become regression suites." Maintenance should be "as routine as maintaining unit tests" with clear ownership.
- **Never trust scores blind**: "we do not take eval scores at face value until someone digs into the details of the eval and reads some transcripts" — weekly transcript spot-checks; Swiss-cheese redundancy (automated evals + production monitoring + user feedback + occasional human review).
- The multi-agent research post (https://www.anthropic.com/engineering/multi-agent-research-system, 2025-06-13) corroborates: "start with small-scale testing right away with a few examples" (~20 queries); single LLM judge with rubric (factual accuracy, citations, completeness, source quality, tool efficiency; 0.0–1.0 + pass/fail); end-state evaluation for state-mutating agents; "human reviewers find edge cases that evals miss."

### 6. Context engineering: attention budget, just-in-time retrieval, notes as memory (Sep 2025)

Source: https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents (published 2025-09-29).

- "Context engineering refers to the set of strategies for curating and maintaining the optimal set of tokens… during LLM inference"; unlike prompt engineering it is "iterative and the curation phase happens each time we decide what to pass to the model."
- **Context rot / attention budget**: accuracy of recall degrades as context grows (n² attention); guiding principle: "Find the smallest set of high-signal tokens that maximize the likelihood of your desired outcome."
- **System-prompt altitude**: avoid both brittle hardcoded logic and vague platitudes — "specific enough to guide behavior effectively, yet flexible enough to provide the model with strong heuristics."
- **Just-in-time context**: keep lightweight identifiers (paths, queries) and let the agent load data via tools at runtime; hybrid pre-load+explore is best.
- Long-horizon techniques: **compaction** (high-fidelity summarize-and-reinit), **structured note-taking** ("agents write notes to persistent memory outside the context window… similar to maintaining a NOTES.md file or to-do list"), **sub-agent architectures** (focused agents return "condensed summaries (typically 1,000-2,000 tokens)" to the coordinator).
- Tools: "If a human engineer can't definitively say which tool should be used in a given situation, an AI agent can't be expected to do better." Examples: few, diverse, canonical — "examples are the 'pictures' worth a thousand words."

### 7. Agents improving their own tools and prompts (the documented self-improvement loops)

Sources: https://www.anthropic.com/engineering/writing-tools-for-agents (2025-09-11); https://www.anthropic.com/engineering/multi-agent-research-system (2025-06-13).

- Tools post's three-step loop: prototype → run realistic evals ("Prompts should be inspired by real-world uses… based on realistic data sources") collecting runtime/tokens/errors, not just accuracy → **"let agents analyze your results and improve your tools for you"**: concatenate eval transcripts into Claude Code and have it refactor tools/descriptions, verified on **held-out** test sets — improved tools "beyond manually written tools."
- "Even small refinements to tool descriptions can yield dramatic improvements" (cited: Sonnet 3.5 SOTA on SWE-bench Verified after description refinement). Other levers: namespacing (`asana_projects_search`), semantically meaningful identifiers over UUIDs, token-efficient responses with actionable error messages.
- Multi-agent research system: "Claude 4 models can be excellent prompt engineers — when given a prompt and a failure mode, they are able to diagnose why the agent is failing and suggest improvements." Their **tool-testing agent** "attempts to use a flawed MCP tool and then rewrites the tool description to avoid failures, finding key nuances and bugs by testing the tool dozens of times" → "a 40% decrease in task completion time for future agents using the new description."
- Production practices from the same post: **rainbow deployments** (gradual traffic shift, both versions live), **durable execution** ("resume from where the agent was when the errors occurred"), observability of "agent decision patterns and interaction structures" rather than transcript surveillance.

### 8. Recurring-maintenance harnesses: Routines, headless mode, GitHub Actions (Apr–Jun 2026)

Sources: https://code.claude.com/docs/en/routines (research preview, launched ~April 2026); https://code.claude.com/docs/en/best-practices; https://github.com/anthropics/claude-code-action; https://www.infoq.com/news/2026/05/anthropic-routines-claude/.

- **Routines** = "a saved Claude Code configuration: a prompt, one or more repositories, and a set of connectors, packaged once and run automatically" on Anthropic cloud; triggers: scheduled (≥1h cadence, cron), API (`POST /fire` with bearer token + freeform `text` payload), GitHub events (PR/release, filterable). Managed via web, Desktop, or `/schedule` in the CLI (`/schedule daily PR review at 9am`, `/schedule list|update|run`).
- Anthropic's own example use cases are exactly recurring maintenance: **"Docs drift. A schedule trigger runs weekly. The routine scans merged PRs since the last run, flags documentation that references changed APIs, and opens update PRs"**; backlog grooming; deploy verification; alert triage; cross-repo library porting. Press coverage adds "dependency audits, test flakiness reports, and documentation freshness checks."
- Prompt guidance for unattended runs: "The prompt is the most important part: the routine runs autonomously, so the prompt must be self-contained and explicit about what to do and what success looks like."
- Trust caveat: "A green status… does not mean the task in your prompt succeeded. Open the run to read the transcript and confirm what Claude actually did." (Same evidence-over-assertion theme as §2.)
- Safety model: `claude/`-prefixed branch pushes by default; scoped repos/connectors/network; runs use *your* identity. Local alternatives: Desktop scheduled tasks, in-session `/loop`, plain cron + `claude -p` with `--allowedTools`, or claude-code-action in CI ("scheduled maintenance for automated repository health checks").

### 9. Keeping prompt/skill libraries current as new models ship

Sources: https://docs.anthropic.com/en/docs/about-claude/models/migrating-to-claude-4; https://platform.claude.com/docs/en/about-claude/models/introducing-claude-fable-5-and-claude-mythos-5; https://platform.claude.com/docs/en/about-claude/model-deprecations; skill best-practices doc (§3); Console tooling (§10).

- Anthropic ships **per-model migration guides** documenting both compat claims ("strong out-of-the-box performance on existing… prompts and evals") and breaking changes (e.g., temperature/top_p/top_k rejected on Opus 4.7+; prefill unsupported on Fable 5/Mythos 5; new tokenizer ≈30% more tokens for the same text). Migration itself is automated as a **skill**: "/claude-api migrate" in Claude Code automates migration to a target model — i.e., Anthropic's own answer to library drift is an agent-runnable runbook.
- The skills best-practices doc bakes model churn into authoring: test with every model you'll use (Haiku needs more guidance; "Does the Skill avoid over-explaining?" for Opus), and structure content so deprecations move to an "Old patterns" `<details>` block rather than rewriting history.
- Enterprise skills guidance: pin versions, re-run the full eval suite before promoting a skill update — the eval suite (not vibes) is what certifies a skill against a new model.
- skill-creator's benchmark harness takes `--model <model-id>` — re-running the same eval set under a new model is the designed re-certification path.

### 10. Console prompt tooling: prompts-that-write-prompts, with built-in eval loop

Sources: https://www.anthropic.com/news/prompt-generator (2024); https://www.anthropic.com/news/prompt-improver (2024-11); https://www.anthropic.com/news/evaluate-prompts; https://docs.anthropic.com/en/docs/build-with-claude/prompt-engineering/prompt-improver; https://docs.anthropic.com/en/docs/test-and-evaluate/eval-tool.

- **Prompt generator**: describe the task → Claude emits a production-ready template using CoT/structure best practices. **Prompt improver**: takes an existing prompt + your feedback on "what's still not working" and iterates — an explicit evaluator-optimizer loop over prompt artifacts.
- **Evaluation tool**: auto test-case generation, side-by-side comparison of prompt versions, 5-point quality grading, prompt versioning with re-run of the suite — i.e., prompts are versioned artifacts with attached regression suites, the same model Anthropic recommends for skills and tools.

## Design implications for neokapi

Concrete moves for a solo maintainer + Claude running format adoption/maintenance over years:

- **Make the parity harness + spec.yaml the "check Claude can run."** Every format-maintenance session/skill must end by executing spec.yaml assertions + parity + maturity guardrails and pasting the output as evidence — never accept "looks done" (best-practices: evidence over assertion; routines: green ≠ success). Wire the hard gate as a Stop hook or `/goal` for unattended runs.
- **Treat spec.yaml files as the eval suite, with the documented lifecycle**: new format work starts by writing failing spec assertions (eval-driven development: "build evals to define planned capabilities before agents can fulfill them"); 20–50 cases from real failures (Okapi xfail burndown, GitHub issues) per format is enough; once a capability assertion saturates it *graduates* into the regression suite (maturity_test.go) and must stay ~100%. Maturity levels L0–L4 map cleanly onto "capability evals → regression suites" graduation.
- **Restructure the format-engineering runbooks as skills with progressive disclosure**: a lean SKILL.md (<500 lines) per workflow (`implement-format`, `refresh-format-maturity`, `audit-format`) that points to `references/` (format-engineering.md, maturity rubric, per-format gotchas) loaded on demand, and `scripts/` (audit-format.py, regen-okapi-fixtures) that are *executed, not read*. Low-freedom wording ("Run exactly `make regen-okapi-fixtures`") for fragile steps; high-freedom heuristics for triage judgment.
- **Bundle repeated work into scripts**: if successive format runs keep re-deriving the same helper (fixture diffing, skeleton round-trip checker), that's the skill-creator signal to write it once into `scripts/` — "this saves every future invocation from reinventing the wheel."
- **Close the loop after every run (Claude A/B pattern)**: end each format-triage/remediation run with a mandatory self-reflection step — "capture its successful approaches and common mistakes into reusable context and code within a skill" — i.e., the run's last action is a PR against the skill/runbook itself, reviewed by the maintainer. Improvements must cite observed Claude-B behavior ("Claude forgot X on the resx run"), not speculation.
- **Keep a ledger of past runs as structured note-taking, not chat history**: a per-format `notes/` or run-ledger (date, model, maturity delta, failures found, transcript pointer, time/tokens) that future runs read just-in-time. This is Anthropic's "NOTES.md outside the context window" memory pattern; the /format-maturity dashboard dataset is already the quantitative half — add the qualitative half (what went wrong and why).
- **Benchmark skill changes like skill-creator does**: when revising a maintenance skill, snapshot the old version, run paired with-old/with-new (or with/without) subagent runs on the same 2–3 format tasks, grade named assertions, and compare pass-rate/tokens/time before adopting. Don't let Claude grade and revise in one breath — review outputs yourself first ("generate the eval viewer BEFORE evaluating inputs yourself").
- **Schedule the cadence with Routines (or local cron + `claude -p`)**: a weekly "format drift" routine (scan merged PRs + upstream Okapi releases + spec failures since last run → open triage issues/PRs) mirrors Anthropic's own "docs drift" example verbatim. Prompt must be self-contained, state success criteria, and write its ledger entry; review the transcript, not the status light. For the 49-format fan-out, use the documented headless loop: refine on 2–3 formats, then run the full set with `--allowedTools` scoping.
- **Use orchestrator-workers + fresh-context review for burndowns**: a triage orchestrator decides per-format subtasks; an adversarial subagent reviews each format diff against the spec in fresh context, told to "report gaps, not style preferences" and only correctness-relevant findings (avoid the over-engineering trap on byte-exact writers).
- **Prompts and skills are versioned, eval-gated artifacts**: keep skills in-repo (already true), pin/CHANGELOG them, and require the format eval suite to pass before "promoting" a skill revision — exactly the enterprise-skills rule ("run the full evaluation suite before promoting a new version").
- **Model-upgrade playbook**: when a new Claude ships, (1) read the migration guide / run the migrate skill for API-level breaks; (2) re-run the per-skill benchmark and the trigger-description eval under the new model id; (3) *prune* — newer models need less scaffolding, and the skills doc's Opus test is "does the skill avoid over-explaining?"; (4) move superseded instructions to an "Old patterns" collapsed section instead of deleting, and never write date-conditional instructions.
- **Spend description budget on routing**: with 15+ in-repo skills (format-triage etc.), trigger accuracy is governed by the one-line descriptions; adopt skill-creator's near-miss negative tests (e.g., "should NOT trigger format-triage: user asks to add a feature to the JSON reader") and tune descriptions on a held-out split.
- **Let Claude fix its own tooling from transcripts**: when audit-format.py or the harness confuses the agent, feed the failing transcripts back and have Claude rewrite the script/help text — Anthropic measured a 40% task-time reduction from exactly this tool-description rewriting loop, validated on held-out tasks.
- **Guard the attention budget**: keep CLAUDE.md to always-true repo facts; push per-format detail behind just-in-time references (file pointers, grep hints like the BigQuery skill's `grep -i "revenue" reference/finance.md`); have subagents return 1–2k-token summaries instead of raw fixture diffs.

## Key sources

- https://www.anthropic.com/engineering/building-effective-agents (2024-12-19)
- https://www.anthropic.com/engineering/multi-agent-research-system (2025-06-13)
- https://www.anthropic.com/engineering/writing-tools-for-agents (2025-09-11)
- https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents (2025-09-29)
- https://www.anthropic.com/engineering/equipping-agents-for-the-real-world-with-agent-skills (2025-10)
- https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents (2026-01-09)
- https://code.claude.com/docs/en/best-practices (live docs, 2026-06)
- https://code.claude.com/docs/en/routines (research preview, ~2026-04)
- https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices
- https://platform.claude.com/docs/en/agents-and-tools/agent-skills/enterprise
- https://github.com/anthropics/skills/blob/main/skills/skill-creator/SKILL.md
- https://github.com/anthropics/anthropic-cookbook/tree/main/patterns/agents
- https://docs.anthropic.com/en/docs/about-claude/models/migrating-to-claude-4 ; https://platform.claude.com/docs/en/about-claude/models/introducing-claude-fable-5-and-claude-mythos-5
- https://www.anthropic.com/news/prompt-improver ; https://www.anthropic.com/news/prompt-generator ; https://www.anthropic.com/news/evaluate-prompts ; https://docs.anthropic.com/en/docs/test-and-evaluate/eval-tool
- https://claude.com/blog/how-anthropic-teams-use-claude-code
- https://github.com/anthropics/claude-code-action
