# kapi-sourcecode

Reads the prose out of source files — product strings, and on request comments —
using tree-sitter grammars.

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

Ruby today. Each grammar needs its prose-bearing node kinds mapped in
`internal/proseread`; the walk itself is language-independent.

One subtlety worth knowing before adding a language: a heredoc body is not a
child of the call that opens it. `caveats <<~EOS` puts a `heredoc_beginning`
under the call and parks the body at the top level, so attributing by parent
alone credits the text to the enclosing block. Bodies appear in the same order
as their openers, which is what makes the pairing safe.

## Build and check

```bash
make build-sourcecode-plugin    # → bin/kapi-sourcecode
make test-sourcecode-plugin
bin/kapi-sourcecode doctor      # confirms the grammars load AND still separate prose
```

cgo, but no system dependency: the grammars are vendored C compiled by the Go
build, so unlike `kapi-pdfium` this needs nothing on `PKG_CONFIG_PATH`. It still
runs as a subprocess, so a parser fault on a malformed file stays in the plugin.
