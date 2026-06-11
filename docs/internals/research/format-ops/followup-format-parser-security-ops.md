# Security engineering for file-format parsers & operational infrastructure for mass-processing hostile document corpora

Research stream for neokapi's format-ops framework. Date: 2026-06-11. neokapi = open-source Go localization engine, ~49 native format readers/writers, three trust contexts (CLI, multi-tenant SaaS `bowrain`, cgo-less browser WASM), solo maintainer + Claude agents, planning nightly wild/hostile-corpus sweeps.

Every load-bearing claim below carries a URL. Where a source is a vendor blog or aggregator, a primary source (spec, repo, CVE, official docs) is preferred and cited.

---

## Findings

### 1. The governing principle: Chromium's "Rule of 2" and where neokapi sits

Chromium's security team codified the canonical mental model for parser security: **do not combine all three of (a) untrustworthy input, (b) an unsafe language, and (c) high privilege; drop at least one.** "The Chrome Security Team will generally not approve landing a code change or new feature that involves all 3 of untrustworthy inputs, unsafe language, and high privilege. To solve this problem, you need to get rid of at least 1 of those 3 things." Memory-safe languages explicitly named safe include "Go, Rust, Python, Java, JavaScript, Kotlin, and Swift." (https://chromium.googlesource.com/chromium/src/+/master/docs/security/rule-of-2.md)

**Implication for neokapi:** Go's memory safety already removes leg (b) for the pure-Go parsers — the dominant risk class (RCE via memory corruption) is largely off the table for native readers. This is the framework's structural advantage and should be stated explicitly. The residual risk classes are: **algorithmic/resource DoS** (CPU, memory, disk, stack exhaustion, decompression bombs), **path traversal / arbitrary file write on extraction**, **SSRF / external fetch**, **round-trip semantic corruption**, **output-injection (XSS) when parsed content is re-rendered**, and the **cgo/native exception** (okapi-bridge JVM, any cgo: ICU, onnxruntime, SQLite — these re-introduce leg (b)). Kelly Shortridge's restatement ("SUX rule") is a useful framing for docs. (https://kellyshortridge.com/blog/posts/the-sux-rule-for-safer-code/)

### 2. Zip-container family (OOXML docx/xlsx/pptx, EPUB, IDML, any zip)

