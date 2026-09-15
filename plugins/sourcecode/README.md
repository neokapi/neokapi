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
under `capabilities.comments`: TypeScript, TSX, JavaScript, Python, Bash, CSS,
Rust, Java, C#, C, C++ and Ruby. kapi sends a file's bytes over the `LocateComments` RPC and reads back what a built-in comment
provider such as Go's returns: each comment's byte span and lines, its subject,
whether it documents a declaration, its runs, and the comments set aside with
their reason. A recipe reaches it with `comments: true` on a content item.

The plugin never writes a file. For a language whose manifest entry declares
`rewrite`, TypeScript at present, `kapi apply` writes a comment back: kapi
renders the text into the comment's layout, has this plugin locate the rewritten
file, and writes it only when the project's formatter, oxfmt or prettier, runs
and agrees.

What the provider decides:

- **Grouping.** Consecutive line comments of one kind (`//`, or `#` in Python and
  Bash), each alone on its line, form one comment. Rust's `///`, `//!` and `//`
  are three kinds.
  A comment after code on its line, and every block comment, stands alone. A
  directive line splits a group, and a line with nothing after its comment
  marker at either edge of a comment is set aside.
- **Directives.** A comment a tool reads is set aside with its form:
  `eslint-disable*` and the other ESLint forms, `oxlint-*`, `biome-ignore`,
  `tslint:`, `@ts-expect-error` and the other TypeScript pragmas,
  `/// <reference>`, `prettier-ignore`, coverage ignores, `#__PURE__`, bundler
  magic comments, `@vite-ignore`, `@vitest-environment` and the JSX pragmas,
  source-map comments, and the shebang in TypeScript and JavaScript. Python sets
  aside the shebang, the encoding declaration on a file's first two lines,
  `# type:`, `# noqa`, `# pragma:`, `# nosec`, and the `pylint:`, `fmt:`,
  `mypy:`, `pyright:`, `ruff:` and `isort:` markers. Bash sets aside the shebang
  and `# shellcheck`, and CSS `stylelint-disable`, `stylelint-enable`,
  `prettier-ignore` and source-map comments. Rust sets aside an SPDX licence tag
  and rust-analyzer's `// region:` folding markers. Java sets aside IntelliJ's
  `noinspection`, Checkstyle, Sonar and PMD suppressions, formatter switches,
  Eclipse's `$NON-NLS` markers and fall-through markers, and C# ReSharper's
  switches, formatter switches and Sonar suppressions. C and C++ set aside an
  SPDX tag, clang-tidy's NOLINT forms, clang-format's switches, cppcheck, IWYU,
  lcov and gcovr pragmas, an editor's mode line, and labels on closing lines such
  as `#endif // DEBUG` or `} // namespace kapi`. Ruby sets aside the shebang,
  magic comments such as `frozen_string_literal:`, Sorbet's `typed:` sigil,
  RuboCop's and Standard's switches, RDoc's directives and SimpleCov's
  `:nocov:`. The tables are
  `jsDirectives`, `pythonDirectives`, `bashDirectives`, `cssDirectives`,
  `rustDirectives`, `javaDirectives`, `csharpDirectives`, `cDirectives` and
  `rubyDirectives`, and each language's
  `testdata/corpus/<language>/directives.*.txt` holds a comment only each form
  matches.
- **Generated files.** A generator phrase with an instruction not to edit, or
  `@generated`, in the comments on a file's first three non-empty lines sets
  every comment in the file aside, and so does `<auto-generated>` in C#.
- **Doc comments.** In TypeScript and JavaScript a `/** */` block documents a
  declaration when nothing but whitespace separates the two. In Rust `///` and
  `/** */` document the item after them, with attributes allowed between, and
  `//!` and `/*! */` the module they sit in, following rustc's rule that `////`,
  `/***` and `/**/` are plain comments. Java's Javadoc and C#'s `///` and
  `/** */` XML documentation document the declaration after them, and their
  HTML and XML tags are placeholders, a `<code>`, `<c>` or `<pre>` element with
  its content. In C and C++ Doxygen's `///`, `//!`, `/** */` and `/*! */`
  document the declaration after them, and its `@param` and `\\param` commands
  are placeholders. In Ruby a `#` comment directly above a method, class, module,
  constant or DSL call documents it, as RDoc and YARD read it, and YARD's tags,
  with their bracketed types, are placeholders. Python, Bash and CSS comments document
  nothing, and a Python docstring is a string. In its runs a block tag with its type and name, a
  tag that holds a value, an `@example` section, an inline tag such as
  `{@link Parser}`, a code span and a URL are placeholders, so a check reads
  sentences only. `@deprecated` sets the deprecated flag.
