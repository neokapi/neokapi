---
id: bowrain-web-claim-project
audience: developer
target_doc: docs/walkthroughs/bowrain-web-claim-project.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: claim-project
    kind: web
    duration_budget_seconds: 45
    seed:
      workspace: fresh
      anonymous_project:
        name: "Demo project"
        source_lang: en
        target_langs: [fr, de]
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

A developer pushes their first project to Bowrain Server using `bowrain
push`. Because they haven't created a workspace yet, the server creates
the project as **anonymous** and returns a claim URL. The walkthrough
shows the developer following that URL, signing in via SSO, and the
claim-project flow attaching the project to their newly-claimed
workspace.

## Scene 1 — claim-project (web)

User opens the claim URL in a browser. They see the "Sign in to claim"
landing page (unauthenticated state, ClaimPage component). After SSO,
they land back on the claim page, which fetches workspace info and
shows the green "Claim project" CTA. Click claims the project; user is
redirected into the workspace dashboard with the new project visible.

The recording covers the full flow: claim URL → sign in → claim → project
visible in workspace.

## Closing

For the CLI side of the same workflow, see the
[bowrain-getting-started walkthrough](/walkthroughs/bowrain-getting-started)
(scenes: init → push → pull).
