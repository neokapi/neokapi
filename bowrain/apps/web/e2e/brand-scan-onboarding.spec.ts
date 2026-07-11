import { test, expect, type Page } from "@playwright/test";
// Leaf subpath export bypasses the @neokapi/ui barrel (which transitively
// imports JSON vocabularies that break Node-ESM Playwright collection).
// See bowrain/packages/ui/package.json `exports."./test-ids"`.
import { TEST_IDS } from "@neokapi/ui/test-ids";
import { authenticate, getOrCreateWorkspace, waitForServer } from "./helpers/api-client";

const BASE_URL = process.env.BOWRAIN_URL || "http://localhost:8080";
const API = `${BASE_URL}/api/v1`;

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

let token: string;
let wsSlug: string;

/**
 * Corpus with recurring capitalized terms (so the offline demo provider's
 * vocabulary heuristic yields deterministic candidates, incl. "Bowrain"),
 * contractions, and first-person-plural voice. Mirrors the Go worker test
 * (bowrain/jobs/brandscan_worker_test.go, demoPasteText).
 */
const PASTE_TEXT =
  "We're building Bowrain for teams that care about voice. " +
  "Bowrain keeps your copy on-brand across every locale. " +
  "With Bowrain, we ship faster and we don't compromise on tone!";

const MD_FILE = "voice-guide.md";
const MD_CONTENT = "# Voice\n\nWe write plainly. We're direct and Bowrain-first. Bowrain wins.";

/**
 * A tiny public repository (2 KB) with a `main` branch and a README.md — the
 * worker's git connector clones it with `--branch main` and harvests markdown.
 * This is the one path that exercises the git binary added to the
 * bowrain-worker image (bowrain/docker/bowrain-worker/Dockerfile).
 */
const REPO_URL = "https://github.com/octocat/Spoon-Knife";

/**
 * AI brand onboarding (epic 016): the scan → review → approve flow against
 * the real Docker stack (server + NATS + bowrain-worker with the offline
 * deterministic demo provider).
 *
 *   uploads endpoint (allowlist + skipped reasons)
 *   → intake page (paste + file + repository) → start
 *   → polled job page → review (confidence badges, sources, candidate terms)
 *   → live tester (stateless check-draft) → approve
 *   → brand profile created + selected terms become concepts
 *   → failure path: all sources unreadable → failed job + retry affordance.
 */
