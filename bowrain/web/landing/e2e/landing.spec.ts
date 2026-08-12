import { test, expect, type Page } from "@playwright/test";

// The landing page argues one thing across six sections and converts on one
// link. Both are the kind of thing that breaks without failing anything: a
// reordered import silently re-tells the argument, and a refactor that moves
// the capture off the click path silently drops every conversion. This suite
// exists for exactly those two.

/** The narrative beats, in the order App.tsx tells them. */
const NARRATIVE = ["hero", "rename", "how", "loop", "proof", "languages"];

/**
 * Every section that must carry an id, in DOM order, so a CTA inside one
 * attributes to it. The reference material a convinced buyer wants (product,
 * apps, licensing) sits between the fifth beat and the sixth; the sixth stays
 * immediately above the price.
 */
const ALL_SECTIONS = [
  "hero",
  "rename",
  "how",
  "loop",
  "proof",
  "product",
  "apps",
  "open-source",
  "languages",
  "pricing",
  "get-started",
];

/**
 * Collect the app-host URLs the page tries to open, and stop them: following
 * one would leave the page under test. Document requests only — the identity
 * probe in useAuthStatus hits the same host on mount and is not a conversion.
 */
async function blockAppNavigation(page: Page): Promise<string[]> {
  const attempted: string[] = [];
  await page.route("**://app.bowrain.cloud/**", (route) => {
    if (route.request().resourceType() === "document") attempted.push(route.request().url());
    return route.abort();
  });
  return attempted;
}

test.describe("landing narrative", () => {
  test("tells the six beats in order, and every section carries an id", async ({ page }) => {
    await page.goto("/");

    const order = await page.$$eval("section[id]", (nodes) => nodes.map((n) => n.id));
    expect(order).toEqual(ALL_SECTIONS);

    // The narrative beats keep their relative order even as tail sections move.
    const narrativePositions = NARRATIVE.map((id) => order.indexOf(id));
    expect(narrativePositions).toEqual([...narrativePositions].sort((a, b) => a - b));
    expect(narrativePositions.every((i) => i >= 0)).toBe(true);
  });

  test("the nav anchors resolve to sections that exist", async ({ page }) => {
    await page.goto("/");
    const hrefs = await page.$$eval("nav a[href^='#']", (nodes) =>
      nodes.map((n) => n.getAttribute("href")!).filter((h) => h !== "#"),
    );
    expect(hrefs.length).toBeGreaterThan(0);
    for (const href of new Set(hrefs)) {
      await expect(page.locator(`section${href}`)).toHaveCount(1);
    }
  });
});

test.describe("signup attribution", () => {
  test("carries inbound utm parameters across to the app", async ({ page }) => {
    const attempted = await blockAppNavigation(page);
    await page.goto("/?utm_source=newsletter&utm_campaign=r13&ref=ignored");

    await page.locator("#hero a[href^='https://app.bowrain.cloud']").first().click();
    await expect.poll(() => attempted.length).toBeGreaterThan(0);

    const url = new URL(attempted[0]);
    expect(url.searchParams.get("utm_source")).toBe("newsletter");
    expect(url.searchParams.get("utm_campaign")).toBe("r13");
    // Only utm_* is forwarded; arbitrary inbound query is not.
    expect(url.searchParams.get("ref")).toBeNull();
  });

  test("stamps a default source when the visitor arrived without one", async ({ page }) => {
    const attempted = await blockAppNavigation(page);
    await page.goto("/");

    await page.locator("#get-started a[href^='https://app.bowrain.cloud']").first().click();
    await expect.poll(() => attempted.length).toBeGreaterThan(0);
    expect(new URL(attempted[0]).searchParams.get("utm_source")).toBe("bowrain-landing");
  });
});

test.describe("the two interactive proofs", () => {
  test("switching the coordinate example changes the resolved point", async ({ page }) => {
    await page.goto("/");
    const how = page.locator("section#how");
    await how.scrollIntoViewIfNeeded();

    const answer = how.locator("div.overflow-x-auto");
    const before = await answer.innerText();
    await how.getByRole("button", { name: "API reference" }).click();
    await expect.poll(async () => answer.innerText()).not.toBe(before);
    await expect(answer).toContainText("product/api");
  });

  test("the voice check re-scores when the point changes", async ({ page }) => {
    await page.goto("/");
    const proof = page.locator("section#proof");
    await proof.scrollIntoViewIfNeeded();

    // The developer reference forbids the new name outright — a critical
    // finding, which costs far more than the help profile's minor ones.
    const score = proof.locator("div.text-5xl");
    const helpScore = Number(await score.innerText());
    await proof.getByRole("button", { name: "Developer reference" }).click();
    await expect.poll(async () => Number(await score.innerText())).toBeLessThan(helpScore);
  });
});
