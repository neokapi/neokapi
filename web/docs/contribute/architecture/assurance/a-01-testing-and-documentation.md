---
id: a-01-testing-and-documentation
sidebar_position: 1
title: "A-01: Testing and documentation"
description: "A three-tier test pyramid (table-driven unit tests, format round-trips and flow integration, and a build-tagged CLI end-to-end suite), evals in three bands by what they have under test, and a single Docusaurus site, built end to end in its pseudo-locale, whose demo assets are generated from real runs rather than authored by hand."
keywords: [testing, documentation, testify, roundtrip, end-to-end, evals, Docusaurus, walkthrough, harness, architecture decision, neokapi]
---

import { LanesDiagram } from "@neokapi/docs-shared";

# A-01: Testing and documentation

## Summary

Testing is a three-tier pyramid: table-driven unit tests with testify, integration
tests covering format round-trips and whole flows, and a build-tagged end-to-end
suite that drives the real `kapi` binary against real files. Above the pyramid
sit the evals, split into three bands by what they have under test: kapi's own
code, a model's output, and an agent's behaviour. Documentation is a
Docusaurus 3 site serving user docs, architecture decisions, and implementation
notes from one deployment, translated through the dogfood loop and offered in a
pseudo locale on every page. Demo assets come from two complementary pipelines:
an authored walkthrough compiled into an in-browser embed that runs the real CLI
as WebAssembly, and a narrated explainer rendered by the harness. Both run
against real systems.

## Context

A framework with a library surface, a CLI, and a desktop app covers a wide testing
surface. Fast unit tests protect refactors; round-trip tests protect format
fidelity; end-to-end tests protect user workflows. Documentation has to stay
synchronized with actual behavior: a recording that shows a command that no longer
exists defeats its own purpose.

Because demo assets exercise real commands, testing and documentation are tightly
coupled: a recording is both a regression signal and user-facing content. Avoiding
mocks in demo assets is what keeps the documented behavior honest.

The documentation consumer splits in two. **End users** (translators, content and
language engineers) need quickstarts, command references, and workflow tutorials.
**Contributors** implementing formats, tools, plugins, and connectors need
architecture documentation, interface specifications, and testing guides. A single
site with per-audience navigation covers both while keeping deployment simple.

## Decision

### The test pyramid

<LanesDiagram
  caption="Three tiers, each a different unit of confidence. Costs rise and counts fall going down the list."
  handoff="widens scope"
  lanes={[
    {
      title: "Unit",
      sub: "testify · table-driven",
      role: "tool",
      steps: [
        "colocated *_test.go",
        "fresh state per test",
        "runs under -short",
      ],
    },
    {
      title: "Integration",
      sub: "real tools, real files",
      role: "annotate",
      steps: [
        "format round-trips: read → write → compare",
        "whole flows with real tools",
        "block store and project store",
      ],
    },
    {
      title: "End-to-end",
      sub: "build tag e2e",
      role: "qa",
      steps: [
        "builds the kapi binary",
        "drives complete user stories",
        "isolated from the developer's environment",
      ],
    },
  ]}
/>

**Unit tests** use `github.com/stretchr/testify` (`assert` and `require`).
Table-driven tests are the standard pattern, test files colocate with the
implementation as `*_test.go`, and each test starts from fresh state, with no
shared mutable fixtures:

```go
tests := []struct {
    name    string
    input   string
    want    string
    wantErr bool
}{
    {"simple", "hello", "HELLO", false},
    {"empty", "", "", false},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        got, err := Upper(tt.input)
        if tt.wantErr {
            require.Error(t, err)
            return
        }
        require.NoError(t, err)
        assert.Equal(t, tt.want, got)
    })
}
```

**Integration tests** validate format round-trips (read, write, compare), whole
flows with real tools, and store operations. They run as part of `make test`
without the `-short` flag.

**End-to-end tests** live in `kapi/e2e`, behind the `e2e` build tag. `TestMain`
builds the `kapi` binary from the current source tree and each test exercises a
complete user story against real files, asserting on the input and output of the
commands themselves.