test.describe("Brand scan onboarding", () => {
  // lg+ viewport so the review's right rail (preview + live tester) renders.
  test.use({ viewport: { width: 1440, height: 900 } });

  test.beforeAll(async () => {
    await waitForServer();
    token = await authenticate();
    const ws = await getOrCreateWorkspace(
      token,
      process.env.BOWRAIN_WORKSPACE_NAME || "Acme Inc.",
      process.env.BOWRAIN_WORKSPACE || "acme",
    );
    wsSlug = ws.slug;
  });

  test("uploads endpoint stores allowed files and names skipped ones", async () => {
    // API-level contract: one allowed markdown file, one disallowed
    // extension, one deferred type (pdf) — the endpoint stores the first and
    // names the other two in "skipped" with per-file reasons.
    const form = new FormData();
    form.append("files", new Blob([MD_CONTENT], { type: "text/markdown" }), MD_FILE);
    form.append(
      "files",
      new Blob(["binary junk"], { type: "application/octet-stream" }),
      "notes.xyz",
    );
    form.append(
      "files",
      new Blob(["%PDF-1.4 stub"], { type: "application/pdf" }),
      "brand-deck.pdf",
    );

    const resp = await fetch(`${API}/${wsSlug}/brand-scans/uploads`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}` },
      body: form,
    });
    expect(resp.status).toBe(200);
    const body = (await resp.json()) as {
      uploads: Array<{ key: string; filename: string; size: number }>;
      skipped?: Array<{ name: string; reason: string }>;
    };

    expect(body.uploads).toHaveLength(1);
    expect(body.uploads[0].filename).toBe(MD_FILE);
    expect(body.uploads[0].key).toBeTruthy();
    expect(body.uploads[0].size).toBe(MD_CONTENT.length);

    expect(body.skipped).toHaveLength(2);
    const skippedByName = new Map(body.skipped!.map((s) => [s.name, s.reason]));
    expect(skippedByName.get("notes.xyz")).toContain("unsupported file type");
    expect(skippedByName.get("brand-deck.pdf")).toContain("deferred");
  });

  test("scan intake requires at least one source", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/brand/scan`);
    await expect(page.getByTestId(TEST_IDS.brandScan.input)).toBeVisible({ timeout: 15000 });
    await expect(page.getByTestId(TEST_IDS.brandScan.start)).toBeDisabled();
  });

  test("paste + file + repository → progress → review → live tester → approve", async ({
    page,
  }) => {
    // The scan crosses upload, a real git clone in the worker, the demo
    // provider, polling, and the approve round-trip; give it headroom without
    // ever sleeping.
    test.setTimeout(180_000);
    const profileName = `Acme Voice E2E ${Date.now()}`;

    await injectAuthCookie(page, token);

    // ── 1. Intake: paste text, drop a markdown file, point at a repository ──
    await page.goto(`/${wsSlug}/brand/scan`);
    await expect(page.getByTestId(TEST_IDS.brandScan.input)).toBeVisible({ timeout: 15000 });

    await page.getByTestId(TEST_IDS.brandScan.input).fill(PASTE_TEXT);
    await expect(page.getByTestId(TEST_IDS.brandScan.fileDrop)).toBeVisible();
    await page
      .locator('input[type="file"]')
      .setInputFiles([
        { name: MD_FILE, mimeType: "text/markdown", buffer: Buffer.from(MD_CONTENT) },
      ]);
    await expect(page.getByText(MD_FILE)).toBeVisible();
    await page.getByPlaceholder("https://github.com/acme/website").fill(REPO_URL);
    await page.locator("#brand-scan-profile-name").fill(profileName);

    // ── 2. Start: files upload to the blob store, then the job is enqueued ──
    const started = page.waitForResponse(
      (r) =>
        r.url().includes("/brand-scans") && r.request().method() === "POST" && r.status() === 202,
    );
    await page.getByTestId(TEST_IDS.brandScan.start).click();
    await started;
    await expect(page).toHaveURL(/\/brand\/scan\/[^/]+$/, { timeout: 15000 });

    // ── 3. The job page polls: progress card first, review when completed ──
    // The demo provider is fast, so the progress card can flip to the review
    // between polls — accept either, then wait for the review itself.
    await expect(
      page.getByTestId(TEST_IDS.brandScan.progress).or(page.getByTestId(TEST_IDS.brandScan.review)),
    ).toBeVisible({ timeout: 20000 });
    const review = page.getByTestId(TEST_IDS.brandScan.review);
    await expect(review).toBeVisible({ timeout: 90000 });

    // ── 4. Provenance: all three sources were read (repo runes > 0 proves ──
    // the worker image's git binary cloned and harvested the README).
    const sourceRow = (label: string) => review.locator("li", { hasText: label });
    await expect(sourceRow("pasted text")).toBeVisible();
    await expect(sourceRow(MD_FILE)).toBeVisible();
    await expect(sourceRow(REPO_URL)).toBeVisible();
    await expect(sourceRow(REPO_URL)).toContainText(/[1-9][\d,]* characters/);

    // ── 5. Evidence: every inferred field carries a confidence badge ──
    await expect(page.getByTestId(TEST_IDS.brandScan.fieldConfidence)).toHaveCount(4);
    for (const badge of await page.getByTestId(TEST_IDS.brandScan.fieldConfidence).all()) {
      await expect(badge).toContainText(/% confidence/);
    }

    // The drafted name came through from the intake form.
    await expect(page.locator("#scan-profile-name")).toHaveValue(profileName);

    // ── 6. Candidate glossary: the corpus' recurring term surfaced ──
    const termRows = page.getByTestId(TEST_IDS.brandScan.termRow);
    expect(await termRows.count()).toBeGreaterThan(0);
    await expect(termRows.filter({ hasText: "Bowrain" }).first()).toBeVisible();

    // ── 7. Live tester: the stateless check-draft scores sample copy ──
    const tester = page.getByTestId(TEST_IDS.brandScan.liveTester);
    await expect(tester).toBeVisible();
    const checked = page.waitForResponse(
      (r) => r.url().includes("/brand-scans/check-draft") && r.ok(),
    );
    await tester.locator("textarea").fill("We ship fast and we keep your copy on-brand.");
    await checked;
    await expect(tester.getByText(/No rule violations|against the draft rules/)).toBeVisible();

    // ── 8. Approve: creates the profile + selected terms, then navigates ──
    const created = page.waitForResponse(
      (r) => r.url().includes("/brand-profiles") && r.request().method() === "POST" && r.ok(),
    );
    await page.getByTestId(TEST_IDS.brandScan.approve).click();
    await created;
    await expect(page).toHaveURL(/\/brand\/voice\/[^/]+$/, { timeout: 30000 });
    // The profile editor loads the created profile (name renders in its form).
    await expect(page.locator("#profile-name")).toHaveValue(profileName, { timeout: 15000 });

    // The approved profile is persisted, and the selected candidate terms
    // became workspace concepts (assert through the served API).
    const profilesResp = await fetch(`${API}/${wsSlug}/brand-profiles`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(profilesResp.ok).toBe(true);
    const profiles = (await profilesResp.json()) as Array<{ name: string }>;
    expect(profiles.some((p) => p.name === profileName)).toBe(true);

    const conceptsResp = await fetch(`${API}/${wsSlug}/concepts`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(conceptsResp.ok).toBe(true);
    expect(JSON.stringify(await conceptsResp.json())).toContain("Bowrain");
  });

  test("scan whose sources are all unreadable fails with a retry affordance", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/brand/scan`);
    await expect(page.getByTestId(TEST_IDS.brandScan.urlInput)).toBeVisible({ timeout: 15000 });

    // The loopback address passes the client-side https check but is refused
    // by the worker's SSRF guard, so the only source yields no text and the
    // job fails with a user-facing reason.
    await page.getByTestId(TEST_IDS.brandScan.urlInput).fill("https://127.0.0.1/");
    await page.getByTestId(TEST_IDS.brandScan.urlInput).press("Enter");
    await expect(page.getByText("https://127.0.0.1/")).toBeVisible();

    await page.getByTestId(TEST_IDS.brandScan.start).click();
    await expect(page).toHaveURL(/\/brand\/scan\/[^/]+$/, { timeout: 15000 });

    const progress = page.getByTestId(TEST_IDS.brandScan.progress);
    await expect(progress).toBeVisible({ timeout: 20000 });
    await expect(progress.getByText("The scan could not complete")).toBeVisible({
      timeout: 60000,
    });
    await expect(progress.getByText(/no readable brand material/)).toBeVisible();
    await expect(progress.getByRole("button", { name: "Try again" })).toBeVisible();
  });
});
