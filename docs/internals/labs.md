# Labs learning path

The website's course map and lesson navigation use
`web/src/components/Lab/curriculum.ts`. Each lesson has a question, a learning
objective and an execution mode. The framework lessons introduce document
representation and processing; the kapi lessons introduce project state,
applicable context and decisions. Specialist media experiments and unrestricted
workspaces are listed as electives.

## Experiments and evidence

Keep instructions and prediction questions visible before an experiment loads.
Wrap expensive components in `LabLaunch`, so visiting a lesson does not start
its engine or download its evidence. The component loads on launch; its own
controls govern subsequent runs. Closing an experiment unmounts its controls;
the shared browser runtime can remain active for another lesson.

Label execution where the learner sees the result:

- Browser experiments run supported engine operations or the specified browser
  model bridge. A deterministic demo provider demonstrates processing behaviour,
  not model quality.
- Recorded experiments display the original inputs and outputs. A selector
  chooses a recorded case rather than executing a modified input.
- Authored references, such as segmentation boundaries, identify their
  interpretation and must not be presented as measured results.

Keep preservation and check coverage separate. An unchanged preview does not
prove byte preservation; a passing check does not establish that a semantic
analyzer ran. Retain errors and coverage limits in evidence views.

## Reproduction

The shared checkout fixtures live in `packages/kapi-lab/src/samples.ts`.
`scripts/verify-snippets/command-surface-smoke.ts` exercises them through the
browser engine. The native context lifecycle has its own isolated runner:

```sh
make build
node samples/context-lifecycle/run.mjs --binary bin/kapi --output web/src/components/Lab/KapiLessonLifecycleEvidence.json
```

The runner records the binary, fixture and runner hashes, the source revision,
commands and results. It uses disposable project and workspace directories.
Do not replace a failed native result with invented evidence. The audience and
authoring lessons reuse the evidence associated with `/content-lab` and
`/authoring-lab`; their own runners define regeneration.

## Validation

```sh
make wasm-surface-smoke
make docs-build-prod
make kapi-storybook-build
make pre-push
```

Run the web and lab-package component tests with `vp test` from `web` and
`packages/kapi-lab`. Inspect the course and launched experiments in the browser,
including keyboard navigation and a narrow viewport. Loading and evidence
stories are registered in `storybook/.storybook/main.ts`.

Run the production build and the development server sequentially: both use
`web/.docusaurus`, and a multilingual production build replaces its generated
route and locale state. The source locale stays strict; incomplete target
translations remain warnings.

The harness currently contains no walkthrough recordings of these website lab
pages. Changes to shared components also used by recorded surfaces must be
checked against the harness inventory and the documentation-asset runbook.
