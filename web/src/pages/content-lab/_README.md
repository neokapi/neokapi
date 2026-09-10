# Content context lab

The public page is `/content-lab`. Its React views distinguish recorded checks
from architecture and scope illustrations. All component and data
filenames start with `_`, so Docusaurus excludes them from page discovery.

## Import measured evidence

From the repository root:

```sh
make build
node samples/audience-context/run.mjs --binary bin/kapi --output web/src/pages/content-lab/_evidence.json
```

The runner output is imported unchanged. Required schema:
`neokapi-audience-coverage/v1`. The page selects case IDs
`<audience>/clean`, `<audience>/assurance`, `<audience>/repair` and
`<audience>/semantic-contradiction`. Each row supplies `input`, `file`,
`resolvedContext`, `report`, `run`, `contextRun` and provenance hashes. It reads
semantic coverage from `report.execution.analyzers` with ID `voice.guidance`.
The repair is a separately checked authored paragraph, not a model-generated fix.

Empty evidence produces an explicit unavailable state. No synthetic result
stands in for a missing run. Fixture evidence carries its commit, dirty state,
binary and input hashes. A source commit with uncommitted changes is labelled.

## Accessibility and boundaries

Native selects and buttons support keyboard interaction. Pressed state is
exposed for the view and case controls, and a polite status announces a changed
case. Details expose raw input, context and reports. Layouts stack on phones;
long hashes and source text wrap. No motion conveys an execution result, and
reduced-motion preferences disable transitions within the lab.

The policy-example exception view is explicitly an expected-impact illustration.
The sample does not execute a policy-example case. Approval fields are asserted
local provenance. The product map makes no claim that this offline fixture
exercises a connected platform or authenticated review.
