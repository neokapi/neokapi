# Google Workspace & Microsoft 365 integration

This document describes how Bowrain integrates with the Google Workspace and
Microsoft 365 ecosystems to provide **brand voice, terminology, and translation**
where content is authored, and what is required to connect each ecosystem.

The integration has two complementary layers that share one backend:

```
                        ┌──────────────────────────────────────────┐
   In-product add-ins   │  Google Workspace add-on   Office add-in  │
   (where the user      │  (Docs/Sheets/Slides)      (Word/Excel/   │
    edits content)      │                             PowerPoint)   │
                        └───────────────┬──────────────┬───────────┘
                                        │  brand / terms / translate
                            ┌───────────▼──────────────▼───────────┐
   Shared backend          │        bowrain/addin  (Service)        │
   (one implementation)    │  Check · Terms · Translate             │
                           └───────────┬──────────────┬─────────────┘
                                       │              │
   Server-side connectors   ┌──────────▼───┐   ┌──────▼──────────────┐
   (bulk sync of files)     │ google-      │   │ microsoft365        │
                            │ workspace    │   │ (Graph + native     │
                            │ (Drive/Docs/ │   │  openxml reader)     │
                            │  Sheets/     │   └─────────────────────┘
                            │  Slides)     │
                            └──────────────┘
```

- **Connectors** (`bowrain/connector`) pull source content out of, and push
  translations back into, Google Workspace and Microsoft 365 — the *bulk sync*
  path used by the Bowrain server/worker like the WordPress, Figma, and HubSpot
  connectors. New category: `productivity`.
- **In-product add-ins** put brand/terminology/translation in the editor's
  sidebar/task pane, operating on the document the user has open. They call the
  shared **add-in REST + Card API** (`bowrain/addin`), which reuses the same
  framework engines (`core/brand`, `core/check`, `core/ai/tools`) as the rest of
  the platform.

Keeping one backend means the two add-ins — and any future surface (a Copilot
agent, an MCP tool) — share exactly one implementation and one set of brand
starter packs.

---

## 1. Server-side connectors

Both connectors implement the framework's `IntegrationConnector` interface
(`Fetch` / `Publish` / `List` / `Status`) and register via
`connector.RegisterRemote`, so they are available to both the server and the
desktop working copy. They authenticate with an OAuth2 token source built from
their config map (`bowrain/connector/oauth.go`): static bearer, refreshing
3-legged user OAuth, or app-only client credentials.

### Google Workspace connector (`google-workspace`)

`bowrain/connector/google.go`. Discovers Google Docs/Sheets/Slides via the Drive
API and extracts/writes translatable text via the **structured** Docs/Sheets/
Slides REST APIs (the structured API *is* the format — no file round-trip):

| Kind | Read | Write-back |
|---|---|---|
| Docs | `documents.get` → paragraph text runs | `documents.batchUpdate` `replaceAllText` |
| Sheets | `spreadsheets.values` per sheet (string cells, A1-keyed) | `values.batchUpdate` (RAW) |
| Slides | `presentations.get` → shape/table text runs | `presentations.batchUpdate` `replaceAllText` |

Config keys: `oauth_access_token` (+ `oauth_refresh_token`, `oauth_client_id`,
`oauth_client_secret`, `oauth_token_url` for auto-refresh), optional `file_ids`
(comma-separated, to scope to specific files), optional `base_url` (test/host
override).

### Microsoft 365 connector (`microsoft365`)

`bowrain/connector/microsoft365.go`. Lists `.docx/.xlsx/.pptx` in a OneDrive or
SharePoint drive via Microsoft Graph, downloads the bytes, and **reuses the
framework's native `openxml` reader/writer** (`core/formats/openxml`) to extract
and re-splice translatable runs — Graph returns opaque bytes, so the parsing is
in-process and dependency-free. Write-back faithfully splices translations into
the live document (the connector re-downloads the current bytes and rebuilds the
reconstruction skeleton at publish time, so it is stateless and preserves
formatting).

Config keys: `oauth_access_token` for delegated/testing, or `tenant_id` +
`client_id` + `client_secret` for app-only client credentials (the Entra token
endpoint and Graph `.default` scope are derived automatically); drive selection
via `drive_id`, or `site` (`host:/sites/team`) + optional `library`; optional
`base_url`.

### Example: configure a connector (REST)

