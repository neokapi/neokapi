# Corpus reality + release-asset publish/fetch pattern (neokapi)

Repo root analyzed: `/Users/asgeirf/src/neokapi/neokapi/.claude/worktrees/format-process`
(all paths below are relative to that root unless absolute).

---

## 1. Per-format testdata inventory

Totals: **207 files, 3.0 MB** across `core/formats/*/testdata` (`du -shc` = 3.0M).
56 entries under `core/formats/` = 52 format dirs + 4 files (`ids.go`, `register.go`,
`register_test.go`, `maturity_test.go`). `memorytest/` is a test-only package; `exec/`
and `jsx/` have no testdata dir.

### Raw inventory (files | size | top extensions)

| format | files | size | extensions |
|---|---|---|---|
| androidxml | 6 | 64KB | xml:5 md:1 |
| applestrings | 6 | 88KB | stringsdict:3 strings:2 md:1 |
| arb | 9 | 220KB | arb:6 md:2 json:1 |
| csv | 1 | 4KB | csv:1 |
| designtokens | 25 | 124KB | json:22 md:2 tokens:1 |
| doxygen | 4 | 16KB | h:4 |
| dtd | 4 | 16KB | dtd:4 |
| epub | 1 | 4KB | epub:1 |
| exec | 0 | — | NONE |
| fixedwidth | 1 | 4KB | dat:1 |
| html | 2 | 8KB | html:2 |
| i18next | 14 | 56KB | json:13 md:1 |
| icml | 4 | 16KB | icml:3 wcml:1 |
| idml | 21 | 444KB | idml:20 md:1 |
| json | 1 | 4KB | json:1 |
| jsx | 0 | — | NONE (inline spec.yaml snippets only) |
| markdown | 1 | 4KB | md:1 |
| mdx | 25 | 132KB | mdx:24 md:1 |
| memorytest | 0 | — | NONE (test-only package) |
| messageformat | 1 | 4KB | mf:1 |
| mif | 8 | 788KB | mif:7 md:1 |
| mo | 0 | — | NONE |
| mosestext | 2 | 8KB | txt:2 |
| odf | 0 | — | NONE (relies on okapi-testdata entirely) |
| openxml | 4 | 40KB | docx:3 xlsx:1 |
| paraplaintext | 1 | 4KB | txt:1 |
| pdf | 8 | 664KB | pdf:8 |
| phpcontent | 2 | 8KB | php:2 |
| plaintext | 4 | 12KB | txt:4 |
| po | 1 | 4KB | po:1 |
| properties | 1 | 4KB | properties:1 |
| regex | 4 | 16KB | txt:2 strings:1 ini:1 |
| resx | 8 | 68KB | resx:6 resw:1 md:1 |
| rtf | 4 | 84KB | rtf:4 |
| splicedlines | 1 | 4KB | txt:1 |
| srt | 1 | 4KB | srt:1 |
| tex | 6 | 24KB | tex:6 |
| tmx | 1 | 4KB | tmx:1 |
| transtable | 0 | — | NONE (okapi: spec inputs) |
| ts | 3 | 12KB | ts:3 |
| ttml | 3 | 12KB | ttml:3 |
| ttx | 0 | — | NONE |
| txml | 0 | — | NONE (okapi: spec inputs) |
| versifiedtext | 1 | 4KB | ver:1 |
| vignette | 0 | — | NONE (okapi: spec inputs) |
| vtt | 1 | 4KB | vtt:1 |
| wiki | 3 | 12KB | wiki:2 txt:1 |
| xcstrings | 11 | 84KB | xcstrings:8 md:2 json:1 |
| xliff | 1 | 4KB | xlf:1 |
| xliff2 | 0 | — | NONE (okapi-testdata byte-equal corpus) |
| xml | 1 | 4KB | xml:1 |
| yaml | 1 | 4KB | yaml:1 |

### Vendored real-world corpora (`testdata/corpus/` + `SOURCES.md`)

