import { test, expect, type Page } from "@playwright/test";
import {
  authenticate,
  getOrCreateWorkspace,
  createEditorProject,
  getEditorProject,
  findItemId,
  uploadSeedFiles,
  deleteAllEditorProjects,
  waitForServer,
} from "./helpers/api-client";

/**
 * Layout scroll/clipping regression guard.
 *
 * The defect this catches: an app-shell node that clips its overflow while its
 * child grows taller than the space allotted, with no scroll container in
 * between. The content below the fold is then unreachable — no scrollbar, no
 * wheel, no keyboard. It is invisible to functional tests (every element is in
 * the DOM and "visible" to a query) and only shows up as a user reporting that
 * "the interface is cut off".
 *
 * Detection is structural rather than pixel-based: walk the rendered tree and
 * flag any element whose scrollHeight exceeds its clientHeight while its
 * computed overflow-y is `hidden`. That is, by construction, content the user
 * cannot reach — a genuine clip, not a styling opinion. Elements that overflow
 * behind `auto`/`scroll` are fine, and so is a page that simply grows the
 * document (the window scrolls).
 *
 * Short and narrow viewports are the interesting ones: a shell only clips once
 * its content no longer fits, so 1280x720 and 390x844 reproduce what a 1440x900
 * developer display hides.
 */

const BASE_URL = process.env.BOWRAIN_URL || "http://localhost:8080";

// A shell only clips once its content stops fitting, so the short viewports are
// the ones that carry the signal. 1280x440 is the size at which the settings
// sub-nav (twelve entries) first outgrows its column — a browser window with
// devtools docked, or a small desktop-app window.
const VIEWPORTS = [
  { name: "very-short-desktop", width: 1280, height: 440 },
  { name: "short-desktop", width: 1280, height: 720 },
  { name: "laptop", width: 1440, height: 900 },
  { name: "narrow-mobile", width: 390, height: 844 },
] as const;

interface Clip {
  selector: string;
  scrollHeight: number;
  clientHeight: number;
  hiddenPx: number;
  overflowY: string;
  documentScrolls: boolean;
}

/** Inject the auth token as an HttpOnly cookie via Playwright's cookie API. */
async function injectAuthCookie(page: Page, authToken: string) {
  const url = new URL(BASE_URL);
  await page.context().addCookies([
    {
      name: "bowrain_session",
      value: authToken,
      domain: url.hostname,
      path: "/api/",
      httpOnly: true,
      sameSite: "Lax" as const,
    },
  ]);
}

/**
 * findClippedContent returns every element that is clipping content the user
 * has no way to reach.
 *
 * A hit requires all of:
 *   - scrollHeight exceeds clientHeight by more than a rounding tolerance,
 *   - computed overflow-y is `hidden` (so the element itself cannot scroll),
 *   - the element is actually rendered (non-zero box),
 *   - no ancestor scroll container can bring the content into view,
 *   - the clipping is not a deliberate truncation idiom.
 *
 * The tolerance absorbs sub-pixel layout: fractional line heights routinely
 * leave scrollHeight a pixel above clientHeight on a box that is not clipping
 * anything a person would notice.
 */
async function findClippedContent(page: Page): Promise<Clip[]> {
  return page.evaluate(() => {
    const TOLERANCE = 4;
    // Below this, a box is not a layout region a person interacts with — it is
    // the `sr-only` visually-hidden idiom (a 1x1 clip around screen-reader text)
    // or a decorative sliver.
    const MIN_BOX = 24;

    function describe(el: Element): string {
      const parts: string[] = [el.tagName.toLowerCase()];
      if (el.id) parts.push(`#${el.id}`);
      const cls = (el.getAttribute("class") || "").trim();
      if (cls) parts.push(`.${cls.split(/\s+/).join(".")}`);
      const testId = el.getAttribute("data-testid");
      if (testId) parts.push(`[data-testid="${testId}"]`);
      return parts.join("");
    }

    /** Can any ancestor scroll to reveal content clipped at el? */
    function ancestorCanScroll(el: Element): boolean {
      let node = el.parentElement;
      while (node) {
        const s = getComputedStyle(node);
        if (
          (s.overflowY === "auto" || s.overflowY === "scroll") &&
          node.scrollHeight > node.clientHeight + TOLERANCE
        ) {
          return true;
        }
        node = node.parentElement;
      }
      return false;
    }

    const doc = document.documentElement;
    const documentScrolls = doc.scrollHeight > doc.clientHeight + TOLERANCE;

    const out: {
      selector: string;
      scrollHeight: number;
      clientHeight: number;
      hiddenPx: number;
      overflowY: string;
      documentScrolls: boolean;
    }[] = [];

    for (const el of Array.from(document.querySelectorAll("body *"))) {
      const style = getComputedStyle(el);
      if (style.overflowY !== "hidden") continue;
      if (style.display === "none" || style.visibility === "hidden") continue;

      // Skip the visually-hidden idiom (`sr-only`): a 1x1 box that deliberately
      // clips its text for screen-reader-only content. It overflows by design
      // and is not something a sighted user is meant to reach.
      const rect = el.getBoundingClientRect();
      if (rect.width < MIN_BOX || rect.height < MIN_BOX) continue;

      // Skip the line-clamp idiom. A clamped paragraph is a preview: the copy
      // is deliberately cut to N lines on a card and reachable in full on the
      // surface the card links to. It is indistinguishable from accidental
      // clipping by geometry alone — `-webkit-line-clamp` is the intent.
      // Nothing tripped this until a voice profile with a real description
      // existed on /context/voice, which only happened once the brand scan
      // started completing again (#1587).
      const clamp = style.getPropertyValue("-webkit-line-clamp");
      if (clamp && clamp !== "none") continue;

      const hidden = el.scrollHeight - el.clientHeight;
      if (hidden <= TOLERANCE) continue;
      if (ancestorCanScroll(el)) continue;

      out.push({
        selector: describe(el),
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight,
        hiddenPx: hidden,
        overflowY: style.overflowY,
        documentScrolls,
      });
    }
    return out;
  });
}

