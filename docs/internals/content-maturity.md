# Content maturity job

Use this job to review a content source, add a conversion target or check how support has changed. The format dashboard shows the declared support tier and the measured axes. Its output and connector tables show the operations exercised by the job. Each operation carries its status, the scope of the fixture and a source or test reference.

## Initial review, 20 November 2026

The existing rubric distinguishes a format's implementation from the support
promise a maintainer makes. The previous publisher could renew certification
across the fleet after a repository audit. The job now preserves the date until
a review has evidence for each gate named by the format.

The writer registry also includes formats that require the original document.
The output probe exercises conversion through the host and reads the result
back. It found that converting WebVTT to SubRip lost cue timing on read-back.
A regression test now checks both cue times. The first observation also records
catalog writers whose readers could not recover both values from a foreign
catalog.

Connector registration establishes which hosts expose an integration. Fetch
and publish need their own evidence. WordPress exercises both operations
against a fixture REST endpoint. Figma has a verified fetch path and a test
for its read-only delivery contract. The format and connector rows share a
history, while each test measures the operation its caller relies on.

## Reproduce a publication

Use the Go version in `go.work`, the repository's ICU build dependencies, Node with the workspace dependencies installed, and Python 3 with PyYAML. Run these commands from the repository root.

```sh
vp install --frozen-lockfile
make check-content-maturity
```

The check uses the date of the committed observation. It runs format tests, the Prose probe, output conversions and connector fixtures. A missing required test or a stale projection fails the check. A recorded conversion failure describes the fixture that failed. A skipped connector check carries an unverified result.

To view the dashboard from this checkout, build and serve the documentation.
Open `/format-maturity` for the support counts and the latest changes.
Select a check to see its evidence. Use History to compare observations or
download the published data.

```sh
pnpm --dir web exec docusaurus build --locale en
pnpm --dir web exec docusaurus serve --host 127.0.0.1 --port 4178 --no-open
```

`make refresh-content-maturity` records observations for the current UTC date. The weekly GitHub Actions job uses that target. A manual dispatch on a branch produces a downloadable patch and agent brief. On the default branch the job also proposes the patch through the repository’s rolling pull request. A maintainer reviews changes to the published data. The job uses local fixtures and requires no model or CMS credentials.

## Hosted consumer checks

On weekly and manual runs, the content maturity job calls the format acceptance
workflow at the same Git revision. The macOS job uploads `acceptance-receipts`.
The measurement job checks the revision and run ID before importing the artifact
into the operations ledger. A retry retains the attempt that produced the
receipt. Pull requests replay the committed evidence.

Each receipt names the format, workflow, GitHub Actions run and attempt, tested
commit, and external consumer tests. The receipt includes the result for each
fixture and a SHA-256 digest of those results. Only a complete set of passing
consumer checks qualifies as passing acceptance. A mix of passes and skips
records partial coverage. Internal reparsing and unregistered tests provide no
consumer evidence. To add a consumer, declare its Go suite in
`scripts/format-ops/acceptance-receipts.mjs`. Use `single` when the top-level test runs the
consumer itself, or `fixtures` when each subtest runs a consumer. The
`jq-ajv` suites run both validators in separate subtests for each fixture. A support-tier renewal also requires evidence
for every other gate in the format's declaration.

## Give an agent a task

Generate a brief for the fleet, one subject or a candidate. The brief is plain text. Pass it to the coding agent that has access to the checkout. The following commands print the briefs without starting an agent.

```sh
node scripts/format-ops/agent-brief.mjs
node scripts/format-ops/agent-brief.mjs format:eml
node scripts/format-ops/agent-brief.mjs output:markdown
node scripts/format-ops/agent-brief.mjs connector:wordpress
node scripts/format-ops/agent-brief.mjs new:format:msg
```

For an unattended invocation, use the CLI's standard input in a dedicated worktree. For example, run either of these commands from the worktree root.

```sh
node scripts/format-ops/agent-brief.mjs format:eml | codex exec -
node scripts/format-ops/agent-brief.mjs format:eml | claude -p
```

