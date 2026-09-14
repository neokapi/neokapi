# kapi-sourcecode

Reads the prose out of source files — product strings, and on request comments —
using tree-sitter grammars. It also locates the comments in source files for
kapi's comment layer, in the languages its manifest lists.

## Why a grammar and not a pattern

In a Homebrew cask, these are both string literals:

```ruby
desc "Desktop workbench for a project's content context"
zap trash: ["~/Library/Caches/Kapi", "~/.config/kapi-desktop"]
```

Only the first is prose. A regex cannot tell them apart, which is why every
grep-based checker eventually grows a hand-curated exemption list. The syntax
tree can: it knows one is the argument of `desc` and the other an element of an
array under `zap`.

So a recipe names the calls that hold prose, the way it already names the keys
that hold prose in a YAML or JSON file:

```yaml
- path: deploy/homebrew/*.rb
  format:
    name: sourcecode
    config:
      nodePathPatterns: [desc, caveats]
```

`nodePathPatterns` is the analogue of the YAML reader's `keyPathPatterns`.
Leaving it empty extracts every string the grammar exposes, each labelled with
the call that owns it — which is how you find out what a file holds before
narrowing it.

## Read-only, deliberately

The manifest declares `capabilities: ["read"]` and there is no writer.

A round-trip error in a document produces a mangled paragraph. A round-trip
error in a program produces one that does not compile — or worse, one that does,
with a changed string escape. kapi's write-back promise rests on byte-faithful
round-trips proven over corpora, and source files would enter it at its weakest
point.

The thing that *does* want to write into source is i18n extraction: wrapping a
literal in a translation call. That is a codemod, a different discipline with
different correctness conditions, and it is deliberately not this.

## Grammars

Ruby for product copy, and the comment languages below. Each grammar needs its
prose-bearing node kinds mapped in `internal/proseread`; the walk itself is
language-independent.

One subtlety worth knowing before adding a language: a heredoc body is not a
child of the call that opens it. `caveats <<~EOS` puts a `heredoc_beginning`
under the call and parks the body at the top level, so attributing by parent
alone credits the text to the enclosing block. Bodies appear in the same order
as their openers, which is what makes the pairing safe.

## Comments

`internal/comments` locates the comments of the languages `manifest.json` lists
under `capabilities.comments`: TypeScript, TSX and JavaScript. kapi sends a file's
bytes over the `LocateComments` RPC and reads back what a built-in comment
provider such as Go's returns: each comment's byte span and lines, its subject,
whether it documents a declaration, its runs, and the comments set aside with
their reason. A recipe reaches it with `comments: true` on a content item.

What the provider decides:

- **Grouping.** Consecutive `//` lines, each alone on its line, form one comment.
  A comment after code on its line, and every block comment, stands alone. A
  directive line splits a group, and a blank line at either edge of a comment is
  set aside.
- **Directives.** A comment a tool reads is set aside with its form:
  `eslint-disable*` and the other ESLint forms, `oxlint-*`, `biome-ignore`,
  `tslint:`, `@ts-expect-error` and the other TypeScript pragmas,
  `/// <reference>`, `prettier-ignore`, coverage ignores, `#__PURE__`, bundler
  magic comments, `@vite-ignore`, `@vitest-environment` and the JSX pragmas,
  source-map comments, and the shebang. The table is `jsDirectives` in `jsts.go`,
  and `testdata/corpus/typescript/directives.ts.txt` holds a comment only each
  form matches.
- **Generated files.** A generator phrase with an instruction not to edit, or
  `@generated`, in the comments on a file's first three non-empty lines sets
  every comment in the file aside.
- **Doc comments.** A `/** */` block documents a declaration when nothing but
  whitespace separates the two. In its runs a block tag with its type and name, a
  tag that holds a value, an `@example` section, an inline tag such as
  `{@link Parser}`, a code span and a URL are placeholders, so a check reads
  sentences only. `@deprecated` sets the deprecated flag.
- **Subjects.** A comment is named for what it sits on, in the Go provider's
  shape: `func/parse`, `class/Parser/run`, `interface/Options/keep`,
  `namespace/Util/const/join`, or `comment` when it sits on nothing.
- **Files that do not parse.** A tree with a syntax error in it is not located,
  and kapi reports the file's comment check as not run.

Each language's canary and comment markers are declared twice, in `jsts.go` and
in `manifest.json`, and a test holds the two equal. kapi sends the manifest's
canary through the plugin beside every real file, and reads a single comment line
through the manifest's markers when a recipe declares comment directives.

### The oracle

The conformance tests hold the provider to comment spans read by a parser that
shares no code with the grammars: `@babel/parser`. The spans for each fixture in
`internal/comments/testdata/corpus/<language>/` sit beside it in
`<fixture>.babel`, with the fixture's sha256, so the Go tests need no node and
fail when a fixture changes without its golden. Grouping and directives in the
oracle come from rules written in `oracle_test.go`, apart from the provider's.

The corpus is drawn from this repository, plus authored fixtures for directives,
literals, doc comments, CRLF line endings and multibyte text. From the repository
root, with node and the pnpm store installed:

```bash
node plugins/sourcecode/internal/comments/testdata/babel-goldens.mjs --add tsx web/src/theme/Root.tsx
make sourcecode-comment-goldens    # regenerates every golden
```

Fixtures come only from outside `bowrain/`: a file copied under this directory
takes this directory's licence.

### Adding a language

A language is an entry in `internal/comments` with its grammar, its syntax
(comment node kinds, directive forms, declarations) and its canary; an entry
under `capabilities.comments` in `manifest.json`; and a hint in
`host/check_comments_plugin.go`, so kapi names this plugin when a file in the
language has no reader. Tests hold the manifest to the code and the hints to the
manifest, and the language needs a key in `core/formats/prose.yaml` for its Prose
axis rung tests.

## Build and check

```bash
make build-sourcecode-plugin    # → bin/kapi-sourcecode
make test-sourcecode-plugin
bin/kapi-sourcecode doctor      # confirms the grammars load AND still separate prose
```

cgo, but no system dependency: the grammars are vendored C compiled by the Go
build, so unlike `kapi-pdfium` this needs nothing on `PKG_CONFIG_PATH`. It still
runs as a subprocess, so a parser fault on a malformed file stays in the plugin.

## Release

The plugin ships on its own tag line, `sourcecode-v*` — the kapi CLI release
does not bundle plugin binaries.

```bash
scripts/package-sourcecode-plugin.sh --version 0.1.0 --out-dir dist/
```

The tarball is the binary, `manifest.json`, `formats/sourcecode/schema.json`, the
`LICENSE` and a `NOTICE` for the MIT grammars compiled in. It bundles no shared
library, because the binary needs none. Pushing a
`sourcecode-v<version>` tag runs `.github/workflows/release-sourcecode.yml`,
which builds the four native platforms, signs each tarball with cosign, publishes
them to the release and registers the version in `neokapi/registry` so `kapi
plugins install sourcecode` resolves. `workflow_dispatch` on that workflow is a
build-only dry run: it never publishes and never writes to the registry.

The registry must already declare a `sourcecode` plugin — `registry-update`
refuses to create new top-level entries, so the register job fails with "plugin
not declared" until the stub is added there once.
