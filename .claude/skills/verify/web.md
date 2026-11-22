# Verify a change to the web app

For `bowrain/apps/web/`, `bowrain/packages/`, `packages/ui/` and the other
packages the web app imports. Run the checks, then the app against the real
server and Keycloak, and look at the changed screen.

## 1. Install at the root

```bash
vp install          # at the repository root, never in a package directory: 16 s warm
```

A worktree under `.claude/worktrees/` has no `node_modules` of its own until
this runs. Until then every `vp` and `node` command resolves workspace packages
from the primary checkout, so a check passes or fails against code you did not
change. Run `vp install` in the worktree before anything below.

## 2. Checks

```bash
make ci-frontend                                       # the CI frontend job: about 2 min
(cd bowrain/apps/web && vp check && vp test)           # 10 s
(cd bowrain/packages/app && vp check && vp test)       # 17 s
(cd bowrain/packages/ui && vp check && vp test)
```

Run the narrow pair for each package you changed while you work, and
`make ci-frontend` once before you commit.

## 3. Run the app against the stack

Start the stack as [server.md](server.md) says, with
`COMPOSE_PROJECT_NAME=bowrain-verify` exported and `make -C bowrain stack-up-e2e`.
Then pick one of two ways to serve the UI.

**Dev server with hot reload** (for iterating). Run it in the background; an
edit shows in the open page within a second or two.

```bash
make -C bowrain dev-web          # http://localhost:5173, proxies /api to :8080
```

**The built app on :8080** (what the Playwright specs expect). Build the web
image and copy it into the server's volume, about 2 min when the image
rebuilds and 8 s when it does not:

```bash
docker compose -f bowrain/compose.full.yaml --profile web run --rm --build web-init
```

`make -C bowrain stack-up-web` stages the same files but exits 2 every time,
because `up --wait` counts the one-shot `web-init` container's normal exit as
a failure (`container bowrain-verify-web-init-1 exited (0)`). Do not read its
exit code as a result.

**Sign in.** Open `http://localhost:8080/api/v1/auth/login`. Keycloak shows a
username and password form; use the test user `stack-up-e2e` printed. You land
on `http://localhost:8080/<slug>`. The session cookie is set for `localhost`
on path `/api/`, so the dev server on `:5173` is signed in too. Do not start
sign-in from `:5173`: Keycloak's client allows only `:8080` redirect URIs.

## 4. Drive the screen with the Playwright browser tools

The browser is shared with other agents. Open your own tab in an isolated
context, and select it again before each action, because another agent's tab
can become the current one between your calls:

- `browser_tabs` with `action: "new"`, `isolatedContext: "verify-web"`,
  `url: "http://localhost:8080/api/v1/auth/login"`.
- `browser_fill_form` on `#username` and `#password`, then `browser_click` on
  `#kc-login`.
- `browser_navigate` to the changed screen, then `browser_snapshot` (with
  `target` and `depth` to keep it short) and assert on the text or role.
- `browser_take_screenshot` with `filename: ".playwright-mcp/<name>.png"`.

Screenshots and snapshots are written under the primary checkout, not the
worktree: a relative filename resolves to `.playwright-mcp/` in the primary
checkout, and a path elsewhere is refused (`outside allowed roots`). Report
the path as `.playwright-mcp/<name>.png` in the primary checkout.

**Viewports.** When layout changed, check the sizes
`bowrain/apps/web/e2e/layout-scroll.spec.ts` uses: 1280x440, 1280x720,
1440x900 and 390x844 (`browser_resize`). Screenshot each one that shows the
change.

**Themes.** When styling changed, screenshot with
`browser_emulate_media` `colorScheme: "light"` and again with `"dark"`. The
app follows the system setting unless its "Theme" button in the top bar has
been set.

## 5. Run the project's specs

The specs in `bowrain/apps/web/e2e/` run against the built app on `:8080`.
They sign in through the stack's development device approval
(`BOWRAIN_ALLOW_INSECURE_DEV` in `compose.full.yaml`), not the Keycloak form.

```bash
cd bowrain/apps/web
vpx playwright install chromium          # once per Playwright version: 1 min
vpx playwright test --config playwright.e2e.config.ts routing.spec.ts --reporter=list            # 8 s
vpx playwright test --config playwright.e2e.config.ts routing.spec.ts --reporter=list --headed   # opens a window
vpx playwright test --config playwright.e2e.config.ts layout-scroll.spec.ts --reporter=list      # 2 min
```

A failing test leaves `test-failed-1.png` and a trace under
`bowrain/apps/web/test-results/`. The specs create projects in a workspace
named `acme` on every run.

Every spec passes on main. `layout-scroll.spec.ts` reports the element that
clips and by how many pixels. When it names `main`, look for an absolutely
positioned descendant below the fold, such as an `sr-only` label, whose
nearest positioned ancestor is `main` and not the scroll container.

## Evidence

For each changed screen: the URL, the screenshot path, and the assertion that
held, such as "snapshot shows heading `Review plan` at 390x844, dark". Add the
spec run with its pass and fail counts. "It renders" is not evidence.
