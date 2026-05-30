# Bowrain for Google Workspace (Editor Add-on)

A Google Workspace add-on for **Google Docs, Sheets, and Slides** that surfaces
Bowrain's brand-voice checking, terminology lookup, and on-brand translation in
a sidebar, operating on the active document.

There are **two ways to run it** — pick one:

## 1. HTTP add-on (recommended) — powered by bowrain-server

The card UI is served by **bowrain-server's** Card-JSON endpoints
(`bowrain/addin`, mounted at `/api/v1/addin/google/*`). Google's add-on runtime
POSTs the trigger events to those endpoints, which read/write the active
document with the Google Workspace connector (`bowrain/connector` →
`google-workspace`) and return Card JSON.

- **Deploy descriptor**: [`deployment.json`](./deployment.json). Replace
  `addin.bowrain.cloud` with the host running bowrain-server.
- **Deploy** (HTTP / alternate runtime):

  ```bash
  # 1. Get the service account Google uses to call your endpoints.
  gcloud workspace-add-ons get-authorization
  # 2. Create + install the deployment for testing.
  gcloud workspace-add-ons deployments create bowrain --deployment-file=deployment.json
  gcloud workspace-add-ons deployments install bowrain
  ```

- **Endpoints** (see `bowrain/addin/google.go`): `/google/homepage`,
  `/google/authorize`, `/google/scan`, `/google/translate`.

This is the path that scales (many users, central control, one shared backend
with the Office add-in).

## 2. Apps Script add-on (no server) — `apps-script/`

A self-contained Apps Script implementation that reads the document with the
Editor services and calls the Bowrain add-in **REST** API. Use it when you'd
rather not host the HTTP card endpoints.

- [`apps-script/Code.gs`](./apps-script/Code.gs),
  [`apps-script/appsscript.json`](./apps-script/appsscript.json).
- Set Script properties `BOWRAIN_API` (your bowrain-server base URL) and
  `BOWRAIN_TOKEN` (a `bwt_` API token), then deploy as an Editor add-on.

## What you must provide (production)

See [`bowrain/docs/integrations/workspace-addons.md`](../../docs/integrations/workspace-addons.md)
for the full checklist. In brief, for the HTTP add-on:

1. A **Google Cloud project** with the **Google Workspace Marketplace SDK**
   enabled (App Configuration + Store Listing for publishing).
2. The **OAuth consent screen** configured with the scopes in `deployment.json`
   (least-privilege `drive.file` is preferred; `documents`/`spreadsheets`/
   `presentations` are broader/restricted scopes).
3. A **public HTTPS host** for bowrain-server reachable by Google, and the
   server's `BOWRAIN_ADDIN_PUBLIC_URL` set to it (so button-callback URLs are
   absolute).
4. **Request verification**: enable Google **system ID token** verification on
   the card endpoints (validate `aud` + the add-on service-account email from
   `gcloud workspace-add-ons get-authorization`).

## Design notes

- HTTP editor add-ons read/write the active document **only via the Docs/Sheets/
  Slides REST APIs** using the per-file `drive.file` scope and the
  `requestFileScopeForActiveDocument` → `onFileScopeGranted` flow. The
  `*.currentonly` scopes are Apps-Script-only.
- Granular OAuth consent is mandatory for HTTP add-ons
  (`httpOptions.granularOauthPermissionSupport: "OPT_IN"`).
