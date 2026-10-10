# The learning labs

`/learn` on the kapi docs site is a curriculum: three series of short labs,
each a scripted session in one of the sample projects under `samples/`, run by
the browser engine. This note is for whoever adds or changes a lab. The
reader-facing description is the index page itself.

## Where things are

| What | Where |
| --- | --- |
| The curriculum (series, labs, chapters) | `packages/kapi-learn/src/curriculum/` |
| The sample projects, as a module the player seeds from | `packages/kapi-learn/src/samples.gen.ts`, written by `scripts/learn-gen/gen.ts` from `samples/` |
| The lab shell (pipes, redirections, the builtins between kapi commands) | `packages/kapi-learn/src/shell/` |
| The player (poster, stage frame, terminal, chapter card, chapter list, files pane, share) | `packages/kapi-learn/src/player/` |
| The lenses: views drawn above the terminal from the engine's answers | `packages/kapi-learn/src/lens/` |
| Deep links and progress | `packages/kapi-learn/src/deeplink.ts`, `progress.ts` |
| The docs pages: the index and one route per lab | `web/src/pages/learn/index.tsx`, `web/src/components/Learn/LabPage.tsx`, `web/plugins/learn-routes.ts` |
| The verifier | `scripts/learn-verify/verify.ts`, `make learn-verify` |

The package is `@neokapi/kapi-learn`, Apache-2.0, beside `kapi-lab` and
`kapi-playground`. It imports the engine through `@neokapi/kapi-playground`'s
plugin manager, so the navbar status widget shows the same boot the labs
cause, and the preview kit from `@neokapi/ui-primitives/preview` for the files
pane. Nothing in it is Docusaurus-specific; the two files under
`web/src/components/Learn` are the adapter.

## A lab is data

A lab is a `Lab` object (`curriculum/types.ts`): an id, a series and a
position, a sample, a summary and the concepts it teaches, and its chapters.
A chapter has a title, a narration, usually a command, and the exit code and
output substrings that command must produce. A chapter may also write files
first (the edit a reviewer makes, shown in the files pane) and name a file to
open when it has run.

The player runs the chapters in order and only in order, and nothing moves on
a timer. Play runs the first chapter; the chapter card's Next runs the one
after the last that ran (it names it, so the reader knows what is coming) and
Previous looks back without running anything. Jumping ahead from the chapter
list replays the chapters in between at once; a deep link to a later chapter
does the same after the engine is up. The reader can type their own commands
at any point; that leaves the chapter count where it was. The stage has an
Expand control that takes it to the whole window, which a kapi report needs:
the page column fits eighty-odd characters.

The commands are the ones the sample READMEs and the harness demos use, in
the lab shell's syntax: quotes, `|` between the builtins (`head`, `tail`,
`grep`, `cat`, `wc`, `echo`, `printf`, `ls`, `cd`), and `>`, `>>`, `2>`,
`2>&1`. Everything else goes to kapi, including the toolbox verbs (`ksed`,
`kgrep`, `kconv`, `kdiff`), which are subcommands of the one browser binary.
The shell cannot pipe into kapi; a chapter that needs a file writes it.

## Adding or changing a lab

1. Write the lab in `curriculum/labs/<sample>.ts` and list it in that file's
   array. Positions within a series are contiguous from 1; the test in
   `curriculum.test.ts` holds that, the ids, and the prose free of em dashes.
2. Run it: `make learn-verify` boots the wasm in Node and runs every chapter
   through the same shell, holding each to its `exit` and `expect`. A lab's
   expectations should name what the narration points at (`FAILS`, `parked`,
   `20 applied`), so that a change in the engine's wording fails the lab
   rather than teaching something stale.
3. Try it in the browser: `make docs-dev`, then `/learn/<id>`. The poster's
   Play boots the engine; the files pane and the share menu are worth a look
   for any chapter that writes files.

A lab that needs a different starting state than the sample's committed tree
has two levers: `files` (written over the sample before the first chapter)
and `setup` (commands run silently before it, named in the terminal's first
line). The labs after the first in a series use `setup` to reach the state
the earlier labs left, so each lab stands alone.