- **Subjects.** A comment is named for what it sits on, in the Go provider's
  shape: `func/parse`, `class/Parser/run`, `interface/Options/keep`,
  `namespace/Util/const/join`, `var/OUT` in Bash, `rule/.header` and
  `media/rule/.c` in CSS, `impl/Point/origin` and `module` in Rust,
  `class/Parser/enum/Kind/TEXT` and `package` in Java, `class/Parser/Run` in C#,
  `module/Kapi/class/Parser/initialize` and `cask/desc` in Ruby, or `comment`
  when it sits on nothing. A comment after
  code on its line sits on nothing after it.
- **Literals.** A marker inside a string, a template literal, a regular
  expression, JSX text, a heredoc, a parameter expansion such as
  `${name#prefix}`, a Rust or C++ raw string, or an unquoted CSS `url()` is
  content.
- **Files that do not parse.** A tree with a syntax error in it is not located,
  and kapi reports the file's comment check as not run. C and C++ read each
  file's comments a second time with a lexical scan, in `clex.go`, that shares
  nothing with the grammar. It reads string and character literals with their
  prefixes, C++ raw strings, line splices, digit separators and header names.
  A file is located only when the tree reports a comment at exactly each span
  the scan reads and at no other. A file whose macros leave syntax errors in the
  tree is therefore located, and a file where the grammar folds a comment into a
  `#define` value is not. A comment is `comment` and documents nothing when the
  declaration it documents, or any declaration its subject path names, holds a
  syntax error.
- **Dialects.** GNU C reads raw string literals and ISO C does not, and `??/` is
  a backslash only where trigraphs are enabled. A file holding either is scanned
  both ways and is unlocated when the two readings find different comments. A
  comment inside `#if 0` is a comment, as libclang reads it. Over the
  repository's C files and the libc++ headers, every span in a located file
  matches libclang.

Each language's canary and comment markers are declared twice, in its Go file
(such as `jsts.go`) and in `manifest.json`, and a test holds the two equal. kapi sends the manifest's
canary through the plugin beside every real file, and reads a single comment line
through the manifest's markers when a recipe declares comment directives.

### The oracle

The conformance tests hold the provider to comment spans read by a reader that
shares no code with the grammars. The fixtures in
`internal/comments/testdata/corpus/<language>/` are read by:

- `@babel/parser` for TypeScript, TSX and JavaScript, whose spans sit beside each
  fixture in `<fixture>.babel`;
- the `tokenize` module of Python's standard library for Python, whose spans sit
  beside each fixture in `<fixture>.tokenize`, its character columns converted
  to byte offsets;
- rustc's own lexer, published as `ra-ap-rustc_lexer`, for Rust, run by the
  program in `testdata/rust-lexer/`, whose spans sit beside each fixture in
  `<fixture>.rustc`;
- javac's own tokenizer for Java, run by `testdata/java-comments/JavaComments.java`,
  whose spans sit beside each fixture in `<fixture>.javac`;
- Roslyn, published as `Microsoft.CodeAnalysis.CSharp`, for C#, run by the
  program in `testdata/csharp-comments/`, which lexes again the regions its
  preprocessor sets aside, and whose spans sit beside each fixture in
  `<fixture>.roslyn`;
- libclang's lexer for C and C++, reached through its C API by
  `testdata/clang-goldens.py`, whose spans sit beside each fixture in
  `<fixture>.clang`, and Ripper, the lexer Ruby's own parser is built on, for
  Ruby, run by `testdata/ruby-goldens.rb`, whose spans sit beside each fixture in
  `<fixture>.ripper`. libclang lexes raw tokens, so it reads `//` inside
  `#include <a//b.h>` as a comment where the preprocessor reads a header name;
  no fixture holds one;
- the `mvdan.cc/sh` parser for Bash and the `tdewolff/parse` CSS lexer for CSS,
  both run by the Go tests themselves.

A golden records its fixture's sha256, so the Go tests need none of node, Python,
Ruby, cargo, a JDK, the .NET SDK or libclang, and fail when a fixture changes without its golden. Grouping and
directives in the oracle come from rules written in `oracle_test.go`, apart from
the provider's.

The corpus is drawn from this repository, plus authored fixtures for directives,
literals, subjects, generated files, doc comments, CRLF line endings and
multibyte text. Rust, C# and C++ have no source in this repository, so their
fixtures are authored; Java's are authored or drawn from okapi-bridge, and C's
from `core/storage`. A copied fixture is named for the last two segments of its path.
From the repository root, with node, Python 3, Ruby, cargo, a JDK, the .NET SDK, libclang and the pnpm
store installed:

```bash
node plugins/sourcecode/internal/comments/testdata/babel-goldens.mjs --add tsx web/src/theme/Root.tsx
python3 plugins/sourcecode/internal/comments/testdata/python-goldens.py --add scripts/make-sample-docx.py
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
