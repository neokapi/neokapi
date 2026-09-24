# Context lifecycle fixture

This fixture records native context answers before and after a wording
observation, a person's decision to keep it, a source edit and a reversal.
The recipe is source-only. `kapi up --passes 1` refreshes its extracted content and context graph without an AI provider.

From the repository root:

```sh
make build
node samples/context-lifecycle/run.mjs --binary bin/kapi --output /tmp/context-lifecycle.json
```

The runner copies the recipe and source to a temporary directory. It isolates
project discovery, configuration, workspace data, caches and plugins. The
recording includes the actual command outputs, context answers, operation log,
source hashes, check reports and binary provenance. A failed command writes an explicit
failure and exits unsuccessfully. The runner also checks that the observation
appears as a suggestion, that keeping establishes the forbidden wording, and
that reverting removes it. Checks must pass before the rule is kept, fail while
it is established, and pass after reversal. Suggested findings only report.

To replace the lesson's recorded evidence after inspecting the result:

```sh
node samples/context-lifecycle/run.mjs --binary bin/kapi --output web/src/components/Lab/KapiLessonLifecycleEvidence.json
```

The browser displays the recorded steps. It does not execute native commands.
The fixture demonstrates wording governance; it does not record translation
approvals. Context search records term usage counts and the document and block where each occurrence was found. Source freshness becomes stale after the fixture edit and fresh after the next native run. Context answers retain
the engine's own coverage notes. Reversal may retain the preferred term while
removing the forbidden variants; inspect the actual answer and log.
