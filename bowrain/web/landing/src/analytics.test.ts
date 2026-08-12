// @vitest-environment jsdom
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

// The conversion event is the one signal the page exists to produce, and it is
// the one that has already been lost once — to a refactor that moved the
// capture off the click path while leaving the link rewrite in place. The
// Playwright suite watches the URL that reaches the app, which that refactor
// would still have satisfied. This watches the capture itself.
//
// The shared module is mocked rather than driven with a key: what is under test
// is that our listener calls it, not that PostHog ingests.

const captureDocsEvent = vi.fn();
const initDocsAnalytics = vi.fn();
const captureDocsPageview = vi.fn();

vi.mock("@neokapi/docs-shared/analytics.ts", () => ({
  initDocsAnalytics,
  captureDocsEvent,
  captureDocsPageview,
}));

const { initAnalytics } = await import("./analytics");

/** A section-wrapped anchor, as every CTA on the page is. */
function anchorIn(sectionId: string, href: string): HTMLAnchorElement {
  const section = document.createElement("section");
  section.id = sectionId;
  const anchor = document.createElement("a");
  anchor.href = href;
  section.append(anchor);
  document.body.append(section);
  return anchor;
}

/** Click without letting jsdom attempt the navigation the anchor asks for. */
function click(anchor: HTMLAnchorElement): void {
  const event = new MouseEvent("click", { bubbles: true, cancelable: true });
  anchor.addEventListener("click", (e) => e.preventDefault(), { once: true });
  anchor.dispatchEvent(event);
}

describe("signup attribution", () => {
  beforeAll(() => {
    // The listener is installed once per page load, so install it once here:
    // a second initAnalytics would double every capture.
    initAnalytics();
  });

  beforeEach(() => {
    captureDocsEvent.mockClear();
    document.body.replaceChildren();
    window.history.replaceState({}, "", "/");
  });

  it("captures the conversion on the click path, attributed to its section", () => {
    const anchor = anchorIn("get-started", "https://app.bowrain.cloud/signup");

    click(anchor);

    expect(captureDocsEvent).toHaveBeenCalledTimes(1);
    const [event, properties] = captureDocsEvent.mock.calls[0];
    expect(event).toBe("signup_cta_clicked");
    expect(properties).toMatchObject({
      section: "get-started",
      narrative: "context-graph-monolingual-first",
    });
    expect(String(properties.href)).toContain("app.bowrain.cloud/signup");
  });

  it("forwards inbound utm parameters and captures the href that was opened", () => {
    window.history.replaceState({}, "", "/?utm_source=newsletter&utm_campaign=r13&ref=ignored");
    const anchor = anchorIn("hero", "https://app.bowrain.cloud/signup");

    click(anchor);

    const opened = new URL(anchor.href);
    expect(opened.searchParams.get("utm_source")).toBe("newsletter");
    expect(opened.searchParams.get("utm_campaign")).toBe("r13");
    expect(opened.searchParams.get("ref")).toBeNull();
    expect(captureDocsEvent.mock.calls[0][1].href).toBe(opened.toString());
  });

  it("stamps a default source when the visitor arrived without one", () => {
    const anchor = anchorIn("hero", "https://app.bowrain.cloud/signup");

    click(anchor);

    expect(new URL(anchor.href).searchParams.get("utm_source")).toBe("bowrain-landing");
  });

  it("leaves the visitor's own utm_source alone when the link already carries one", () => {
    window.history.replaceState({}, "", "/?utm_source=newsletter");
    const anchor = anchorIn("hero", "https://app.bowrain.cloud/signup?utm_source=partner");

    click(anchor);

    expect(new URL(anchor.href).searchParams.get("utm_source")).toBe("partner");
  });

  it("ignores links that do not leave for the app", () => {
    click(anchorIn("hero", "https://github.com/neokapi/neokapi"));
    click(anchorIn("hero", "#how"));

    expect(captureDocsEvent).not.toHaveBeenCalled();
  });

  it("attributes to null rather than throwing when a CTA sits outside a section", () => {
    const anchor = document.createElement("a");
    anchor.href = "https://app.bowrain.cloud/signup";
    document.body.append(anchor);

    click(anchor);

    expect(captureDocsEvent.mock.calls[0][1].section).toBeNull();
  });
});
