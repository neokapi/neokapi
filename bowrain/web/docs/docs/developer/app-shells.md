---
title: One app, two shells
sidebar_position: 15
description: The browser app and the desktop app are one application. The surfaces and the route tree are shared packages; each shell adds what its host provides.
---

# One app, two shells

The browser app and the desktop app are one application. The surfaces and the
route tree live in two shared packages. Each shell mounts them over its own
API transport and host adapter, and adds the screens its host needs.

## What the shells share

| Package | Path | Holds |
| --- | --- | --- |
| `@neokapi/ui` | `bowrain/packages/ui` | The surfaces: the app shell (workspace rail, sidebar, top bar), the project dashboard and project view, the editor, the review surfaces, the context hub and explorer, the content memory and terms explorers, automations, connectors, streams, members and settings panels, and the `RestApiAdapter`. |
| `@neokapi/bowrain-app` | `bowrain/packages/app` | `BowrainApp` and the router: the workspace, project, context, review, run and settings routes, the flows editor under a project's automations, the account pages (login, join, claim, welcome, device verification, email confirmation) and the pricing page. |

A shell gives `BowrainApp` two things: an `ApiAdapter`, which carries every
request to the server, and a `PlatformAdapter`
(`bowrain/packages/app/src/platform.tsx`), which carries what a route needs
from its host. A route never imports shell code. Where the shells differ, the
route reads `platform.kind` or feature-detects an optional adapter member.

## What the web shell adds

The web shell is `bowrain/apps/web`. It mounts the shared app over the
`RestApiAdapter` with cookie sessions and browser history.

- Sign-in through the server's identity provider. An expired session redirects
  to `/api/v1/auth/login`, except on the join, claim, device and GitHub setup
  pages, which handle the round-trip themselves.
- The account pages and the pricing page, reached by their URLs.
- Live updates over a same-origin event stream.
- The **Open in Bowrain Desktop** banner on a project, which hands the project
  off through a `bowrain://` deep link.
- Error reporting and product analytics for the hosted service.

## What the desktop shell adds

The desktop shell is `bowrain/apps/bowrain`. A Wails app hosts the same route
tree in a webview over hash history, with a Go backend that holds the server
connection, a content cache and an offline edit queue.

- A connection gate before the app mounts: the **ServerConnect** screen signs
  in with PKCE against the server you name, and the **OfflineLaunch** screen
  holds and retries when that server cannot be reached.
- Connectivity in the shared chrome: the connection indicator, the count of
  pending changes, and the list of changes that did not reach the server.
- Working-copy reads served from the backend, so a project stays readable
  offline. Connectors are served the same way; the desktop registers the
  remote connectors only.
- Deep links from the OS, native file drops, opening an exported file in the
  system file manager, and a telemetry notice with an opt-out.
- A flow that needs a browser origin, such as registering a passkey, opens the
  same page of the web app in the browser.

## How the documentation treats them

The [user documentation](/server/web-overview) describes one application. A
surface both shells share is documented and recorded once, and the page says
nothing about which shell it was recorded in. [The desktop app](/server/desktop-app)
documents what the desktop shell adds, as additions to the same application.
