# Scoped writing context fixture

This fixture is a real, isolated kapi project with authored synthetic products
and writing policies. Harbor has support and public status channels. Beacon is
a separate product with deliberately different event communication guidance.
Product profiles and channel bindings select the voice. Audience coordinates
are recorded context metadata; they do not select profile overrides.

The same service update appears as a warm support reply, a faithful paraphrase
and a neutral status notice. Each version is placed in both Harbor destinations.
The first two versions fit the support relationship; their personal opening and
closing conflict with the public status guidance. The neutral version fits the
status channel, while its absence of an acknowledgment and courteous personal
closing departs from the support guidance. These are provisional authored style
judgments, not exact wording targets or claims about real organizations. The
service facts are fictional and consistent across the versions; this exercise
does not establish their real-world truth.

The constraints use semantic guidance rather than regexes matching the candidate
wording. Deterministic checks report applicable guidance as unsupported. Their
passing results do not establish writing compliance. The Beacon destination and
an unbound draft are offline controls. The recipe has no default voice, so the
unbound draft tests a genuine absence of applicable writing guidance.

Prepare resolved context and a bounded review schedule:

```sh
make build
python3 scripts/checkeval/prepare_scoped_context.py \
  --kapi bin/kapi --out harness/out/scoped-context
```

The preparer copies the fixture into `project/`, writes the candidates and runs
actual `kapi context <path> --json` and `kapi check <path> --json` commands for
all eight destinations. Every command uses an explicit project recipe and the
repository's full isolation environment. No model, API, memory service or
plugin is called. `kapi version --json` records the binary identity without
loading a project.

The output contains:

- `resolved/<case>/`: raw context and check stdout/stderr, invocation details,
  timings, hashes and the resolved profile/channel relationship.
- `resolution.json`: the same per-case summaries together, including analyzer
  coverage and effective context reported by the checks.
- `subject/`: six inputs and a fixed Sonnet 5 high `style` schedule. Guidance is
  copied only from the actual resolver's `voice.guide`; point, scope, voice
  identity and notes are retained in `variables.resolved_context`. There is no
  procedural requirement list.
- `offline-controls.json`: the unscheduled Beacon and missing-voice inputs.
- `labels.json` and `provenance.json`: authored judgments, fixture and binary
  hashes, and preparation identity, all outside the subject directory.
- `review.html`: a browser casebook showing each candidate beside its actual
  retrieved guidance, with raw context and deterministic coverage available.

The subject task, audience and surface descriptions are generic and identical.
Destination-specific writing instructions come exclusively from the resolved
voice. No manual selection from the profile files feeds the model input. The
preparer verifies agreement between context and check on the selected profile,
channel and whether a voice was applied. It refuses to overwrite an existing
output and detects a binary changed during preparation.

Validate offline with a built binary:

```sh
python3 -m unittest discover -s scripts/checkeval -p 'test_prepare_scoped_context.py'
```

Set `KAPI_SCOPED_TEST_BINARY` to use another local binary. Real-binary tests are
explicitly skipped when none is available. These checks establish selection and
input integrity, not semantic review quality or benefit relative to a model
without kapi context. Preparation schedules six possible reviews but launches
none; the runner's explicit live switch and persistent attempt cap still apply.