**10 formats** ship a vendored, provenance-pinned corpus dir (each with a `SOURCES.md`):
`androidxml, applestrings, arb, designtokens, i18next, idml, mdx, mif, resx, xcstrings`
(`ls core/formats/*/testdata/corpus/SOURCES.md | wc -l` = 10).

`SOURCES.md` is the de-facto per-format corpus manifest already in use. E.g.
`core/formats/arb/testdata/corpus/SOURCES.md` records, per file: source repo URL,
**license SPDX id**, branch, **pinned commit**, upstream paths, feature-coverage notes,
and **commit-pinned re-fetch commands**:

```sh
curl -sSL -o flutter_gallery_intl_en.arb \
  "https://raw.githubusercontent.com/flutter/gallery/66a69803cc63dfc02878fae1959a2555f26ea25f/lib/l10n/intl_en.arb"
```

`core/formats/idml/testdata/corpus/SOURCES.md` uses a provenance **table**
(`File | Upstream repo | License | Commit`) and states the round-trip contract
("semantic, not byte-exact" for IDML; ARB asserts byte-faithful). **What no SOURCES.md
carries today: sha256 digests** — files are "byte-identical to source" by assertion only.

`corpus_test.go` exists for exactly those 10 formats; `upstream_test.go` (driven off the
external okapi-testdata tree) exists for `csv, icml, idml, odf`.

### The external okapi-testdata corpus (the existing fetch-corpus precedent)

There is already a fetched, gitignored, versioned external corpus:

- **Fetcher:** `scripts/fetch-okapi-testdata.sh` — downloads asset `okapi-testdata.tar.gz`
  from the GitHub release **tag `okapi-testdata-1.48.0` on repo `neokapi/okapi-bridge`**
  (not this repo), extracts to `okapi-testdata/<TESTDATA_VERSION>/` (default `1.48.0-v4`)
  at the repo root. Idempotent: "Skip if already present and not forced"
  (`fetch-okapi-testdata.sh:39-42`), `FORCE_FETCH=1` to re-download, `OKAPI_TESTDATA_TAG`
  / `TESTDATA_VERSION` overrides. The **local dir version is suffixed `-vN`** so content
  fixes under the same upstream tag bust the cache without FORCE_FETCH. Downloads via the
  GitHub API asset URL + `Accept: application/octet-stream` (browser download URLs don't
  reliably follow CDN redirects when authenticated; `fetch-okapi-testdata.sh:59-98`),
  with optional `GITHUB_TOKEN` passed via a header **file** (not argv) to avoid leaking.
- **gitignore:** `.gitignore:34` → `okapi-testdata/`.
- **Resolution at test time:** `core/format/spec/helpers.go:60-90`
  `FindOkapiTestdataRoot()` walks up from cwd to `go.work`, then picks the
  **lexically-latest version dir** under `okapi-testdata/`. Error message tells you the
  fetch command: `"okapi-testdata not found at %s — run scripts/fetch-okapi-testdata.sh"`.
- **`okapi:` scheme:** `core/format/spec/helpers.go:42-49` —
  `if strings.HasPrefix(rel, "okapi:")` → join under the testdata root. Used by
  `spec.ResolveInput` / `core/format/spectest/runner.go:66`.
- **Skip-not-fail:** tests that need it skip cleanly, e.g.
  `core/formats/wiki/reader_test.go:916-919`
  (`t.Skipf("okapi-testdata not available: %v", err)`), and
  `core/formats/openxml/native_test.go:438`
  (`t.Skip("okapi-testdata/ not found — run scripts/fetch-okapi-testdata.sh")`).