function report(route: string, viewport: string, clips: Clip[]): string {
  return [
    `${clips.length} element(s) clip unreachable content on ${route} at ${viewport}:`,
    ...clips.map(
      (c) =>
        `  - ${c.selector}\n` +
        `      ${c.hiddenPx}px hidden (scrollHeight ${c.scrollHeight} vs clientHeight ${c.clientHeight}), ` +
        `overflow-y: ${c.overflowY}, document scrolls: ${c.documentScrolls}`,
    ),
  ].join("\n");
}

let token: string;
let wsSlug: string;
let projectId: string;
let itemId: string;

test.describe("Layout: no route clips content the user cannot reach", () => {
  test.beforeAll(async () => {
    await waitForServer();
    token = await authenticate();
    const ws = await getOrCreateWorkspace(
      token,
      process.env.BOWRAIN_WORKSPACE_NAME || "Acme Inc.",
      process.env.BOWRAIN_WORKSPACE || "acme",
    );
    wsSlug = ws.slug;
    await deleteAllEditorProjects(token, wsSlug);

    const proj = await createEditorProject(token, wsSlug, "Layout Guard Project", "en", [
      "fr",
      "de",
      "nb",
    ]);
    await uploadSeedFiles(token, wsSlug, proj.id, ["about-us.html"]);
    projectId = proj.id;
    itemId = findItemId(await getEditorProject(token, wsSlug, projectId), "about-us.html");
  });

  // The routes whose shells own a secondary navigation column or a long form —
  // the shapes that outgrow a short viewport first.
  const routes = () => [
    { name: "dashboard", path: `/${wsSlug}`, ready: "nav-translate" },
    { name: "settings-index", path: `/${wsSlug}/settings`, ready: "nav-settings" },
    { name: "settings-languages", path: `/${wsSlug}/settings/languages`, ready: "nav-settings" },
    { name: "settings-members", path: `/${wsSlug}/settings/members`, ready: "nav-settings" },
    { name: "settings-system", path: `/${wsSlug}/settings/system`, ready: "nav-settings" },
    { name: "settings-billing", path: `/${wsSlug}/settings/billing`, ready: "nav-settings" },
    { name: "context-concepts", path: `/${wsSlug}/context/concepts`, ready: "nav-context" },
    { name: "context-voice", path: `/${wsSlug}/context/voice`, ready: "nav-context" },
    // Content memory is a Context surface now; `nav-memory` is not a rail
    // entry any more, and waiting on it only bought an 8s timeout per sweep.
    { name: "memory", path: `/${wsSlug}/context/memory`, ready: "nav-context" },
    { name: "project-detail", path: `/${wsSlug}/p/${projectId}/s/main`, ready: "nav-translate" },
    // The editor replaces the shell's scroll container with overflow-hidden and
    // manages its own panes, so it is the route most likely to lose a scroller.
    {
      name: "editor",
      path: `/${wsSlug}/p/${projectId}/s/main/${itemId}/translate`,
      ready: "view-switcher",
    },
  ];

  for (const viewport of VIEWPORTS) {
    test(`no clipped content at ${viewport.name} (${viewport.width}x${viewport.height})`, async ({
      page,
    }) => {
      test.setTimeout(240_000); // a full route sweep, not a single assertion
      await page.setViewportSize({ width: viewport.width, height: viewport.height });
      await injectAuthCookie(page, token);

      const failures: string[] = [];
      for (const route of routes()) {
        await page.goto(route.path);
        // Wait for the shell rather than the page body: the shell is what is
        // under test, and it mounts before any route's data resolves.
        await page
          .getByTestId(route.ready)
          .first()
          .waitFor({ state: "attached", timeout: 8000 })
          .catch(() => {
            // On a narrow viewport the sidebar collapses behind a trigger, so a
            // nav test id may never attach. Measure the route anyway rather than
            // skip the very width most likely to clip.
          });
        // Let layout settle so scrollHeight reflects the final tree.
        await page.waitForTimeout(300);

        const clips = await findClippedContent(page);
        if (clips.length > 0) {
          failures.push(report(route.path, viewport.name, clips));
          await page.screenshot({
            path: `test-results/layout-${viewport.name}-${route.name}.png`,
            fullPage: false,
          });
        }
      }

      expect(failures.join("\n\n"), failures.join("\n\n")).toBe("");
    });
  }

  test("the settings secondary nav can reach its last entry on a short viewport", async ({
    page,
  }) => {
    // The concrete symptom behind the structural check: the settings sub-nav is
    // long enough to outgrow a 720px-tall window, so its bottom entries were
    // unreachable. Asserting the last one is scrollable-to keeps the fix honest
    // even if the detector's tolerances are ever loosened.
    await page.setViewportSize({ width: 1280, height: 720 });
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/settings`);

    const lastEntry = page.getByTestId("subnav-system");
    await lastEntry.waitFor({ state: "attached", timeout: 20000 });
    await lastEntry.scrollIntoViewIfNeeded();
    await expect(lastEntry).toBeInViewport({ ratio: 0.9 });
  });
});
