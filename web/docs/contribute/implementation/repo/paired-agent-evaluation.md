---
sidebar_position: 5
title: Paired agent evaluation
description: Compare the same content tasks through ordinary file tools, the kapi CLI skill and MCP, with explicit model identities and bounded subscription usage.
---

# Paired agent evaluation

The paired mode in `scripts/skilleval` measures the contribution of each kapi
integration within a fixed agent and model. It runs identical tasks with ordinary
file tools, the shipped CLI skill, and MCP. Each condition receives the same
source guidance and starting content. The comparison covers each integration as
a package, including its instructions and tool interface.

The existing `skill-eval`, `skill-eval-completion` and `mcp-eval` targets measure
their own scenario sets. Use the paired targets for a comparison across the same
tasks. See [S-03](../../architecture/surfaces/s-03-agent-surfaces.md) for the
interfaces and [A-01](../../architecture/assurance/a-01-testing-and-documentation.md)
for the evidence policy.

## Conditions

| Condition | Available kapi integration | Ordinary file tools |
| --- | --- | --- |
| `baseline` | None | Available |
| `skill-cli` | Shipped skill and CLI, without kapi MCP | Available |
| `mcp` | kapi MCP, without the skill or direct CLI access | Available |

The manifest identifies each agent host, model and effort setting. Comparisons
keep these settings fixed within each host. Effort labels across providers do
not represent equivalent computation. Preserve separate results for different
models and document families.

Each attempt has a private command path and fresh agent configuration. The
transcript audit invalidates observed use of the wrong kapi interface. This
controls accidental mixing of integrations; it is not a boundary against hostile
code. Preparation records the configured sandbox and any unverified behavior.
In particular, configured read restrictions must not be described as enforced
without a successful runtime check. Keep hidden evaluation artifacts outside the
agent's workspace and inspect smoke transcripts before a scored study.

The route audit is conservative: a proposed shell command containing a kapi CLI
route is rejected outside the CLI condition, including a conditional fallback.
That status records the proposed route; it does not prove that the command's
condition was true or that a kapi subprocess executed.

The CLI wrapper binds `KAPI_PROJECT` to the fixture's absolute recipe path and
clears `KAPI_NO_PROJECT` for that invocation. Explicit environment binding resolves
before directory discovery. This supports commands without a recipe flag,
including `inspect`, whose `--project` selects output formats. The surrounding
agent environment retains `KAPI_NO_PROJECT=1`, isolated configuration and plugin
discovery. MCP binds the recipe through its explicit `-p` argument.

## Build and preflight

Build kapi once before preparing a study:

```sh
make build
make paired-eval-preflight
```

Check the CLI wrapper against that binary without launching an agent:

```sh
PAIRED_TEST_KAPI="$PWD/bin/kapi" go test ./scripts/skilleval -run TestPairedCLIWrapper
```

Preflight prepares configurations without inference. The default manifest is
`scripts/skilleval/testdata/paired-study.json`; output defaults to the ignored
`harness/out/paired-eval` directory. Override them with `PAIRED_EVAL_MANIFEST` and
`PAIRED_EVAL_DIR`.

For MCP, preparation also starts the isolated kapi server, initializes its
protocol and lists tools and context resources. Missing required capabilities
or failed discovery block inference. The saved readiness record establishes
server capability at preparation time. It does not establish that an agent host
exposed those capabilities to its model or that the model used them; those are
separate observations from the live transcript.

Keep the binary unchanged within a study. A build embeds its build identity, so
rebuilding can change its hash even when the content-checking source is the same.
The live targets use the existing binary. A changed study requires a new evidence
directory rather than silently combining incompatible attempts.

## Subscription batches

Live runs use signed-in subscriptions. They consume the account's allowance;
token counts or locally estimated dollar figures do not establish the remaining
subscription quota. Check account usage before commissioning a batch. The runner
does not enable extra credits or switch to API billing.

Input-token totals include reported cache reads and writes, with those counts
also retained separately. Host tokenization and subscription weighting differ,
so these observations do not establish equivalent allowance consumption.

The prepared environment excludes API keys and endpoint overrides. Claude uses
the existing subscription credential in memory, with token redaction on recorded
output. Codex uses the signed-in account from a fresh configuration directory.
Prepared reports omit credential-bearing environment values. Review local
transcripts before sharing them even when automatic redaction passes.

```sh
make paired-eval-smoke
make paired-eval-score
```

The default persistent ceiling is six started attempts, including failures.
Review that batch before increasing `PAIRED_EVAL_MAX_ATTEMPTS`. Re-running a
command does not erase attempts already recorded under the study directory.
Smoke and pilot share the ceiling. There are no automatic retries.

The pilot target uses the manifest's task set and repetitions:

```sh
make paired-eval-pilot
```

It remains subject to the same ceiling. Raising the ceiling authorizes additional
subscription use; select it deliberately after reviewing the preceding batch.
`PAIRED_EVAL_ARGS` passes additional paired-runner options. Run the evaluator's
help for the available flags.

## Tasks and scoring

Pilot tasks exercise audience adaptation, scoped wording changes and revised
guidance. They support harness development. Use separate document families for
scored experiments, keeping related original, mutated and repaired variants in
the same partition.

Natural task prompts measure end-to-end behavior, including discovery. Record
failure to use an available integration as an outcome. Explicitly instructed
diagnostic runs answer a separate execution question and retain their own labels.

Independent validators inspect output files and protected content. A passing
artifact check establishes its declared conditions. Human reviewers assess
meaning, suitability and acceptance using a task rubric; absent labels remain
pending. The evaluator's own checks cannot establish general writing quality or
reviewer time savings.

Keep failed, interrupted and capped attempts in the record. Read paired outcomes
within each host and task family, and use document-level uncertainty for scored
studies. Repeated runs on one document do not create independent documents.

## Evidence handling

Keep raw transcripts, generated artifacts and private review records in ignored
local output. Inspect and scrub evidence before publication. The study records
its manifest, implementation and input identities so incompatible runs can be
distinguished. Scoring reads saved attempts without launching an agent.

Model-backed checking is a separate intervention. Record the checker model and
prompt independently from the authoring agent, include its resource use, and
compare against an equivalent additional review pass when attributing benefit.