- **Second consumer:** `scripts/parity-sandbox.sh:88-105` curls the same tarball from the
  plain release-download URL into `.parity/okapi-testdata/<okapi_version>/` (warns, does
  not fail, on miss; publisher is okapi-bridge's `scripts/publish-okapi-testdata.sh`).
  CI gets it via `make parity-test → parity-sandbox` (`.github/workflows/parity.yml:120`).
- Packages referencing okapi-testdata: csv, epub, fixedwidth, html, icml, idml, mif, odf,
  openxml, paraplaintext, properties, transtable, ttml, txml, vignette, wiki, xliff2, xml.

### cli/parity fixtures (`fixtures_*_generated.go`)

12 files in `cli/parity/formats/`: `dtd, html, json, markdown, po, properties, regex,
tmx, ts, wiki, xliff, yaml`. Each is build-tagged `//go:build parity`, header
`// Code generated by scripts/okapi-test-scan. DO NOT EDIT.`, and holds
`[]FormatInput{ {Name: "gen-testX", Content: ttext(`…`), OkapiTest: "Class#method",
Informational: true}, … }` — i.e. **inline string snippets regex-scraped from upstream
Okapi Java `@Test` methods**, not files. Hand-curated fixtures in `spec.go` remain
authoritative (`scripts/okapi-test-scan/main.go:1-34`; scanner is intentionally lossy,
skips ~10-15% of tests that load resources or build inputs programmatically).

### spec.yaml `input_file: okapi:` usage

- 41 `spec.yaml` files total under `core/formats/`.
- **41 occurrences** of `input_file: okapi:` across **15 spec.yamls**:
  odf:10, idml:9, csv:5, fixedwidth:3, vignette:2, transtable:2, openxml:2,
  wiki:1, txml:1, ttml:1, pdf:1, paraplaintext:1, mif:1, icml:1, epub:1.

---

## 2. Acceptance tests (`//go:build acceptance`)

**8 formats** ship acceptance suites (11 files): `androidxml, applestrings, arb,
designtokens, i18next, mdx, resx, xcstrings` (helpers `acceptance_helpers_test.go` in
applestrings/arb/xcstrings). They run the **translated output** (pseudo-translate via the
writer's splice path) through real downstream consumers, over `testdata/corpus/*` files
(`core/formats/androidxml/acceptance_test.go:55-60` globs `testdata/corpus/*.xml`).

External validators:

| format | validators |
|---|---|
| androidxml | `xmllint --noout`; `aapt2 compile` (self-skips); structural re-parse (no tool) |
| applestrings | `plutil` (macOS) |
| arb, xcstrings | `jq` or ajv runner (`ajv`/`npx`) — skips if **neither** present |
| designtokens, i18next | `ajv` (ajv-cli@5; falls back to `corepack pnpm dlx ajv-cli@5`) |
| mdx | node + `@mdx-js/mdx` compile (skips when not installed/offline) |
| resx | `resgen` (mono, the real .resx consumer); `xmllint` |

Tool-absence skip pattern (the canonical quote, `core/formats/androidxml/acceptance_test.go:64-68`):

```go
xmllint, err := exec.LookPath("xmllint")
if err != nil {
    t.Skip("xmllint not found on PATH — skipping well-formedness acceptance check")
}
```

Shared helper variant (`core/formats/i18next/acceptance_test.go:43-50`):
`lookPath(t, bin)` → `t.Skipf("%s not on PATH; skipping consumer-acceptance check", bin)`.
Network-failure tolerance: `isLikelyOffline(output)` (i18next acceptance_test.go:76+)
matches `"network"/"enotfound"/"registry.npmjs"/…` so a failed `pnpm dlx` provisioning
skips instead of failing. ajv preference order (`ajvCommand()`, i18next:68-74): real
`ajv` on PATH first, else `corepack pnpm dlx ajv-cli@5`.

**Make target:** `Makefile:141` `format-acceptance:` runs
`NODE_OPTIONS= $(GO) test -tags acceptance -count=1` over an **explicit package list**
(the 8 formats) — deliberately not `./core/formats/...` ("would also pull in … xliff2's
okapi byte-equal corpus"). `NODE_OPTIONS` is cleared so spawned node tooling doesn't
inherit CI flags.

**CI:** `.github/workflows/format-acceptance.yml` — `macos-latest` (plutil is macOS-only),
path-triggered on `core/formats/**`; installs ajv-cli globally via corepack pnpm
(best-effort `|| true`), `brew install mono || true` for resgen, prints a
"Report available validators" step, then `make format-acceptance`. Design principle
(header, lines 3-6): *"if a validator is absent OR cannot execute, the subtest skips; it
fails only when the validator runs and rejects kapi output."*

---

## 3. The docs-assets release pattern (template to replicate)

### Makefile targets (`Makefile:1083-1106`)

```make
fetch-docs-assets:
	@gh release download docs-assets --pattern 'docs-assets.tar.gz' --dir /tmp --clobber
	@mkdir -p web/static
	@tar xzf /tmp/docs-assets.tar.gz -C web/static
	@rm -f /tmp/docs-assets.tar.gz
	@du -sh web/static/img web/static/video 2>/dev/null || true

publish-docs-assets:
	@bash scripts/publish-docs-assets.sh
```

`fetch-bowrain-docs-assets` / `publish-bowrain-docs-assets` are byte-symmetric with
`bowrain-docs-assets` tag and `bowrain/web/docs/static` (verified by diff). Both pairs are
listed `.PHONY` (`Makefile:1364-1366`). `publish-website` (`Makefile:1250`) depends on
both fetch targets before the prod docs builds; `docs-build-prod` (`Makefile:1240`)
documents "run fetch-docs-assets first to stage videos".

### Release addressing

- **By tag name only**: `gh release download docs-assets …` / `gh release upload docs-assets …`
  — `gh` infers the repo from the current git remote (`neokapi/neokapi`); no `-R` flag,
  no version in the tag. The release is a **mutable singleton bucket**, one asset
  (`docs-assets.tar.gz`), re-uploaded with `--clobber`.
- Contrast: the okapi-testdata corpus uses **versioned tags** (`okapi-testdata-1.48.0`)
  on a different repo + versioned local dirs — the better model for a corpus (see §5).

### Merge-never-drop publish (`scripts/publish-docs-assets.sh`)

Algorithm (lines 22-58):
1. `mktemp -d` workdir, `trap rm -rf EXIT`.
2. Download current `docs-assets.tar.gz` from the release; extract into workdir
   ("base extracted (existing assets preserved)"); tolerate absence ("no existing
   tarball — creating a fresh one").
3. **Overlay** local `web/static/{img,video}` onto the base:
   `rsync -a "$STATIC/$d/" "$WORK/$d/"` — *"additive: never deletes"*, local wins on
   conflict, files only published if non-empty (`find … -print -quit` guard, line 36).
4. Re-pack `tar czf` of whichever of `img/ video/` exist; error if neither.
5. `gh release upload "$RELEASE" "$WORK/$TARBALL" --clobber`.

The bowrain variant adds **first-publish auto-create**
(`scripts/publish-bowrain-docs-assets.sh:34-41`):

```sh
if ! gh release view "$RELEASE" >/dev/null 2>&1; then
  gh release create "$RELEASE" \
    --title "bowrain docs assets" \
    --notes "… Managed by scripts/publish-bowrain-docs-assets.sh — do not edit by hand." \
    --latest=false
fi
```

(`--latest=false` keeps the asset bucket from masquerading as a product release.)
The kapi script lacks this create step (release pre-exists) — replicate the bowrain form.

### Where fetched files land + .gitignore

Fetch untars into `web/static/` → `web/static/img/…`, `web/static/video/…`.
`.gitignore:120-131`:

```
# Generated documentation assets (fetched from docs-assets release in CI)
web/static/img/bowrain/
web/static/img/web-app/
web/static/video/
…
bowrain/web/docs/static/img/bowrain/
bowrain/web/docs/static/img/web-app/
bowrain/web/docs/static/video/
```

(Note: only the *generated* subtrees are ignored; hand-authored img content stays in git.)

### CI staging (`.github/workflows/docs-kapi.yml:96-110`)

```yaml
- name: Stage docs videos from docs-assets
  env:
    GH_TOKEN: ${{ github.token }}
  run: |
    mkdir -p web/static/video
    if gh release download docs-assets -p docs-assets.tar.gz -D /tmp/da 2>/dev/null; then
      tar xzf /tmp/da/docs-assets.tar.gz -C /tmp/da
      [ -d /tmp/da/video ] && cp -R /tmp/da/video/. web/static/video/
      echo "Staged docs videos: $(find web/static/video -name '*.webm' …) webm"
    else
      echo "::warning::docs-assets release unavailable; docs videos will be missing"
    fi
```

Key properties: `GH_TOKEN: ${{ github.token }}` suffices (same-repo release, read-only
`permissions: contents: read`); **best-effort with `::warning` degradation**, never a
hard CI failure; CI **never records/produces** assets — comment at lines 96-98: *"All
docs videos are produced by the harness on the desktop … CI never records; it stages
them here."* `docs-bowrain.yml:45-60` is symmetric.

---

## 4. `make regen-okapi-fixtures` end-to-end (`Makefile:540-572`)

1. **Inputs:** a local clone of upstream Okapi Java at `OKAPI_REPO`
   (default `/Users/asgeirf/src/okapi/Okapi`, `Makefile:598`); guard:
   `@[ -d "$(OKAPI_REPO)/okapi/filters" ] || { echo "OKAPI_REPO not found…"; exit 1; }`.
2. **Spec table:** `OKAPI_FIXTURE_SPECS` (`Makefile:547-559`) maps 12 formats to Java
   test classes, e.g. `html=HtmlConfigurationSupportTest,HtmlEventTest,HtmlSnippetsTest,…`
   — *"the class lists below are the source of truth … keep them in committed order so
   regeneration stays byte-stable"*.
3. **Per format:** `go run ./scripts/okapi-test-scan -src $(OKAPI_REPO)/okapi/filters
   -class "$classes" -package formats -out cli/parity/formats/fixtures_<fmt>_generated.go`.
   The scanner regex-walks Java `@Test` methods, recovers `String snippet = "…"`-shaped
   inline inputs (incl. concatenations), emits a `//go:build parity` Go file of
   `FormatInput{Name, Content, OkapiTest, Informational:true}`.
4. **Lifecycle:** *NOT* regenerated by `make test`; **no `//go:generate`** (source path is
   machine-specific); refresh after bumping `OKAPI_VERSION` or when upstream adds tests;
   finishes with *"review 'git diff cli/parity/formats/fixtures_*_generated.go'"* —
   generated output is **committed** and reviewed, unlike okapi-testdata (fetched) or
   docs-assets (released).

---

## 5. Conclusion — mechanics for `make fetch-corpus` / `publish-corpus`

The repo already has all three storage idioms; a format corpus store should compose them:

1. **Release addressing — versioned tag, this repo.** Follow okapi-testdata's tag scheme,
   not docs-assets' mutable singleton: tag `format-corpus-vN` (or per-format
   `corpus-<format>-vN`), asset `format-corpus.tar.gz` (or `corpus-<format>.tar.gz` for
   per-format granularity — large binaries like idml/mif/pdf argue for per-format assets
   in ONE release so a format bump doesn't re-ship 3 MB+). Created on first publish with
   the bowrain guard: `gh release view || gh release create "$TAG" --latest=false --notes
   "Managed by scripts/publish-corpus.sh — do not edit by hand."`. `gh` addresses by tag
   in the repo from the git remote — no `-R` needed.
2. **fetch-corpus** = clone of `scripts/fetch-okapi-testdata.sh` mechanics:
   extract to a **versioned, gitignored dir** (`corpus/<version>/<format>/…`, add
   `corpus/` next to `.gitignore:34`'s `okapi-testdata/`); idempotent skip when the
   version dir exists; `FORCE_FETCH=1`; `-vN` local-dir suffix for content respins under
   the same upstream snapshot; GitHub API asset-URL + `Accept: application/octet-stream`
   download with `GITHUB_TOKEN` via header file. Resolution helper mirrors
   `spec.FindOkapiTestdataRoot` (`core/format/spec/helpers.go:60-90`): walk up to
   `go.work`, pick latest version dir; introduce a `corpus:` input_file scheme alongside
   `okapi:` in `ResolveFilePath` (`helpers.go:42-49`). Tests **skip with the fetch
   command in the message** (wiki/openxml pattern), never fail on absence.
3. **publish-corpus** = clone of `scripts/publish-docs-assets.sh` merge-never-drop:
   download current tarball → extract to mktemp → `rsync -a` local trees over it
   (additive, local wins) → repack → `gh release upload --clobber`. This lets per-format
   contributions land independently without clobbering other formats' corpus files.
4. **Per-format corpus manifest** — promote the existing `SOURCES.md` convention
   (arb/idml are the best exemplars) to a machine-readable
   `corpus/<format>/manifest.yaml` carrying per file: `source_repo`, `source_path`,
   `commit` (pinned), `license` (SPDX id, "confirmed against the repo's LICENSE at the
   pinned commit" — idml wording), `fetch_cmd` (commit-pinned curl, arb pattern),
   `roundtrip_contract` (`byte-exact` | `semantic` — arb vs idml distinction), plus the
   one field nothing has today: **`sha256`**. The publish script should verify sha256
   before packing; `corpus_test.go` / fetch script verify after extraction. Keep a
   human `SOURCES.md` view or generate it from the manifest.
5. **CI staging**: a best-effort step in the relevant workflow(s) exactly like
   `docs-kapi.yml:99-110` (`GH_TOKEN: ${{ github.token }}`, `::warning` on miss,
   `contents: read`), plus the parity job already fetches okapi-testdata implicitly via
   `make parity-test → parity-sandbox` (`parity-sandbox.sh:88-105`) — wire fetch-corpus
   the same way (a make prerequisite, not a bespoke workflow step, where possible).
6. **What stays in git** vs release: tiny synthetic fixtures and the 3.0 MB of current
   testdata stay vendored (they gate `make test` and must work offline); the corpus
   release is for **growth** — large/real files (more idml/mif/pdf/openxml class
   binaries) and license-cleared real-world catalogs beyond what's reasonable to vendor.

## 6. Per-format corpus-status table

Classes: **real-corpus** = vendored `testdata/corpus/` + SOURCES.md provenance;
**some-real** = real upstream files reachable (okapi-testdata refs and/or `input_file:
okapi:` spec examples, or a few vendored real files without a corpus dir);
**synthetic** = hand-written fixtures only; **none** = no file fixtures at all.
(Cross-checked against the maturity dashboard's `corpus` dimension in
`web/static/data/format-maturity.json`: complete/partial/none.)

| format | class | evidence |
|---|---|---|
| androidxml | real-corpus | corpus/ (AOSP calendar, K-9 Mail) + SOURCES.md; acceptance suite |
| applestrings | real-corpus | corpus/ (playem, utm) + SOURCES.md; acceptance |
| arb | real-corpus | corpus/ (flutter/gallery, commit-pinned, BSD-3) + SOURCES.md; acceptance |
| designtokens | real-corpus | corpus/ (style-dictionary demo) + SOURCES.md; acceptance |
| i18next | real-corpus | corpus/ (react-i18next, typescript ns) + SOURCES.md; acceptance |
| idml | real-corpus | corpus/ 11 files (SimpleIDML/BatchIDML/Okapi) + SOURCES.md table; also okapi: ×9 |
| mdx | real-corpus | corpus/ 17 real docs-site pages + SOURCES.md; acceptance |
| mif | real-corpus | corpus/ 6 okapi-* files + SOURCES.md; also okapi: ×1 |
| resx | real-corpus | corpus/ (roslyn, stylecop) + SOURCES.md; acceptance |
| xcstrings | real-corpus | corpus/ (xckit, zeitgeist) + SOURCES.md; acceptance |
| csv | some-real | upstream_test.go + okapi: ×5 (1 local synthetic csv) |
| epub | some-real | okapi: ×1; 1 local synthetic minimal.epub |
| fixedwidth | some-real | okapi: ×3; 1 local .dat |
| html | some-real | okapi-testdata refs in tests; 2 local synthetic |
| icml | some-real | upstream_test.go + okapi: ×1; 4 local files |
| odf | some-real | upstream_test.go + okapi: ×10; zero local testdata |
| openxml | some-real | okapi: ×2 + native_test.go:438 okapi-testdata skip; 4 vendored upstream-named files (test_859.docx, EksempelFiltrering.xlsx) |
| paraplaintext | some-real | okapi: ×1; 1 local txt |
| pdf | some-real | okapi: ×1; 1 real vendored PDF (TAUS-QualityDashboard) among 8 |
| properties | some-real | okapi-testdata test refs; 1 local synthetic |
| transtable | some-real | okapi: ×2; zero local testdata |
| ttml | some-real | okapi: ×1; 3 local synthetic |
| txml | some-real | okapi: ×1; zero local testdata |
| vignette | some-real | okapi: ×2; zero local testdata |
| wiki | some-real | okapi-testdata upstream DokuWiki fixture test (reader_test.go:905+); okapi: ×1 |
| xliff2 | some-real | okapi-testdata byte-equal corpus (zero local testdata) |
| xml | some-real | okapi-testdata via presets_test.go:78; 1 local synthetic |
| doxygen | synthetic | 4 .h fixtures only (dashboard: partial) |
| dtd | synthetic | 4 local .dtd; parity covers via generated inline snippets (dashboard oddly: complete) |
| json | synthetic | 1 local json; parity generated snippets |
| jsx | synthetic | no testdata; inline spec.yaml snippets |
| markdown | synthetic | 1 local md; parity generated snippets |
| messageformat | synthetic | 1 local .mf |
| mosestext | synthetic | 2 local txt |
| phpcontent | synthetic | 2 local php |
| plaintext | synthetic | 4 local txt |
| po | synthetic | 1 local po; parity generated snippets |
| regex | synthetic | 4 local; parity generated snippets |
| rtf | synthetic | 4 local (Test01/02/AddComments — Okapi-style names, no provenance recorded) |
| splicedlines | synthetic | 1 local txt |
| srt | synthetic | 1 local srt |
| tex | synthetic | 6 local tex |
| tmx | synthetic | 1 local tmx; parity generated snippets |
| ts | synthetic | 3 local ts; parity generated snippets |
| versifiedtext | synthetic | 1 local .ver |
| vtt | synthetic | 1 local vtt |
| wiki(local) | — | counted under some-real above |
| xliff | synthetic | 1 local .xlf; parity generated snippets (dashboard: partial despite L3) |
| yaml | synthetic | 1 local yaml; parity generated snippets |
| exec | none | no testdata (reads process output; arguably N/A) |
| memorytest | none | test-only package, N/A |
| mo | none | zero fixtures (dashboard: none) |
| ttx | none | zero fixtures (dashboard: none) |

Rollup: **10 real-corpus / 17 some-real / 21 synthetic-only / 4 none** — consistent with
`docs/internals/format-maturity.md:232` ("Synthetic-only corpus | 25/48 | vendor/upstream
real files (or `spec.yaml input_file: okapi:…`)") given that table predates a few corpus
landings. The maturity rubric (format-maturity.md:76-78, 99) makes corpus breadth an L3
requirement ("a corpus/upstream test exercises real files") and L4 requires byte-faithful
round-trip "over a real-world corpus" — so fetch-corpus/publish-corpus directly serves
the L3→L4 burndown for the 21 synthetic + 4 none formats.