Use your agent's normal repository permissions. The deterministic check runs independently of the model. For changes to a judged quality dimension, follow the calibration and citation rules in the [format operations runbook](./format-ops.md).

## Decide what to implement

| Subject | Entry point | Evidence to collect |
| --- | --- | --- |
| File format | A reader maps bytes to blocks. A native writer may use the source skeleton. | Detection, extraction, same-format edits, preserved data, malformed input and a consumer that can read the result |
| Output format | A registered writer projects foreign blocks through `kconv`. | A source with the expected shape, a target read-back and assertions about content and structure |
| Integration connector | A registry factory exposes an external content system. | Discovery, fetch and publish through its public methods against a fixture endpoint |
| Project source connector | A checkout exchanges blocks with a venue. | Push and pull through the host, including block identity and delivery into target files |

A CMS can expose documents in several formats. Register its connector once. Reference the relevant formats in tests for fetched and delivered items. The shared model covers identity, operation, evidence and history. Apply format fidelity and support tiers to the file contract. Keep connector operations directional so that read-only sources and delivery gaps remain visible.

1. Record the candidate, its use case, the governing specification or API and a fixture with known provenance. Classify it on the [radar](./format-radar.yaml). Record the acceptance decision before implementation.
2. Implement the accepted operations at the normal registry entry points. Use the [format implementation guide](../../.skills/implement-format/SKILL.md) for file formats. Add the source corpus, native spec examples and a support entry. A format that can write edits back to its source needs a separate test before it can claim generation from foreign blocks.
3. Add behavioral evidence for each operation you claim. Exercise the consumer-facing path as well as the package. Include malformed or unsupported input and a representative edit. Add a conversion fixture to the host probe or a connector operation to its probe map. A new registered subject appears even when it has open evidence gaps.
4. Run the package tests and consumer acceptance tests. Refresh the dashboards, then run the check again. Inspect the changed rows, prior observations, declared tier and remaining gaps.
5. Propose a promotion when the gating axes and named CI jobs meet the [tier requirements](./format-maturity.md). A maintainer approves a change to the promise. Renew certification only with passing evidence for that format's named gates. Continue with the first unverified operation or the next axis gap.

## Worked example using EML

The accepted input is a saved Internet message. The native reader extracts the subject and eligible plain-text MIME bodies. The writer splices edits into the original message. Attachments and HTML alternatives stay in the skeleton. The subject becomes a heading when converted to Markdown, HTML or DocLang.

```sh
make build
sample=core/formats/eml/testdata/plain.eml
bin/kapi kconv "$sample" --to md
bin/kapi kconv "$sample" -o /tmp/neokapi-message.dclg.xml
```

The package tests compare unchanged messages byte for byte and read back edited messages. The acceptance test uses Python's email library as an independent consumer. The binary handoff test extracts a multipart message into a project archive, removes the source, applies an edit and merges from the archive. The file connector test delivers an edited subject while preserving unrelated headers and body bytes.

EML enters the registry at Available. The [operations ledger](./format-ops-ledger.json)
records the proposal to promote it to Maintained. The initial corpus contains
project fixtures and a message generated by Python. Supported requires additional
corpus evidence at C2. The format row records its current axes. The dossier and corpus identify the covered MIME cases and fixture origins. The output row records the requirement for the original message. The host and CLI conversion evidence records what reaches each tested target. Outlook MSG uses a different storage format and has its own inception brief.

## Read the history

The current datasets are `web/static/data/format-maturity.json` and `web/static/data/content-capabilities.json`. Their adjacent history files preserve dated observations. The capability history starts with format Engine levels recovered from published Git revisions. Output and connector measurements begin on the date their probes first ran. A missing historical observation carries no grade. Pull request checks compare prior
observations with the base revision. External Okapi parity is measured in the
okapi-bridge repository. Use the [parity publication runbook](../../.skills/format-ops/references/parity-publish.md)
to trace that result to its workflow run and artifact.

The dashboard shows source links, pass or failure details and changes between observations. Repeating a measurement on the same UTC date replaces that day's observation. Retain the earlier dates when reviewing a publication. A format’s support tier and certification date come from `core/formats/support.yaml`.