Every lab seeds its sandbox under `/learn/<id>`, and Play starts the whole
`/learn` tree over (`KapiRuntime.reset`), not the lab's own directory. The
engine keys a project by its recipe's `name:`, so every lab that runs in one
sample is one project with one context, and the workspace outlives the page:
resetting only this lab's directory would leave the decisions and the memory
another lab, or an earlier visit, built in force here. The reset forgets the
project in the workspace, which empties its stores and its record alike
(`App.ForgetProjectsUnder`).

## Explorer labs

The fourth series puts the engine explorers on the stage instead of a
terminal: the flow workspace, the segmentation comparison, conversion, the
PDF structure viewer, vision, audio and video, the bundle anatomy, and a
free terminal (which is an ordinary terminal lab in the KapiMart sample).
An explorer lab has `kind: "explorer"`, an `explorer` id, and chapters whose
`stage` says what the explorer shows, in the explorer's own terms: a sample
name, a scenario, a target format, an anatomy part. A chapter may name
another explorer in its `stage.explorer`; the media lab moves from the
recorded showcase to the live audio and video explorers that way.

The ids are resolved by the docs site in
`web/src/components/Learn/stages.tsx`, which also maps the logical props to
the component's: sample names to `/samples/` URLs, the model base for the
ONNX models, the recorded traces for the flow workspace. The explorer player
(`ExplorerPlayer`) boots what the lab declares on Play, the engine unless
`engine: false` and the `plugins` it names, and then mounts the explorer with
`autoStart`, so the poster is the only gate: each explorer gained an
`autoStart` prop (and the segmentation lab `defaultSampleId` and `autoRun`,
the anatomy viewer `part`) for exactly this. The stage remounts only when a
chapter changes what it shows, so a chapter that only narrates leaves the
explorer and its state alone.

The old routes (`/lab`, `/lab/*`, `/playground-cli`, `/kbf-lab`) redirect to
the labs. The verifier covers terminal labs only; an explorer lab is checked
by the site build (the plugin resolves every lab) and by `assert-no-eager-engine`,
which presses Play on the vision lab and expects the model fetch.

## Playgrounds

A lab with `kind: "playground"` has no chapters. Play opens the stage and a
"Try" card: in a terminal playground (`free-terminal`) the items are commands
the player types and runs when pressed; in an explorer playground (`vision`,
`structure`) they are prompts in prose and the lab's `stage` says what the
explorer shows. The verifier skips playgrounds; the curriculum test holds
them to having something to try.

## Lenses

A lab with `lens` draws a view above the terminal and rebuilds it after
every command settles, from the engine's own answers (`runCapture` on the
session runs a line with nothing on the terminal and returns what it
printed). The registry is `lens/index.ts`; the one lens is the context graph
(`lens/contextGraph.ts` builds it from `kapi ls --stats`, `kapi status`,
`kapi context <file>`, `kapi terms search`, `kapi terms occurrences`, `kapi
memory stats` and `kapi context log`, all `--json`; `lens/layout.ts` places
it; `lens/ContextGraphLens.tsx` draws it). A chapter's `focus` names the part
the view brings forward (`content`, `point`, `context`, `terms`, `memory`,
`editions`, `record`); a node that appeared since the previous snapshot
pulses once, and pressing a node lists what the engine said about it. The
record row holds the context operations the log prints (imports, notes,
rules) and, beside them, the blocks a person established in each edition,
counted from the established share `kapi status` reports, because the
context log leaves block decisions out. In a lens lab the chapter card sits
between the view and the terminal, so Next is pressed with the graph in
sight. The context engine lab (`labs/context.ts`, the Compass sample) is the
one lab with a lens: read the context in, converge, review, converge again,
and watch what each command adds.

## Changing a sample

The samples are the fixtures. After editing anything under `samples/`, run
`make learn-gen` to rewrite `samples.gen.ts`; `make learn-gen-check` is the
freshness gate CI runs. Then `make learn-verify`, because a changed sample
changes what every chapter prints.