Every in-repo invocation of `kapi` that is not the dogfood workflow must isolate
itself, and the end-to-end suite does: `TestMain` pins a throwaway config, data,
and cache home, disables project discovery, and restricts plugin discovery. Without
that, a suite run from inside the tree binds to, and acts on, the repository's
own recipe, discovered by an upward walk from any working directory.

**Frontend tests** run under the workspace runner from a single root install.
Desktop frontend packages carry their own unit suites; the desktop's Go backend is
covered by ordinary colocated Go tests.

**Face parity** holds the CLI, the MCP server and the desktop to one committed
record of answers. `host/facetest` writes one fixture from one description and
embeds one set of expected replies; each face's own suite builds the fixture,
asks its own entry point (the cobra verb, the MCP tool or resource through a
real server session, the desktop backend method), and compares. `make face-parity`
runs the three; a known gap between two faces is pinned by a test that asserts
it, so closing it fails the test that describes it.

The catalog of test targets is `make help`; it is self-documenting and current.
Run a single test directly when iterating: `go test ./core/flow/ -run
TestExecutorCancellation -v`.

### Evals: three bands

An eval measures something a unit test cannot assert: how well a model
translates under kapi's context, whether an agent reaches for kapi at all,
whether a converter keeps the text. What its numbers can mean depends on what it
has under test, so the evals are split into three bands by subject, and within
each band an eval sits beside the AD series describing what it measures:

| Band | Subject | What the numbers can do |
| --- | --- | --- |
| **Engine** | kapi's own code, deterministic | gate a build |
| **Model** | a model's output, sampled | be tracked over time |
| **Agent** | an agent's behaviour, scored per scenario | be read with its transcript, on a maintainer's machine |

The evals live under `scripts/` as small Go programs, one directory each:
`checkeval` (the content checks), `conversioneval` (converter text
completeness), `contexteval` (context-adherence lift), `batcheval` (the batch
ceiling, [M-05](../multilingual/m-05-prompts-and-batching.md)), `priorabeval`
(the effect of a prior version), `authoringeval` (the voice checks and the
voice guide, with an authoring lab that reads a real, pinned repository and
infers its voice from its own docs), and `skilleval` (whether the Agent Skill
fires and whether an agent picks the right MCP tool,
[S-03](../surfaces/s-03-agent-surfaces.md)). Each has a make target of the
same name (`make check-eval`, `make conversion-eval`, `make context-eval`,
`make batch-eval`, `make authoring-eval`, `make skill-eval`, `make mcp-eval`),
a `-publish` variant where a run costs calls, and a committed dataset under
`web/src/pages/<eval>/` that the site renders. `make eval-index` rebuilds the
cover page, and each card's freshness is read out of its dataset rather than
typed on the card.

Three rules keep a published number honest:

- **A sampled eval is reproducible.** The spending evals pin the temperature at
  0 and record it in the dataset, and a test asserts that every provider
  actually sends the temperature it was asked for. A judged score is published
  only after the judge has been validated against human labels
  (`make judge-label`, `make context-eval-validate`).
- **The whole transcript is published, per pass.** An agent eval records every
  message, tool call and tool result of every pass under
  `web/static/skill-eval/transcripts/`, fetched when a reader opens a row. The
  recorder scrubs workspace and home paths, and `make check-eval-publishable`
  refuses a transcript carrying a credential shape.
- **An agent is confined.** Each scenario runs in a throwaway workspace with
  the project's own settings only, so a maintainer's installed skills and
  servers cannot leak into the measurement, and the subject checkout is
  extracted fresh per run because an agent mutates the tree it reads.

None of the agent or model evals runs in CI: they drive real models and real
agents and need local credentials, so the committed dataset is all a build ever
sees. The dashboards are published under the site's **Tests & Evals** navigation.

### The documentation site