**Decompression / zip bombs.** DEFLATE caps at a ~1032:1 ratio per layer, so classic bombs nest archives (×1032 per layer) — but modern non-recursive bombs overlap entries and exploit Zip64 to hit millions:1, reaching "281 TB from a 10 MB payload and even 4.5 PB when Zip64 is enabled" with a single decompression pass. (https://www.bamsoftware.com/hacks/zipbomb/ ; https://en.wikipedia.org/wiki/Zip_bomb)

**Canonical mitigation = Apache POI's `ZipSecureFile`.** Two global knobs: `setMinInflateRatio()` defaults to **0.01 (1%)** — "when the compression is better than 1% for any given read package part, the parsing will fail indicating a Zip-Bomb"; and `setMaxEntrySize()` defaults to **4 GiB** (the 32-bit zip max) per single entry. These are checked *per entry, on streaming read*, not after full extraction. (https://poi.apache.org/apidocs/dev/org/apache/poi/openxml4j/util/ZipSecureFile.html) Note this is imperfect: legitimate high-compression files trigger false positives (POI bug reports of "ZIP BOMB DETECTED" on real docs) — limits must be tunable per format. (https://www.mail-archive.com/dev@poi.apache.org/msg42358.html)

**A 2026 CVE shows the exact mistake to avoid.** `file-type` (npm) `CVE-2026-32630`: probing ZIP-based formats such as OOXML inflated `[Content_Types].xml` — "a ZIP of about 255 KB caused about 257 MB of RSS growth during fileTypeFromBuffer()." Root cause: **different size-limit logic applied to different input types** (stream path bounded `maximumZipEntrySizeInBytes` to 1 MiB; buffer path did not). Lesson: apply identical limits across all entry points (CLI file, server upload, WASM buffer). (https://github.com/advisories/GHSA-j47w-4g3g-c36v)

**Zip-slip path traversal on extraction.** Archive entry names can contain `../` or absolute paths; naive `filepath.Join(dest, entry.Name)` writes outside the destination. Go's own fix path: `path/filepath.IsLocal` (Go 1.20) "reports whether a path is local … `../etc/passwd` is not allowed … `/etc/passwd` is not allowed," and `filepath.Localize` (Go 1.23). The Go blog "Traversal-resistant file APIs" recommends these plus `os.Root` (Go 1.24) for confined extraction. (https://go.dev/blog/osroot ; https://codeql.github.com/codeql-query-help/go/go-zipslip/) `github.com/cyphar/filepath-securejoin` is the battle-tested helper (use ≥0.2.4 — it had its own traversal CVE). (https://security.snyk.io/vuln/SNYK-GOLANG-GITHUBCOMCYPHARFILEPATHSECUREJOIN-5889602)

**Operational note:** neokapi mostly reads OOXML *in place* (zip entries → XML parts) rather than extracting to disk, which sidesteps zip-slip for the common path — but any `--extract-media`-style feature, EPUB unpacking, or temp-dir spill re-introduces it. Audit every place an archive entry name reaches the filesystem.

### 3. XML family (XLIFF, Android XML, RESX, OOXML parts, IDML, TMX, SVG-in-assets)

**Good news, with caveats.** Go's `encoding/xml` does **not** expand external entities — "the decoder used to fail when it encountered external entities, but now it simply prints the entity name instead of failing," so XXE and billion-laughs entity-expansion attacks "don't work" against the stdlib parser. (https://knowledge-base.secureflag.com/vulnerabilities/xml_injection/xml_entity_expansion_go_lang.html ; https://www.stackhawk.com/blog/golang-xml-external-entities-guide-examples-and-prevention/)

**But three documented stdlib issues remain relevant:**

1. **Stack exhaustion via deep nesting — `CVE-2022-28131`.** "Uncontrolled recursion in `Decoder.Skip` … allows an attacker to cause a panic due to stack exhaustion via a deeply nested XML document" (fixed Go 1.17.12 / 1.18.4). It is one of a cluster of stdlib stack-exhaustion bugs (also `encoding/xml` Unmarshal, `encoding/gob`, `compress/gzip`, `path/filepath.Glob`). Go has an open proposal to make stack exhaustion *recoverable* (#74577) — until then a deeply nested doc can `panic` an unprotected goroutine and there is no `recover` for stack overflow. (https://github.com/golang/go/issues/53614 ; https://app.opencve.io/cve/CVE-2022-28131 ; https://github.com/golang/go/issues/74577)

2. **Round-trip semantic instability — `CVE-2020-29509/29510/29511`.** "Maliciously crafted XML markup mutates during round-trips through Go's decoder and encoder," enabling SAML signature bypass. **The Go team did not fix this in the stdlib** — it remains a documented design limitation; Mattermost shipped `xml-roundtrip-validator` as the workaround. This is directly load-bearing for neokapi because its *entire value proposition is faithful read→write round-trips of XML formats.* A format that survives crash-free fuzzing can still silently corrupt content. (https://mattermost.com/blog/coordinated-disclosure-go-xml-vulnerabilities/)

3. **Third-party XML libs are worse.** `github.com/antchfx/xmlquery` had `CVE-2020-25614` (DoS / SIGSEGV via non-XML `LoadURL` response). Any non-stdlib XML/XPath dependency re-introduces XXE/SSRF/DoS surface and must be pinned + govulncheck-watched. (https://security.snyk.io/vuln/SNYK-GOLANG-GITHUBCOMANTCHFXXMLQUERY-759275)

### 4. YAML family (project recipes, i18next, ARB-adjacent, config)

**Billion-laughs applies to YAML too**, via anchor/alias amplification: "the 'billion laughs' attack was initially targeted at XML parsers, [but] similar attacks can be launched against … YAML." (https://en.wikipedia.org/wiki/Billion_laughs_attack) A real advisory shows the impact: MarkUs "YAML alias ('billion laughs') DoS in config upload." (https://github.com/MarkUsProject/Markus/security/advisories/GHSA-m9rx-85mx-q9h6)

**`gopkg.in/yaml.v3` history.** `CVE-2022-28948`: "an issue in the Unmarshal function … causes the program to crash when attempting to deserialize invalid input." (https://nvd.nist.gov/vuln/detail/CVE-2022-28948) The hardening PR by Jordan Liggitt (go-yaml #515) is the **reference design for amplification limits**: "limiting stack depth to 10,000 keeps parse times of pathological documents sub-second (~.25 s)"; and the alias-expansion budget was tightened from "10,000% expansion … too permissive" to "10% expansion for larger documents," explicitly tying allowed expansion to *input size* so callers can bound resource use by capping bytes in. (https://github.com/go-yaml/yaml/pull/515) Academic backing: "Laughter in the Wild: A Study into DoS Vulnerabilities in YAML Libraries." (https://www.researchgate.net/publication/333505459)

### 5. JSON, encoding, and image/media

- **Deeply-nested JSON** is a recursion-depth DoS in any recursive descent parser; the YAML stack-depth-cap pattern (cap at N, return error not panic) is the mitigation. Go's `encoding/json` is iterative for arrays/objects but still has practical depth concerns; cap depth and total token count.
- **Image/media decode bombs.** Go's `image` package position: "A call to `Decode` which produces an extremely large image, as defined in the header returned by `DecodeConfig`, is **not** considered a security issue." The mandated defense is caller-side: "When operating on arbitrary images, `DecodeConfig` should be called before `Decode`, so that the program can decide whether the image … can be safely decoded with the available resources." There is no built-in `MaxWidth/MaxHeight` (proposal #27830 still open). `x/image/tiff` had OOM-from-malicious-IFD-offset (#78267). Any media neokapi pulls out of containers (images in docx, EPUB) needs explicit dimension/size pre-checks. (https://pkg.go.dev/image ; https://github.com/golang/go/issues/27830 ; https://github.com/golang/go/issues/78267)
- **Malformed UTF-8 / encoding attacks**: detection→conversion (neokapi's `core/encoding`) must reject or replace invalid sequences deterministically; overlong/ambiguous encodings cause divergence between what a parser sees and what a downstream renderer sees.

### 6. Markdown / HTML re-rendering (XSS when output is re-displayed)

This is neokapi's biggest *output-side* risk because docs/labs re-render parsed content in a browser. **goldmark is safe by default:** "By default, goldmark does not render raw HTML or potentially-dangerous URLs"; raw HTML requires explicitly opting into `WithUnsafe()`, which "renders dangerous contents (raw htmls and potentially dangerous links) as it is." (https://github.com/yuin/goldmark/blob/master/README.md) When untrusted content *is* rendered, the canonical chain is **goldmark (no `WithUnsafe`) → bluemonday sanitizer**, run last: "You should always run bluemonday after any other processing." (https://pkg.go.dev/github.com/microcosm-cc/bluemonday)

CVE precedent in comparable parsers shows the DoS surface: `marked` `CVE-2022-21681` (catastrophic regex backtracking) and `CVE-2026-41680` (3-byte sequence → infinite recursion → memory exhaustion). The MdPerfFuzz research project (ASE '21) specifically fuzzes markdown compilers for *performance* bugs (algorithmic DoS), a model neokapi can borrow. (https://www.cvedetails.com/cve/CVE-2022-21681 ; https://cvereports.com/reports/CVE-2026-41680 ; https://github.com/cuhk-seclab/MdPerfFuzz)

### 7. The cgo / native-bridge exception (re-introduces memory unsafety)

neokapi's memory-safety advantage **evaporates wherever cgo or a subprocess crosses into unsafe code**: the okapi-bridge JVM (57+ Okapi filters), cgo ICU, onnxruntime (SaT), native SQLite. The canonical real-world lesson is **ExifTool `CVE-2021-22204`**: a malicious DjVu annotation reached Perl `eval` → arbitrary code execution, "notably exploited in attacks against GitLab instances, where uploaded images are automatically processed by ExifTool." (https://www.sentinelone.com/vulnerability-database/cve-2021-22204/) The takeaway: **any format whose parsing dispatches to a non-Go engine (okapi-bridge especially) must be treated as Rule-of-2 leg (b) present, and isolated accordingly** (process boundary + resource caps + no network). This matches Apache Tika's whole architecture (below).

### 8. Apache Tika — the reference architecture for "running untrusted parsers on untrusted data"

Tika is the closest analog to neokapi-at-scale and its hard-won operational doctrine is the single best source.

**Failure taxonomy (load-bearing for neokapi's classification taxonomy).** Tika frames the problem as: parsers "can go into infinite loops or allocate surprising amounts of memory (OutOfMemoryExceptions (OOMs))." Its robustness page distinguishes **catchable exceptions** (a thrown parse error you can log and continue past) from **OOMs** and **infinite loops / permahangs** that you *cannot* catch in-process. (https://cwiki.apache.org/confluence/display/TIKA/The+Robustness+of+Apache+Tika) The canonical informal taxonomy from Tika's Tim Allison is "catchable exceptions vs. evil OOMs vs. permahangs."

**Defense = process isolation, always.** "ForkParser … forks a child process and will protect against OOM and infinite loops." "In Tika >= 2.x, the parsing is done in a forked process by default. If there's an OOM or a timeout or other crash during the parse, the forked process will shutdown and restart." The strongest stated rule: **"avoid running Tika in the same process as anything that matters, such as your indexer."** (same robustness page)

**Concrete production config (tika-server).** `taskTimeoutMillis` = "number of milliseconds to allow per task (parse, detection, unzipping, etc.)" **default 300000 (5 min)** — on expiry "shutting down the forked server process and restarting it." `maxRestarts` default `-1` (always restart). `-spawnChild` became the default in 2.0. (https://tika.apache.org/3.0.0/api/org/apache/tika/server/core/TikaServerConfig.html ; https://www.mail-archive.com/commits@tika.apache.org/msg08017.html)

**Regression corpus.** Tika/POI/PDFBox jointly gathered ">1TB of documents from govdocs1 and from Common Crawl" and a "regression corpus ~2 million files from Common Crawl" swept pre-release. They also code-review dependencies for "read-a-length-then-allocate patterns" — the single most common OOM root cause. (robustness page) Real CVEs that came out of this work: `CVE-2019-10088` ("OOM from a crafted Zip File in … RecursiveParserWrapper"), `CVE-2022-30126` (regex-backtracking DoS, whose *first two fixes were insufficient* — a cautionary tale that DoS fixes need fuzzing to confirm). (https://www.openwall.com/lists/oss-security/2019/08/02/2 ; https://tika.apache.org/security.html)

### 9. LibreOffice crashtesting — the round-trip sweep model

LibreOffice runs a Python harness (`test-bugzilla-files.py`) that imports a large document corpus and detects crashes, then extends to **import + export (round-trip)** testing — directly analogous to neokapi's read→write→read property. Corpus is drawn from Bugzilla attachments and grew to tens of thousands of files across `doc/docx/ppt/pptx/xls/xlsx/odt/rtf/...`; runs regularly on a TDF server, with results posted to the dev list. (https://mmohrhard.wordpress.com/2013/04/19/automated-import-crash-testing-in-libreoffice/ ; https://cgit.freedesktop.org/libreoffice/contrib/dev-tools/tree/test-bugzilla-files/test-bugzilla-files.py ; https://lists.freedesktop.org/archives/libreoffice/2023-March/090052.html) Their crash-fix blog series documents the operational classification of assertion failures vs. real crashes. (https://dev.blog.documentfoundation.org/2024/04/18/crash-fixes-part-3-testing-crashes/)

### 10. Hostile corpora available as inputs

- **Digital Corpora UNSAFE-DOCS (CC-MAIN-2021-31-UNSAFE)** — "over 5.0 million files … known to contain malformed and malicious PDF files," gathered by NASA JPL + Kudu Dynamics for DARPA SafeDocs; free, AWS Open Data sponsored. (https://digitalcorpora.org/corpora/file-corpora/unsafe-docs-cc-main-2021-31-unsafe/)
- **govdocs1** — ~1M real-world files of mixed formats, the classic regression corpus. (https://digitalcorpora.org/corpora/file-corpora/files/)
- **PDF-Association pdf-corpora index** for PDF-specific error/edge corpora. (https://github.com/pdf-association/pdf-corpora)
- These are mostly PDF/Office; neokapi will need to *harvest* per-format wild sets (bug-tracker attachments, Common Crawl WARC extraction by content-type) for its 49 formats.

### 11. Continuous fuzzing as ongoing process

- **Go native fuzzing** (`go test -fuzz`, Go 1.18+). Crash workflow is the key feature: "When fuzzing discovers a failure, the fuzzing engine writes the failing input to the seed corpus … serving as a regression test once the bug has been fixed." Seeds live in `testdata/fuzz/<FuzzName>/`; `f.Add` registers in-code seeds. **Crashers automatically become committed regression tests** — exactly the promotion mechanism neokapi wants. (https://go.dev/doc/security/fuzz/ ; https://pkg.go.dev/testing)
- **ClusterFuzzLite** = PR-time + batch fuzzing in CI. Modes: "Quick code change (pull request) fuzzing to find bugs before they land," "Continuous longer running fuzzing (batch fuzzing)," corpus pruning, and "Coverage reports showing which parts of your code are fuzzed." Supports GitHub Actions, GitLab, Google Cloud Build, Prow; languages C/C++/Java/Go/Python/Rust/Swift; sanitizers ASan/MSan/UBSan (sanitizers mostly relevant to the cgo bridges, not pure Go). Config in `.clusterfuzzlite/`. (https://google.github.io/clusterfuzzlite/ ; https://github.com/google/clusterfuzzlite)
- **OSS-Fuzz** supports native Go fuzz tests; integration = `projects/<name>/project.yaml` + `Dockerfile` + `build.sh` via PR. **Acceptance bar:** "a significant user base and/or be critical to the global IT infrastructure," weighing "exposure to remote attacks (e.g. libraries that … process untrusted input)." Note: the **OSS-Fuzz *Rewards Program* is sunsetting May 1, 2026**, but the platform keeps running and keeps accepting projects. (https://google.github.io/oss-fuzz/getting-started/new-project-guide/go-lang/ ; https://google.github.io/oss-fuzz/getting-started/accepting-new-projects/) For a solo maintainer, **ClusterFuzzLite (self-hosted in own CI) is the pragmatic entry point**; OSS-Fuzz is a stretch goal once adoption grows.
- **Structured round-trip fuzzing**: fuzz `Read→Write→Read` asserting (1) no crash/panic, (2) idempotence (second read == first), (3) no resource blowup. This catches both the crash class *and* neokapi's faithfulness invariant (the XML round-trip-instability class from §3).

### 12. Resource-limit primitives that real Go libraries expose

- `io.LimitReader` / `http.MaxBytesReader` — cap total bytes read before parsing (the simplest, most effective DoS bound; also the fix the `file-type` CVE needed applied uniformly).
- `context.WithTimeout` / `WithDeadline` — wall-clock budget per parse; propagate to all pipeline goroutines (neokapi's channel pipeline already threads `ctx`).
- **klauspost/compress zstd**: `WithDecoderMaxMemory` — "set a maximum decoded size for in-memory non-streaming operations or maximum window size for streaming operations … to control memory usage of potentially hostile content" (default 64 GiB — far too high; set explicitly), plus `WithDecoderMaxWindow` to "reject packets that will cause big memory usage." (https://pkg.go.dev/github.com/klauspost/compress/zstd)
- **archive/zip**: no built-in ratio limit — you wrap each `File.Open()` reader in a counting `io.LimitReader` and compare uncompressed-bytes-out to compressed-bytes-in per entry (the POI `MinInflateRatio` pattern, reimplemented).
- **Depth counters**: explicit recursion depth cap returning an error (not relying on stack overflow), per the go-yaml stack-depth-10,000 precedent. (https://github.com/go-yaml/yaml/pull/515)
- **systemd-run** (Linux ops layer): `MemoryMax=`, `CPUQuota=`, `--wait --collect` for transient sandboxed batch jobs with `NoNewPrivileges=yes`, `ProtectSystem=strict`, `PrivateNetwork=yes`. (https://wiki.archlinux.org/title/Systemd/Sandboxing)

### 13. Sandboxing & isolation tiers for hostile sweeps (2026 landscape)

Four primitives, weakest→strongest: **plain containers** (fast, weak), **gVisor** (syscall interception in userspace "Sentry," moderate overhead, "drastically reducing kernel attack surface"), **Firecracker microVMs** (KVM hardware isolation, "~125 ms boot, ~5 MB overhead … current gold standard for untrusted code," powers AWS Lambda/E2B/Vercel Sandbox), **WASM** (capability-first, near-zero overhead). (https://northflank.com/blog/how-to-sandbox-ai-agents ; https://zylos.ai/research/2026-04-04-ai-agent-sandboxing-security-isolation/)

**GitHub Actions runner caveat.** GitHub-hosted runners are ephemeral, isolated, and rebuilt per job — acceptable for *running* hostile inputs against a memory-safe Go parser since the worst case is a crash/OOM of a throwaway VM. The danger is **self-hosted persistent runners**, which "can be weaponized into persistent backdoors." Rule: hostile-corpus sweeps run only on **GitHub-hosted or ephemeral self-hosted** runners, **never persistent self-hosted**, **with network egress disabled**. (https://www.sysdig.com/blog/how-threat-actors-are-using-self-hosted-github-actions-runners-as-backdoors ; https://docs.github.com/en/actions/concepts/security/compromised-runners)

### 14. SSRF / external fetch — the network leg

Gotenberg's 2026 hardening is the model: OOXML/RTF/ODF "files can embed external URLs that LibreOffice's libcurl resolves," so it "routes every outbound fetch through an in-process forward proxy" and "rejects non-public addresses including loopback, RFC1918, link-local … and IPv4-mapped IPv6," exposed via `--libreoffice-deny-private-ips`. (https://gotenberg.dev/docs/configuration ; https://integsec.com/blog/cve-2026-40281-gotenberg-metadata-injection-vulnerability...) For neokapi: **default-deny all network egress during parsing.** The pure-Go XML parser doesn't fetch DTDs, but XPath libs (`LoadURL`), image-with-remote-href, and the okapi-bridge can. Belt-and-suspenders: process-level network namespace off.

### 15. Output-side: pandoc arbitrary-file-write & Trojan Source

- **pandoc `CVE-2023-35936`**: "specially crafted image element … when generating files using `--extract-media` or outputting to PDF" → arbitrary file write via percent-encoded `../` in a `data:` URI. Disclosed via **GitHub Security Advisory** with a clean workaround ("disallow PDF output and `--extract-media`") — a model disclosure. Directly relevant: any neokapi feature that writes media/output paths derived from document content must unescape *before* the traversal check. (https://github.com/jgm/pandoc/security/advisories/GHSA-xj5q-fv23-575g)
- **Trojan Source `CVE-2021-42574`**: invisible Unicode BiDi override chars reorder displayed text vs. parsed/compiled meaning. For a *localization* tool this is acute — translated content can carry BiDi/zero-width chars that look benign but render/inject differently downstream. A "contains-suspicious-Unicode-controls" lint belongs in neokapi's QA/check family. (https://access.redhat.com/security/vulnerabilities/RHSB-2021-007)

### 16. Vulnerability intake & advisory practice for parser-heavy OSS

- **govulncheck** in CI: call-graph-aware (reports only *reachable* vulnerable symbols, not every vulnerable module in `go.mod`), emits SARIF to GitHub code-scanning, non-zero exit blocks merges. (https://go.dev/doc/security/vuln/ ; https://go.dev/doc/security/best-practices) Run on PRs + nightly (new advisories land against unchanged code).
- **GitHub private vulnerability reporting + Security Advisories** is the disclosure channel (pandoc, Tika both use it). Publish a `SECURITY.md` with a private reporting path and a stated triage SLA. (https://github.com/jgm/pandoc/security/advisories/GHSA-xj5q-fv23-575g)
- **Watch advisory feeds for format-adjacent deps**: goldmark, bluemonday, yaml.v3, klauspost/compress, antchfx/* , the JVM Okapi filters (POI/PDFBox CVEs flow through okapi-bridge), ICU, onnxruntime. Dependabot/Renovate + govulncheck covers Go; the JVM bridge needs its own dependency scan.

### 17. Security in maturity rubrics elsewhere — precedent for an L0–L4 gate

- **OpenSSF Scorecard "Fuzzing" check**: "Does the project use fuzzing tools, e.g. OSS-Fuzz, QuickCheck or fast-check?" Passing probes include `fuzzedWithGoNative`, `fuzzedWithClusterFuzzLite`, `fuzzedWithOSSFuzz` — **"have Go fuzzers … in its source tree"** alone passes. Scorecard also has `Security-Policy`, `Vulnerabilities` (open unfixed), and `Dangerous-Workflow` checks. (https://github.com/ossf/scorecard/blob/main/docs/checks.md) This is the **direct precedent that "has fuzzers + clean vuln scan" is a recognized, machine-checkable security bar** — neokapi can adopt the same probes per-format.
- **OSS-Fuzz "fuzzing" badge** is the recognized public certification of continuous fuzzing.
- No widely-adopted *per-component* "hardened against malicious input" cert exists — neokapi's proposed "parses UNSAFE corpus without crash/hang/OOM under X MB / Y s" gate would be novel and defensible, analogous to the DARPA SafeDocs program's premise.

---

## Design implications for neokapi

**(a) Per-format-family attack/mitigation matrix** (for the format-ops dataset; one row per family):

| Family (neokapi formats) | Top attack classes | Canonical mitigation (Go) | Reference |
|---|---|---|---|
| Zip-container (docx/xlsx/pptx, EPUB, IDML) | Decompression bomb (nested/overlap/Zip64); zip-slip; entry-count flood; oversized single entry | Per-entry `io.LimitReader` ratio check (POI `MinInflateRatio` 1% + `MaxEntrySize`); cap entry count; `filepath.IsLocal`/`os.Root` for any extraction; identical limits on CLI/server/WASM paths | POI ZipSecureFile; bamsoftware; go.dev/blog/osroot; file-type CVE-2026-32630 |
| XML (XLIFF, AndroidXML, RESX, TMX, OOXML parts, SVG) | Deep-nesting stack-exhaustion panic; round-trip semantic mutation; (XXE/billion-laughs mostly N/A in stdlib) | Depth counter (error not panic); prefer stdlib `encoding/xml`; round-trip equivalence assertion in fuzzing; xml-roundtrip-validator pattern; pin/scan any 3rd-party XML lib | CVE-2022-28131; CVE-2020-29509..11; antchfx CVE-2020-25614 |
| YAML (recipes, i18next, config) | Anchor/alias amplification (billion-laughs); crash-on-invalid Unmarshal | Stack-depth cap (≤10k); expansion budget tied to input size (≤~10%); yaml.v3 ≥ patched | go-yaml PR#515; CVE-2022-28948 |
| JSON (ARB, xcstrings, designtokens, i18next) | Recursion-depth DoS; huge-number/precision | Depth + token-count caps; `io.LimitReader` | go-yaml depth precedent |
| Markdown/HTML | Algorithmic DoS (regex backtracking, infinite recursion); XSS on re-render | goldmark default (no `WithUnsafe`) → bluemonday last; fuzz for perf bugs (MdPerfFuzz) | goldmark README; bluemonday; marked CVEs; MdPerfFuzz |
| Image/media (extracted from containers) | Decode bomb (dimension/pixel) | `DecodeConfig` + Max width/height pre-check before `Decode`; size cap | Go image pkg; #27830; x/image/tiff #78267 |
| PO/gettext, subtitles, properties | Encoding/UTF-8 confusion; pathological line counts | Reject invalid UTF-8 deterministically; line/size caps | (encoding family) |
| cgo/bridge (okapi-bridge JVM, ICU, onnxruntime, native SQLite) | Memory-corruption RCE (Rule-of-2 leg b returns) | Subprocess isolation + timeout + memory cap + no network; treat as Tika ForkParser | ExifTool CVE-2021-22204; Tika robustness |
| Any (cross-cutting) | SSRF / external fetch; arbitrary file write on output; BiDi/zero-width injection in translated content | Default-deny network namespace; unescape-before-traversal-check on output paths; suspicious-Unicode lint | Gotenberg; pandoc CVE-2023-35936; Trojan Source CVE-2021-42574 |

**(b) Go resource-limit pattern catalog** (codify as a shared `core/safeio` package; every reader uses it):
- `LimitedReader(r, maxBytes)` wrapping `io.LimitReader` — single global byte budget, **applied identically across CLI/server/WASM** (the file-type CVE's lesson).
- `ParseContext(ctx, timeout)` — wall-clock deadline threaded through the existing channel pipeline; tools already take `ctx`.
- `DepthGuard` — increment/decrement counter, return `ErrTooDeep` at cap (default ~1000; YAML/XML); never rely on stack-overflow recovery (not recoverable in Go today, #74577).
- `RatioLimitedZipEntry` — per-entry counting reader enforcing min-inflate-ratio + max-uncompressed-size + global entry-count cap (POI semantics).
- `SafeJoin` — `filepath.IsLocal` + `os.Root` (Go 1.24) for any path derived from document content; unescape before checking (pandoc lesson).
- zstd/gzip readers always constructed with explicit `WithDecoderMaxMemory`/window caps, never defaults.
- Output writers: cap output size; reject output paths that aren't `IsLocal`.

**(c) Solo-maintainer nightly hostile-corpus sweep runbook (sketch):**
1. **Corpus tiers**: `wild/` (govdocs1 + Common-Crawl-by-content-type + bug-tracker attachments — expected-parseable) and `unsafe/` (Digital Corpora UNSAFE-DOCS, OPF/PDF-Association error corpora — expected-to-break-safely). Store corpora *out of git* (LFS/release assets/object store), pin by content hash.
2. **Isolation**: run each batch in an **ephemeral, network-disabled** container (`--network=none`) with hard caps — `systemd-run --scope -p MemoryMax=2G -p CPUQuota=200%` or container `--memory`/`--pids-limit`, plus a per-file **wall-clock timeout** (start at Tika's 5 min, likely 30–60 s for localization formats). GitHub-hosted runners are acceptable (throwaway VMs, memory-safe Go); **never persistent self-hosted**; escalate to gVisor/Firecracker only when the cgo bridges are in scope.
3. **One file = one subprocess** (Tika ForkParser doctrine): never parse a hostile file in the sweep-orchestrator process. The orchestrator forks `kapi parse --limits=strict <file>` and watches exit status + wall clock + RSS.
4. **Failure classification taxonomy** (the deliverable taxonomy, Tika-derived):
   - `OK` — parsed within limits.
   - `OK_ROUNDTRIP` — read→write→read idempotent (the faithfulness bar).
   - `EXPECTED_REJECT` — clean error returned within limits (good: graceful refusal).
   - `CRASH` — Go panic / non-zero unexpected exit → **promote to regression corpus immediately**.
   - `HANG` (permahang) — killed by wall-clock timeout → promote + flag for depth/iteration audit.
   - `OOM` (evil OOM) — killed by memory cgroup → promote + flag for allocation/ratio audit ("read-a-length-then-allocate").
   - `ROUNDTRIP_DRIFT` — parsed but second read ≠ first (the XML-instability class) → faithfulness bug, promote.
5. **Promotion**: any `CRASH`/`HANG`/`OOM`/`ROUNDTRIP_DRIFT` input is minimized (go-fuzz minimization or manual), hashed, and committed to `testdata/fuzz/<Format>/` so `go test` re-runs it forever as a regression — Go's native crasher-becomes-seed mechanism does this automatically for fuzz-found crashes; the sweep does it for corpus-found ones.
6. **Triage by Claude agents**: nightly diff of the classification report; agents file/update issues per new crasher, attempt the resource-limit fix, and add the regression seed. Watch for *insufficient-fix* recurrence (Tika CVE-2022-30126 needed three rounds).
7. **Dashboard**: extend `/format-maturity` with a per-format security panel: % corpus `OK_ROUNDTRIP`, count of open `CRASH/HANG/OOM`, fuzz coverage %, last sweep date, govulncheck status.

**(d) Proposed L0–L4 "security/hardening" axis** (add as an orthogonal dimension to the maturity rubric, with machine-checkable exit criteria mirroring OpenSSF Scorecard probes):
- **S0 — Unhardened**: no resource limits; parses in-process; no fuzz target.
- **S1 — Bounded**: reader uses the `core/safeio` budget primitives (byte cap, depth guard, ctx deadline); zip readers enforce ratio+entry caps; no path-traversal on any output; **passes the wild corpus** with 0 `CRASH`/`HANG`/`OOM`.
- **S2 — Fuzzed**: a committed Go native fuzz target (`Fuzz<Format>`) exists for read, and a round-trip fuzz target asserting crash-freedom + idempotence; runs in CI (ClusterFuzzLite PR mode); Scorecard `fuzzedWithGoNative` would pass.
- **S3 — Hostile-hardened**: **passes the UNSAFE corpus** — for every file, terminates as `EXPECTED_REJECT`/`OK` within fixed limits (proposal: **≤256 MB RSS, ≤10 s wall-clock per file** for text formats; tunable per family), 0 `CRASH`/`HANG`/`OOM`; govulncheck clean for the format's dependency closure; round-trip drift = 0 on the wild corpus.
- **S4 — Continuously assured**: continuous (batch) fuzzing (ClusterFuzzLite batch or OSS-Fuzz); every historical crasher is a committed regression seed; nightly UNSAFE sweep green ≥30 days; SECURITY.md disclosure path; for cgo/bridge formats, subprocess isolation verified. This is novel per-component "hardened against malicious input" certification with no current OSS precedent below the whole-project Scorecard level.

**(e) CI/fuzzing wiring recommendation:**
- **Now (solo-maintainer pragmatic path):** (1) one `Fuzz<Format>` native target per format under `core/formats/<fmt>/fuzz_test.go`, read + round-trip; (2) `ClusterFuzzLite` GitHub Actions in **PR (code-change) mode** for fast pre-merge fuzzing of touched formats, plus **batch mode** nightly; (3) `govulncheck` PR + nightly with SARIF→code-scanning, non-zero-exit gate; (4) the nightly hostile-sweep workflow on GitHub-hosted runners with `--network=none` + memory/timeout caps; (5) crashers auto-committed to `testdata/fuzz/`.
- **Later (stretch):** OSS-Fuzz onboarding once neokapi clears the "significant user base / processes untrusted input" bar (it plainly processes untrusted input — the strong half of the criterion); note the Rewards Program closed 2026-05-01 but the platform still onboards. Add the OpenSSF Scorecard workflow and surface the fuzzing/security-policy/vulnerabilities checks as repo badges.
- **Bridge-specific:** ASan/MSan/UBSan builds (ClusterFuzzLite supports them) only matter for the cgo paths (ICU, onnxruntime) and the JVM bridge — run those as a separate, isolated lane; the pure-Go formats get coverage-guided fuzzing without sanitizers.
