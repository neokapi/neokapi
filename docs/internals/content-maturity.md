# Content maturity job

Use this job to review a content source, add a conversion target or check how support has changed. The format dashboard shows the declared support tier and the measured axes. Its output table shows the conversions exercised by the job. The connector maturity page on the Bowrain docs site shows the connector operations. Each operation carries its status, the scope of the fixture and a source or test reference.

## What the job measures

The rubric distinguishes a format's implementation from the support promise a
maintainer makes. A format keeps its certification date until a review records
passing evidence for each gate the format names.

The output probe converts a fixture through the host for each registered
writer and reads the result back. A writer that requires the original document
is recorded as such. A target whose reader cannot recover the fixture's values
is recorded as failed, with the fixture that failed.

Connector registration establishes which hosts expose an integration. Fetch
and publish each need their own test against a fixture endpoint. A read-only
connector records publish as unsupported. Every connector is a platform
integration, so `bowrain/scripts/connector-maturity/` measures them and the
Bowrain docs site publishes the result.

## Reproduce a publication

Use the Go version in `go.work`, the repository's ICU build dependencies, Node with the workspace dependencies installed, and Python 3 with PyYAML. Run these commands from the repository root.

```sh
vp install --frozen-lockfile
make check-content-maturity
make -C bowrain check-connector-maturity
```

The first check covers formats and outputs, and the second covers connectors.
Each uses the date of the committed observation and replays the committed
evidence. It runs the format tests, the Prose probe, the output conversions or
the connector fixtures, and fails when:

- a committed dataset disagrees with its history, docs block, evaluation index
  or support declarations, or cites a test file or test name that does not exist
- a pull request changes an observation dated before its own
- a fresh probe weakens a committed result, such as a verified conversion that
  now fails or a lower format grade

A stronger result, a new format and a changed gap pass the check, which prints
them for the next refresh. Repository watermarks in the
[operations ledger](./format-ops-ledger.json), its run records and the
freshness of hosted acceptance receipts belong to the refresh, so the check
leaves them alone. A pull request that changes code without changing a result
needs no refresh. A recorded conversion failure describes the fixture that
failed. A skipped connector check carries an unverified result.

To view the dashboard from this checkout, build and serve the documentation.
Open `/format-maturity` for the support counts and the latest changes.
Select a check to see its evidence. Use History to compare observations or
download the published data. The Bowrain docs site serves the connector
evidence at `/connector-maturity`.

```sh
pnpm --dir web exec docusaurus build --locale en
pnpm --dir web exec docusaurus serve --host 127.0.0.1 --port 4178 --no-open
```

`make refresh-content-maturity` and `make -C bowrain refresh-connector-maturity`
record observations for the current UTC date, or for `MATURITY_DATE`. The weekly
GitHub Actions job runs both, then runs both checks on the refreshed tree. A
manual dispatch on a branch produces a downloadable patch and agent brief. On
the default branch the job also proposes the patch through the repository's
rolling pull request. GitHub starts no checks on that pull request, because a
workflow token opens it, so the producing run's checks are its evidence. A
maintainer reviews changes to the published data. The job uses local fixtures
and requires no model or CMS credentials.

## Hosted consumer checks

On weekly and manual runs, the content maturity job calls the format acceptance
workflow at the same Git revision. The macOS job uploads `acceptance-receipts`.
The measurement job checks the revision and run ID before importing the artifact
into the operations ledger. A retry retains the attempt that produced the
receipt. Pull requests replay the committed receipts, and the weekly refresh
expires a receipt whose tested inputs have changed since its run.

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

Generate a brief for the fleet, one subject or a candidate. The brief is plain text. A connector brief reads the dataset on the Bowrain docs site. Pass it to the coding agent that has access to the checkout. The following commands print the briefs without starting an agent.

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

EML is registered at Available. The [operations ledger](./format-ops-ledger.json)
records the proposal to promote it to Maintained. The initial corpus contains
project fixtures and a message generated by Python. Supported requires additional
corpus evidence at C2. The format row records its current axes. The dossier and corpus identify the covered MIME cases and fixture origins. The output row records the requirement for the original message. The host and CLI conversion evidence records what reaches each tested target. Outlook MSG uses a different storage format and has its own inception brief.

## Read the history

The current datasets are `web/static/data/format-maturity.json`,
`web/static/data/content-capabilities.json` and
`bowrain/web/docs/static/data/connector-capabilities.json`. Their adjacent
history files preserve dated observations. The capability history starts with
format Engine levels recovered from published Git revisions. Output and
connector measurements begin on the date their probes first ran. A missing
historical observation carries no grade. Pull request checks compare prior
observations with the base revision. External Okapi parity is measured in the
okapi-bridge repository. Use the [parity publication runbook](../../.skills/format-ops/references/parity-publish.md)
to trace that result to its workflow run and artifact.

The dashboard shows source links, pass or failure details and changes between observations. Repeating a measurement on the same UTC date replaces that day's observation. Retain the earlier dates when reviewing a publication. A format’s support tier and certification date come from `core/formats/support.yaml`.