The site at `web/` uses [Docusaurus](https://docusaurus.io/) 3 with React 19:

```
web/
├── docusaurus.config.ts     # site configuration (single docs instance at "/")
├── sidebars.ts              # docs sidebar
├── src/pages/               # custom React pages (landing, dashboards)
├── docs/                    # all documentation, served at "/"
│   ├── kapi/                # CLI + desktop + get-started + guides + walkthrough MDX
│   ├── framework/           # concepts: content model, flows, formats, segmentation
│   ├── react/               # the i18n runtime for React
│   ├── toolbox/             # format-aware command-line utilities
│   ├── reference/           # generated command / format / tool reference
│   └── contribute/
│       ├── architecture/    # architecture decisions (this document)
│       └── implementation/  # schemas, protocols, algorithms
├── i18n/<locale>/           # translated docs, a build artefact the loop writes
├── walkthroughs/            # authored prompts: {id}.md + {id}.scene.yaml
├── scenes/                  # per-walkthrough embed fixtures, seeded in-browser
└── static/
    ├── img/                 # local images (logos, favicons)
    └── data/                # generated dashboard datasets
```

A single content-docs instance serves everything from `web/docs/` with
`routeBasePath: "/"`. Audience separation is by top-level section rather than by
separate plugin instances: user-facing docs under `kapi/`, `framework/`, `react/`,
`toolbox/`, and `reference/`; contributor docs under `contribute/architecture/` and
`contribute/implementation/`.

The site is itself a kapi project. The dogfood recipe reads `web/docs/` as a
collection and writes each target locale under `web/i18n/<locale>/`, which is
gitignored and regenerated by the loop, never edited by hand. Every page,
the sidebar labels, the React pages and the site chrome are offered in the
pseudo locale (`qps`), so a string that escaped extraction is visible as plain
English on a page where everything else is marked. A language switcher offers
the locales the site builds. The source locale builds strictly (a broken link
fails the build); a target locale warns, because a translation that has not
caught up is pending work rather than a defect.

Architecture decisions are organized by concern and **updated in place** as
subsystems evolve, rather than appended chronologically. Each one describes the
current state of its subsystem; the history lives in version control.
Implementation notes hold tactical detail (schemas, algorithms, routes) that
would otherwise bloat a decision document.

Production is hosted on GitHub Pages, deployed on push to the main branch. Large
assets (videos, screenshots, ML models, the WebAssembly engine) are served from
a CDN rather than committed, so `static/video/` and `static/wasm/` are build
outputs and are not in version control. That is a delivery detail deliberately
left out of this decision; see
[CDN assets](/contribute/implementation/repo/cdn-assets).

### The walkthrough pipeline

Demo assets for the site come from one authored unit compiled into two artifacts:

1. **The walkthrough prompt** is what a human writes. Each lives at
   `web/walkthroughs/{id}.md` with YAML frontmatter declaring an ordered scene
   list, each scene carrying an id, a kind, a binary, fixtures, and a
   `smoke_contract` of commands to re-run for regression. The prose sections are
   the source of truth for everything the published page says.

2. **The interactive embed** is generated from the companion
   `web/walkthroughs/{id}.scene.yaml`. `scripts/walkthrough-gen/gen.ts` compiles
   each scene into a playground embed config, keeps the prompt's `smoke_contract`
   in sync, and writes any fixture bytes the embed seeds in the browser under
   `web/scenes/{id}/`. The generator is deterministic: it formats its output with
   the workspace formatter and writes no timestamps, so re-runs are idempotent:

   ```bash
   node --experimental-strip-types scripts/walkthrough-gen/gen.ts <id>     # one walkthrough
   node --experimental-strip-types scripts/walkthrough-gen/gen.ts --all    # all of them
   node --experimental-strip-types scripts/walkthrough-gen/gen.ts --check  # fail if any output is stale
   ```

   The embeds are committed, so the site builds straight from them; regenerate only
   when a `.scene.yaml` changes. There is no make target or CI step that invokes
   the generator; it is run by hand. The embeds render live in the browser against
   the CLI compiled to WebAssembly (`make web-wasm-cli`).

3. **The published page** interleaves the prompt's prose into the MDX under
   `web/docs/kapi/`, embedding the playground as the primary artifact and, where a
   narrated explainer exists, a themed video:

   ```mdx
   import { ThemedVideo } from "@neokapi/docs-shared";

   <ThemedVideo
     sources={{
       light: "/video/kapi/bilingual-workflow-light.webm",
       dark: "/video/kapi/bilingual-workflow-dark.webm",
     }}
     maxWidth="900px"
   />
   ```

   `ThemedVideo` matches the active colour scheme; the harness supplies matched
   light and dark WebM files, preferred for size and quality.

The site generates no screenshots of its own: `web/static/img/` carries logos and
favicons plus whatever image set is staged from the assets bundle.

### Real systems

Demo assets run against real infrastructure. The embeds execute the real CLI
compiled to WebAssembly against fixtures under the scene directory; no command is
mocked. Harness recordings drive real binaries, a real identity provider, and a
real SQLite database. Third-party services outside this project (translation
providers, external model APIs) may be mocked for isolation, and nothing else may.

The `smoke_contract` in each prompt is re-run by `make docs-verify-snippets`,
driving the WebAssembly CLI, to prove the documented commands still pass. A
behavior change that breaks a documented command fails the contract.

### Asset generation and staging

Videos, screenshots, and ML models are produced on a developer machine, not in CI,
and published to the CDN; the site references them by URL:

```bash
make harness-videos        # render the narrated explainers (light + dark)
make harness-videos-staged # full pass: stack up → seed → record → narrate → package
make publish-cdn-videos    # publish the videos → CDN
make publish-cdn-all       # publish videos + images + models → CDN
make web-wasm-cli          # build the in-browser CLI → web/static/wasm/
```

CI does **not** record, render, or stage. The site builds with the CDN URL set and
references the assets there; the WebAssembly playground is built in CI and
published to the CDN by commit. Someone editing only documentation text relies on
the live CDN assets, and can stage assets same-origin on demand to preview without
the CDN.

### Verification checklist for surface changes

Before committing a change that affects documented behavior:

1. Lint, format, and typecheck pass across the frontend.
2. Frontend unit tests pass.
3. Production builds succeed.
4. The Go build succeeds.
5. Affected `smoke_contract`s still pass under `make docs-verify-snippets`.
6. Affected embeds are regenerated, and where a demo video changed, it is
   re-rendered and republished.
7. The prose passes `make check-docs-prose`, which runs `kapi check` over the
   documentation under the project's own voice profile.
8. Every sidebar id resolves to a page (`scripts/check-sidebar-ids.sh`), and the
   desktop's generated bindings are fresh when a bound Go type changed
   (`scripts/check-wails-bindings.mjs`).
9. The three faces still agree (`make face-parity`), and the desktop names no
   interchange format (`scripts/check-desktop-interchange.sh`).
10. The JSX translatability table and its Go mirror agree
    (`make check-translatability`).

## Consequences

- Documentation lives alongside code, so it moves with the feature.
- Two-audience separation gives clear navigation without a second site.
- Architecture decisions and implementation notes are readable both in-repo and on
  the published site.
- Demo assets are generated from actual commands and interfaces, so they cannot
  quietly describe a version that no longer exists.
- One authored walkthrough prompt drives both the generated embed and the published
  page, so a documented command and its in-browser playground stay in lock-step.
- Recording against real systems means a breaking change in identity or an API
  surfaces as a recording failure, a useful canary for integration regressions.
- The pyramid buys coverage at every level with the appropriate speed and cost
  trade-off, and the build-tagged end-to-end suite stays out of the ordinary test
  run.
- An eval's band says what its number can be used for, and a published number
  carries the temperature it ran at and the transcript behind it.
- The site translates itself through the same loop it documents, so a gap in
  extraction shows on the page before a user reports it.
- One documentation stack shared across the library, CLI, and desktop keeps the
  documentation single-sourced and avoids duplicated infrastructure.

## See also

- [F-01: The framework and its modules](../foundations/f-01-framework-and-modules.md): the module layout the isolated builds assert
- [A-02: Parity with the Okapi Framework](a-02-parity.md): the fidelity harness above the pyramid
- [M-05: Prompts and batching](../multilingual/m-05-prompts-and-batching.md): the batch eval and its kept record
- [S-01: The kapi CLI](../surfaces/s-01-kapi-cli.md): the surface the embeds run
- [S-02: Kapi Desktop](../surfaces/s-02-kapi-desktop.md): the desktop surface and its tests
- [S-03: Agent surfaces](../surfaces/s-03-agent-surfaces.md): the skill and MCP evals in the agent band
- [Testing guide](/contribute/testing): how to write tests for a new format or tool
- [CDN assets](/contribute/implementation/repo/cdn-assets): where the generated assets live
