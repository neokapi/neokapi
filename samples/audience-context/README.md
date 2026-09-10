# Shared constraints across audiences

This English-only sample adapts the audience experiment at `1266554d2`.
The original experiment and its results remain available at that commit.

Four collections bind child, teen, adult and older-adult channels explicitly.
Each channel replaces presentation preferences. A top-level critical wording
constraint survives those overrides. Shared factual guidance remains visible
for a writer or reviewer; deterministic checks do not verify its meaning.

Read [the fixture facts and reading situations](service-facts.md). The wording
rule has a scoped example exception with asserted local approval provenance.
None of the four audience pages receives that exception.

From the repository root, build a fresh binary and run the isolated fixture:

```sh
make build
node samples/audience-context/run.mjs --binary bin/kapi --output /tmp/audience-results.json
```

The runner copies the sample to a temporary directory, disables project and
plugin discovery outside that fixture, and uses throwaway configuration and
cache directories. It records the binary and source hashes, context answer,
raw check output and process exit status. It fails if a critical wording
violation disappears in any audience.

The JSON artifact uses `schema: neokapi-audience-coverage/v1`. Top-level fields
identify the source commit and dirty state, source diff, binary version and hash,
and profile and recipe hashes. Each `results` entry contains:

- `id`, `audience`, `file`, the full `input` object and its SHA-256.
- Expected and actual constraint finding counts, `expectedExit` and `passed`.
- `resolvedContext` and `report`, preserving their structured API shapes.
- `contextRun` and `run`, each with `command`, `exitCode`, `stdout` and `stderr`.
- `semanticVerification`, read from the report's `voice.guidance` execution
  record. Passing requires `unsupported` with `required: false`.

Temporary fixture paths are represented as `<fixture>` in output. Consumers
can use `results[].report` directly; the raw process output remains available
alongside it. The runner removes its temporary project after writing the artifact.

If a process or JSON decode fails, the runner still writes an artifact and exits
unsuccessfully. `failure` retains the case input, error and last raw attempt,
including stdout, stderr, exit status, signal and any timeout or spawn error.
Completed cases remain in `results`; a missing or invalid report is never replaced
with a synthetic passing report.

The semantic contradiction case is intentionally undetected by the literal
wording rule. Its zero findings do not mean the paragraph is factually correct.
The context answer identifies the factual guidance and check reports identify
unsupported semantic analysis. This is a coverage demonstration, with no claim
of measured productivity or comprehensive factual checking.

Stores preserve shared constraints when older clients omit the field during an
update. An explicit empty `constraints` array removes them. The initial authoring
surface is the profile YAML; visual editors may preserve constraints without
offering controls to edit them.