`samples/mart/context/terms.json` exists for the labs: the Mart terms and
brand words, as the bundle `kapi store import ./context` reads, so the Mart
project lab imports its vocabulary the way the Northsea and Compass labs do.

The File conversion lab converts three Office documents rather than text
samples: a Word handbook, an Excel workbook and a PowerPoint deck for KapiMart,
served from `web/static/samples/` and named in the lab's chapters by file name.
`scripts/learn-gen/office.py` writes them (`make learn-samples`, run through
`uv`, which installs python-docx, openpyxl and python-pptx for the run), with a
fixed date in the document properties and a fixed timestamp on every zip entry,
so `make learn-samples-check` can hold the committed files to the script. Each
document carries what has to survive a format crossing: headings at two levels,
Word's own list styles, a table whose first row is a header row, bold and
struck-through runs, hyperlinks through the Hyperlink style, a figure with alt
text, three worksheets with currency, percentage and date formats, a slide table
and speaker notes. `core/formats/openxml/wml_header_row_test.go` reads the
handbook, so a regeneration that loses one of those fails a Go test.

The explorer offers as targets the generative writers the engine reports in a
document family (`family` in `kapi formats list --json`: rich-markup and
plain-text), so a document converts to HTML, Markdown, DocLang, AsciiDoc and
plain text, and never to a string catalog or a media format.

## Deep links

`/learn/<id>?c=<chapter-id>` opens a lab at a chapter. `&s=<token>` adds the
files the reader changed on top of the sample (base64url over JSON, bounded
at 96 KB of content), so a colleague opens the same sandbox. The share menu
writes both; the page rewrites `c` as the reader moves and drops `s` once the
session has been restored.

## How a reader finds and follows the labs

Four doors lead to `/learn`: the navbar's **Learn** menu (the index first,
then one entry per series, then the engine explorers), the kapi overview's
"Next" list, a pointer at the top of the Quick Start, and the old `/labs`
address, which redirects. Inside, the index orders the three series and
every card says what the lab teaches and how long it takes; a card shows
progress from this browser, and a "continue" card reopens the last lab at
its chapter. Each lab ends with an "up next" card for the next lab, across
series, so the whole curriculum can be read in one sitting of about an hour.

The path is the product's own: what a project is and how content is
addressed (lab 1), the context and the two questions an agent asks (2), the
checks as a gate and a decision that changes what it finds (3), one edit
through the change contract (4); then the multilingual loop as an extension
of the same point (5), recycle before AI with the demo provider (6), review
as a change set that moves the ship manifest (7), and the hand-off formats
(8); then the engine on its own terms, formats and the content model (9),
tools, flows and prompts (10), and a project with memory and terms (11).
Every chapter runs the real command, so what a reader learns is what the
CLI does, and the "try next" lines under each lab hand the terminal over.

What the earlier lab set lacked, and what this one does not yet do:

- The explorers were feature demonstrations (segmentation engines, OCR,
  conversion, the bundle format) on throwaway fixtures, each with its own
  gate and chrome, ordered from the engine's internals outward. They are now
  the fourth series, each played through the same player with chapters that
  say what to look at; the curriculum starts from the product's first
  concepts and runs on the samples the recordings use.
- The Tidewatch sample (the loop in CI) has no lab: its point is a workflow
  file and a build, which the browser cannot run. A reading chapter on
  `kapi check --ship` exit codes under `.github/workflows` would cover the
  idea without the build.
- The desktop app and the agent surfaces (the skill, MCP) are not in the
  browser, so the labs teach the CLI's view of the same concepts and link
  to the desktop and agent pages.
- The lab prose is English only (the curriculum is TypeScript data, not a
  catalog), and no narration is spoken; the harness's narration pipeline
  could voice the chapters later.

## What the engine does not do here

Every provider is the built-in demo stub, keyless and marked, so a translate
step runs for real with illustrative output. The engine prints its one-time
demo-mode notice once per page, so a lab never expects it. Exit codes are the
native ones (`kapi check` exits 3 on a failing finding); the browser
entrypoint maps errors through `cli.ExitCode`, as the native binary does.
