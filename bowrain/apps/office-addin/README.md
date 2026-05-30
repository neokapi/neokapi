# Bowrain for Microsoft 365 (Office Add-in)

A task-pane add-in for **Word, Excel, and PowerPoint** (web + desktop) that
surfaces Bowrain's three in-product operations against the open document:

- **Brand voice** — score the selection against a brand voice profile and list
  findings.
- **Terminology** — show the approved / forbidden / competitor terms present in
  the selection.
- **Translation** — translate the selection on-brand and write it back.

It is a small React SPA (this directory) that calls the Bowrain **add-in REST
API** (`/api/v1/addin/check|terms|translate`, implemented in `bowrain/addin`)
and drives the document through Office.js.

## Develop

```bash
make -C bowrain office-addin-deps     # install (or `vp install` at the repo root)
make -C bowrain office-addin-dev      # dev server on https://localhost:3300
make -C bowrain office-addin-check    # oxlint + prettier + tsc
make -C bowrain office-addin-test     # vitest
make -C bowrain office-addin-build    # production build → dist/
```

The dev server proxies `/api` to a local `bowrain-server` on `:8080`. In
production the built `dist/` is served from the Bowrain web origin (same origin
as the API — no CORS), and `manifest.xml` points the task pane there.

## Files

| File              | Purpose                                                                                        |
| ----------------- | ---------------------------------------------------------------------------------------------- |
| `manifest.xml`    | Add-in-only (XML) manifest — **the baseline**, widest reach (incl. perpetual Office + mobile). |
| `manifest.json`   | Unified Microsoft 365 manifest — for Teams co-distribution / Copilot agent co-packaging.       |
| `src/api.ts`      | Typed client for the Bowrain add-in REST API.                                                  |
| `src/office.ts`   | Office.js Common-API wrappers (read selection / replace / SSO token).                          |
| `src/office.d.ts` | Minimal ambient Office.js types (no `@types/office-js` dependency).                            |
| `src/App.tsx`     | The task-pane UI.                                                                              |

## What you must provide (production)

This add-in is functionally complete but needs an identity and a host before it
can be sideloaded or published. See
[`bowrain/docs/integrations/workspace-addons.md`](../../docs/integrations/workspace-addons.md)
for the full checklist; in brief:

1. **A host** for the built `dist/` at an HTTPS domain (e.g.
   `addin.bowrain.cloud`). Replace `addin.bowrain.cloud` in both manifests.
2. **An Entra (Azure AD) app registration** with:
   - SPA redirect URIs `brk-multihub://<domain>` and `https://<domain>/index.html`
     (for MSAL nested app authentication / SSO),
   - an exposed API scope `api://<domain>/<appId>/access_as_user`,
   - a fresh **GUID** in `<Id>` / `id` and the `WebApplicationInfo` / `webApplicationInfo`.
3. **Distribution**: sideload for dev, or deploy via the Microsoft 365 admin
   center (centralized deployment) / AppSource.

## Production upgrades (intentionally out of scope here)

- **Auth**: swap the Office SSO `getAccessToken` fallback in `src/office.ts` for
  **MSAL nested app authentication** (`@azure/msal-browser`,
  `createNestablePublicClientApplication`) as the primary path. NAA is
  Microsoft's recommended model and lets the client request the Bowrain API
  scope directly.
- **Richer edits**: use the host-specific APIs (`Word.run` comments / critiques /
  track-changes, `Excel.run` comments) via `@types/office-js` to surface findings
  inline instead of only in the task pane.
