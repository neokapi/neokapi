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
| The player (poster, terminal, transport, chapters, files pane, share) | `packages/kapi-learn/src/player/` |
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

The player runs the chapters in order and only in order. Jumping ahead
replays the chapters in between at once; a deep link to a later chapter does
the same after the engine is up. The reader can type their own commands at
any point; that pauses autoplay and leaves the chapter count where it was.

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

## Changing a sample

The samples are the fixtures. After editing anything under `samples/`, run
`make learn-gen` to rewrite `samples.gen.ts`; `make learn-gen-check` is the
freshness gate CI runs. Then `make learn-verify`, because a changed sample
changes what every chapter prints.

`samples/mart/context/terms.json` exists for the labs: the Mart terms and
brand words, as the bundle `kapi store import ./context` reads, so the Mart
project lab imports its vocabulary the way the Northsea and Compass labs do.

## Deep links

`/learn/<id>?c=<chapter-id>` opens a lab at a chapter. `&s=<token>` adds the
files the reader changed on top of the sample (base64url over JSON, bounded
at 96 KB of content), so a colleague opens the same sandbox. The share menu
writes both; the page rewrites `c` as the reader moves and drops `s` once the
session has been restored.

## What the engine does not do here

Every provider is the built-in demo stub, keyless and marked, so a translate
step runs for real with illustrative output. The engine prints its one-time
demo-mode notice once per page, so a lab never expects it. Exit codes are the
native ones (`kapi check` exits 3 on a failing finding); the browser
entrypoint maps errors through `cli.ExitCode`, as the native binary does.