```bash
# Add a Google Workspace connector to a workspace, scoped to one doc.
curl -X POST https://api.bowrain.cloud/api/v1/$WS/connectors \
  -H "Authorization: Bearer $BWT" -H 'Content-Type: application/json' \
  -d '{"type":"google-workspace","config":{
        "oauth_refresh_token":"1//0g...","oauth_client_id":"...apps.googleusercontent.com",
        "oauth_client_secret":"GOCSPX-...","file_ids":"1AbC...docId"}}'

# Microsoft 365 connector over a SharePoint library, app-only.
curl -X POST https://api.bowrain.cloud/api/v1/$WS/connectors \
  -H "Authorization: Bearer $BWT" -H 'Content-Type: application/json' \
  -d '{"type":"microsoft365","config":{
        "tenant_id":"contoso.onmicrosoft.com","client_id":"...","client_secret":"...",
        "site":"contoso.sharepoint.com:/sites/marketing","library":"Documents"}}'
```

---

## 2. In-product add-ins

### Shared add-in backend (`bowrain/addin`)

One `Service` with three operations, mounted on the server at `/api/v1/addin`:

| Endpoint | Body | Result |
|---|---|---|
| `POST /addin/check` | `{text, profile?}` | `{profile, score, findings[]}` |
| `POST /addin/terms` | `{text, profile?}` | `{profile, matches[]}` |
| `POST /addin/translate` | `{text, target_locale, source_locale?, profile?}` | `{translation, provider, …}` |

`check` runs the deterministic rule-based brand-vocabulary checker; `terms`
surfaces the profile's preferred/forbidden/competitor terms present in the text;
`translate` uses the configured platform AI provider
(`BOWRAIN_PLATFORM_PROVIDER`), falling back to the keyless **demo** provider so
the surface works with zero extra configuration. The REST endpoints sit behind
the standard bearer-token auth middleware.

### Microsoft 365 Office add-in (`bowrain/apps/office-addin`)

A React task-pane SPA that reads the selection with Office.js, calls the add-in
REST API, and writes translations back. Works across Word, Excel, and PowerPoint
on web + desktop. Ships both an add-in-only **XML** manifest (widest reach) and a
**unified** Microsoft 365 manifest (Teams / Copilot co-packaging). See the app
README for build + deployment.

### Google Workspace add-on (`bowrain/apps/google-workspace-addon`)

The server hosts the add-on's **Card-JSON** endpoints at `/api/v1/addin/google/*`
(`bowrain/addin/google.go`): `homepage`, `authorize` (the `drive.file`
scope-request), `scan` (fetch the active doc, render brand findings + a term
glossary), and `translate` (fetch → translate on-brand → write back). The
`scan`/`translate` handlers drive the Google Workspace connector scoped to the
active file with the user's per-file OAuth token from the event. An Apps Script
implementation is included as a no-server alternative.

### Example: brand-check a draft from the task pane

```ts
// bowrain/apps/office-addin/src/App.tsx (simplified)
const text = await getSelectedText();            // Office.js Common API
const token = await getAccessToken();            // Office SSO / MSAL NAA
const result = await checkBrand(text, token);    // POST /api/v1/addin/check
// result.findings → rendered in the task pane; result.score is 0–100
```

---

## 3. Deep-integration best practices

**Google Workspace**

- Prefer the least-privilege **`drive.file`** scope with the per-file
  authorization flow (`requestFileScopeForActiveDocument` → `onFileScopeGranted`)
  over the broad, *restricted* `documents`/`spreadsheets`/`presentations` scopes
  — it eases OAuth verification and builds user trust.
- HTTP (alternate-runtime) add-ons read/write only via the **REST** APIs; the
  `*.currentonly` scopes are Apps-Script-only.
- **Verify every request**: validate the inbound Google **system ID token**
  (signature, `aud` = the deployment/endpoint URL, and the add-on
  service-account email from `gcloud workspace-add-ons get-authorization`).
