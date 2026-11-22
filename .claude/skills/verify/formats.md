# Verify a change to a format

For a diff under `core/formats/` or `plugins/`. Verified means the built binary
reads and writes real files of the format, and the package's round trip and
fuzz targets pass. Work from the checkout root, with the sandbox from
[cli.md](cli.md):

```bash
make build
S=.claude/skills/verify/scripts/kapi-sandbox.sh
$S init
SB=$($S path); mkdir -p "$SB/rt"
```

`<fmt>` below is the package under `core/formats/`. Use a fixture from its
`testdata/` that holds the construct you changed, by absolute path.

## Package tests

If `go` cannot find ICU, export `PKG_CONFIG_PATH` as [cli.md](cli.md) shows.

```bash
go test -count=1 ./core/formats/<fmt>/          # round trip and extraction: 1 to 5 s
go test -count=1 -tags fts5 ./core/formats/     # cross-format matrix and fidelity: 8 s
```

## An unedited round trip is byte-identical

A same-format `kconv` runs the reader, the skeleton and the writer. `--to`
takes the format's extension:

```bash
M=$PWD/core/formats/markdown/testdata/formatting.md
$S run kconv "$M" --to md -o "$SB/rt/out.md"
cmp "$M" "$SB/rt/out.md" && echo identical
```

`kconv --to json` and `--to yaml` write a block listing instead of the catalog.
For those, and for any format, edit one value and edit it back. The second file
must match the fixture byte for byte, and the first must differ in one place:

```bash
F=$PWD/core/formats/json/testdata/simple.json
$S run ksed 's/Hello World/Hello Xorld/' "$F" > "$SB/rt/a.json"
$S run ksed 's/Hello Xorld/Hello World/' "$SB/rt/a.json" > "$SB/rt/b.json"
cmp "$F" "$SB/rt/b.json" && echo identical
diff "$F" "$SB/rt/a.json"     # one changed line: the edited value
```

Any other changed line (reordered keys, reflowed text, a lost final newline) is
a writer defect. `ksed --target <locale>` edits the translation in a bilingual
file (PO, XLIFF, string catalogs) the same way.

## The writer keeps placeholders and paired codes

A writer renders runs with `model.RenderRunsWithData`. `TargetText`,
`SourceText` and `RunsText` drop every placeholder and paired code:

```bash
git diff "$(git merge-base origin/main HEAD)" -- core/formats/<fmt>/ | grep -nE 'TargetText|SourceText|RunsText'
```

Pseudo-translation writes a target for every block through the writer, and a
`ksed` edit inside a link must keep the link:

```bash
H=$PWD/core/formats/html/testdata/inline_codes.html
$S run pseudo-translate "$H" --target-lang qps -o "$SB/qps"
cat "$SB/qps/inline_codes.html"   # <b>ĥéŕé</b> ƒöŕ <a href="http://example.com">ḿöŕé îñƒö</a>
$S run ksed 's/more info/more details/' "$H"   # <a href="http://example.com">more details</a>
```

## Malformed input

Where the format can recover (PO, Markdown, HTML, line formats), one bad entry
must not lose the rest of the file:

```bash
printf 'msgid "a"\nmsgstr "b"\n\nmsgid "broken\nmsgstr "x"\n\nmsgid "c"\nmsgstr "d"\n' > "$SB/rt/bad.po"
$S run kcat -n "$SB/rt/bad.po"; echo "exit=$?"    # a and c still read, exit=0
```

A format that cannot recover (JSON, XML) must fail with an error that names the
problem. A panic, or exit 0 with content missing, is a defect.

## Fuzz for a bounded time

```bash
grep -n '^func Fuzz' core/formats/<fmt>/*_test.go
go test -run '^$' -fuzz '^FuzzRoundTripPo$' -fuzztime 30s ./core/formats/po/   # 34 s
```

A crasher lands in `core/formats/<fmt>/testdata/fuzz/<Target>/<id>`, and the
failure prints how to rerun it. Rerun it with your diff reverted. If it fails
there too, it predates your change: report it to the person and delete the
file. If your diff caused it, fix the cause and commit the file with the fix.

## Consumer toolchain acceptance

```bash
make format-acceptance     # 17 s; each check skips when its tool is absent
```

It covers only the packages its Makefile target lists. Add yours there when you
give it an acceptance suite.

## Plugin formats

kapi reaches a plugin format only through discovery. Build the plugin and
install it into the sandbox after `init`, which empties the plugins directory:

```bash
make build-sourcecode-plugin && make test-sourcecode-plugin    # 2 s and 4 s warm
P="$($S path)/plugins/sourcecode"; mkdir -p "$P"
/bin/cp -f bin/kapi-sourcecode plugins/sourcecode/manifest.json "$P/"
/bin/cp -Rf plugins/sourcecode/formats "$P/"
$S run plugin list             # sourcecode ... $KAPI_PLUGINS_DIR  active
$S run kcat -n "$PWD/plugins/sourcecode/internal/proseread/testdata/kapi-desktop.rb"
```

The directory name must equal the manifest's `plugin` field, and the binary and
each `schema` path the manifest names must sit inside it. `make help` lists the
`build-*-plugin` targets. `kapi-pdfium` also needs libpdfium on
`PKG_CONFIG_PATH` (`plugins/pdfium/README.md`); this recipe has not run it.

## Evidence

Record the `cmp` and `diff` results, the written line that shows the codes
kept, the malformed-input output with its exit code, and the fuzz summary line
(`elapsed: 31s, execs: ...`, then `PASS`). Finish with `$S clean`.