- Apply translations and term fixes via a single `batchUpdate` so concurrent
  collaborators never see a partial state; sort index-based Docs edits
  **descending** (Google's "write backwards" rule) and never delete a paragraph's
  trailing newline.
- Render cards fast: show a spinner immediately, then `updateCard` when results
  are ready; cache scan results by document id + revision.

**Microsoft 365**

- Use **MSAL nested app authentication (NAA)** as the primary SSO path (request
  both Graph scopes and the Bowrain API scope from the client); keep the Office
  Dialog API OAuth as a fallback. Never cache tokens yourself.
- For the connector, prefer **application permissions** (app-only client
  credentials) with **`Sites.Selected`** over the tenant-wide
  `Sites.ReadWrite.All` where the workflow allows it.
- Reuse the native `openxml` reader/writer — do not re-parse OOXML. Use Graph
  **`/delta`** for incremental sync and upload sessions (320 KiB-multiple chunks)
  for large files.
- Keep the connector server-side only (it never belongs in the add-in client),
  matching Bowrain's "remote connectors are a server concern" boundary.

**Both**

- One backend, two surfaces: the add-ins are thin; all brand/terminology/
  translation logic lives in `bowrain/addin` + the framework engines.
- Serve the Office task pane from the Bowrain web origin (same origin as the API)
  to avoid CORS; the Google card endpoints are server-to-server (no CORS).

---

## 4. Use cases

- **On-brand drafting in Docs/Word.** A writer drafts in their editor, opens the
  Bowrain sidebar, and gets a brand-voice score + the forbidden/approved terms in
  their draft — without leaving the document.
- **One-click on-brand translation.** Select a paragraph, pick a target language,
  and replace it in place with a translation that respects the brand voice and
  glossary.
- **Bulk localization of a SharePoint library / Drive folder.** The server
  connector fetches every `.docx`/Doc, runs a translation flow, and publishes the
  translated files back — faithfully, preserving formatting.
- **Governed terminology in Sheets.** A localization manager keeps a string sheet
  in Google Sheets; the connector extracts the string column, translates, and
  writes results back to the matching cells.

---

## 5. What you must provide to connect each ecosystem

The implementation is complete and tested against mock APIs, but connecting to
the **real** ecosystems requires identities and secrets that only you can create.

### Google Workspace

| # | What | Where it's used |
|---|---|---|
| 1 | A **Google Cloud project** with the Drive, Docs, Sheets, Slides APIs enabled. | All Google access. |
| 2 | An **OAuth client** (Web application) → `client_id` + `client_secret`. | Connector 3-legged auth; add-on. |
| 3 | The **OAuth consent screen** configured with the scopes in `deployment.json` (prefer `drive.file`). | Consent. |
| 4 | For the add-on: the **Google Workspace Marketplace SDK** enabled (App Configuration + Store Listing) and the deployment installed via `gcloud workspace-add-ons`. | Add-on distribution. |
| 5 | A **public HTTPS host** for bowrain-server reachable by Google, and `BOWRAIN_ADDIN_PUBLIC_URL` set to it. | Card-callback URLs. |
| 6 | (Connector) per-user **refresh tokens** obtained via the OAuth consent flow (offline access), or a Marketplace domain-wide install. | Connector tokens. |

### Microsoft 365

| # | What | Where it's used |
|---|---|---|
| 1 | A **Microsoft Entra ID (Azure AD) app registration** (multi-tenant if serving multiple customers). | All M365 access. |
| 2 | For the **connector**: **application** Graph permissions `Files.ReadWrite.All` and `Sites.ReadWrite.All` (or `Sites.Selected`), with **tenant admin consent**; a `client_secret` (or certificate). | App-only sync. |
| 3 | For the **add-in**: an **SPA redirect** `brk-multihub://<domain>` + `https://<domain>/index.html`, and an exposed API scope `api://<domain>/<appId>/access_as_user`. | NAA SSO → Bowrain API. |
| 4 | A fresh **GUID** in both Office manifests' `id` / `<Id>` and `WebApplicationInfo`/`webApplicationInfo`. | Add-in identity. |
| 5 | An **HTTPS host** for the built task pane (`dist/`); replace `addin.bowrain.cloud` in the manifests. | Task pane source. |
| 6 | Distribution: **sideload** (dev) or the **Microsoft 365 admin center** (centralized deployment) / **AppSource**. | Add-in rollout. |

### Bowrain server configuration

| Env var | Purpose |
|---|---|
| `BOWRAIN_ADDIN_PUBLIC_URL` | Public base URL the Google add-on uses for button-callback URLs (defaults to `OIDC public URL`). |
| `BOWRAIN_PLATFORM_PROVIDER` (+ `BOWRAIN_PLATFORM_API_KEY`, `BOWRAIN_PLATFORM_MODEL`) | The AI provider used for add-in translation; defaults to the keyless `demo` provider. |

Until these are provided, the connectors and add-ins run end-to-end against the
mock servers in the test suite and against the keyless demo translation provider,
so the full code path is exercised in CI without any external credentials.
