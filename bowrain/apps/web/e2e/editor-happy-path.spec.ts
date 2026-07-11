import * as fs from "fs";
import * as path from "path";
import { fileURLToPath } from "url";
import { test, expect, type Page } from "@playwright/test";
import {
  authenticate,
  getOrCreateWorkspace,
  createEditorProject,
  deleteAllEditorProjects,
  waitForServer,
} from "./helpers/api-client";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const BASE_URL = process.env.BOWRAIN_URL || "http://localhost:8080";

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
let projectId: string;

const FILE_NAME = "app-strings.json";
const BOGUS_FILE = "notes.xyz";
const FRENCH_TEXT = "Tableau de bord Acme";

/**
 * The launch smoke test (#867, epic 006 task 4c): ONE linear editor
 * happy path against the real Docker stack —
 *
 *   upload (incl. a bogus-extension file → per-file skip message, task 5)
 *   → open the editor → edit a target → approve it (review button, tasks 1-3)
 *   → RELOAD → review state persisted per locale → export the merged file.
 *
 * Flake hardening: single worker, web-first assertions only (no fixed
 * sleeps), and the approve step waits for the review PUT response before
 * reloading so the persistence assertion can never race the write.
 */
test.describe("Editor happy path", () => {
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
    await deleteAllEditorProjects(token, wsSlug);

    const proj = await createEditorProject(token, wsSlug, "Editor Happy Path", "en", ["fr", "de"]);
    projectId = proj.id;
  });

  test("upload → edit → approve → reload persists review → export", async ({ page }) => {
    // The full loop crosses upload, editor, review round-trip, and export;
    // give it headroom over the default 60s without ever sleeping.
    test.setTimeout(120_000);

    await injectAuthCookie(page, token);

    // ── 1. Upload a good JSON file + a bogus-extension file via the UI ──────
    await page.goto(`/${wsSlug}/p/${projectId}/s/main`);
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 15000 });

    const seedFile = path.resolve(__dirname, "seed", FILE_NAME);
    await page.locator('input[type="file"]').setInputFiles([
      {
        name: FILE_NAME,
        mimeType: "application/json",
        buffer: fs.readFileSync(seedFile),
      },
      {
        name: BOGUS_FILE,
        mimeType: "application/octet-stream",
        buffer: Buffer.from("not a localizable format"),
      },
    ]);

    // Task 5: the server names the skipped file and why; the UI surfaces it.
    const skippedAlert = page.getByTestId("upload-skipped-alert");
    await expect(skippedAlert).toBeVisible({ timeout: 15000 });
    await expect(skippedAlert).toContainText(BOGUS_FILE);
    await expect(skippedAlert).toContainText("unsupported file type");

    // The JSON imported; the bogus file did not become a project item.
    await expect(page.getByTestId(`file-row-${FILE_NAME}`)).toBeVisible({ timeout: 15000 });
    await expect(page.getByTestId(`file-row-${BOGUS_FILE}`)).not.toBeVisible();

    // ── 2. Open the editor (Visual view is the default) ─────────────────────
    await page.getByTestId(`open-file-${FILE_NAME}`).click();
    await expect(page.getByTestId("view-switcher")).toBeVisible({ timeout: 30000 });
    await expect(page.getByTestId("visual-editor-card")).toBeVisible({ timeout: 30000 });

    // ── 3. Edit the target of the first block (locale fr = default) ─────────
    await page.getByTestId("edit-btn").click();
    const targetEditor = page.getByTestId("unified-target-editor");
    await expect(targetEditor).toBeVisible();
    const editable = targetEditor.locator('[contenteditable="true"]');
    await editable.click();
    await expect(editable).toBeFocused();
    await editable.pressSequentially(FRENCH_TEXT);
    // The typed text must be in the editor document before saving.
    await expect(editable).toContainText(FRENCH_TEXT);

    // Saving persists the target and auto-advances to the next block.
    const savedTarget = page.waitForResponse(
      (r) => r.url().includes("/blocks/") && r.request().method() === "PUT" && r.ok(),
    );
    await page.getByTestId("unified-save").click();
    await savedTarget;

    // ── 4. Approve the edited block (review button, per-locale status) ──────
    // Save advanced the card to block 2 — step back to the block we edited.
    await page.getByTestId("prev-block-btn").click();
    await expect(page.getByTestId("target-display")).toContainText(FRENCH_TEXT);

    // Approve/Reject live in the card's Review mode (Translate|Enrich|Review).
    await page.getByRole("tab", { name: "Review" }).click();
    const reviewed = page.waitForResponse(
      (r) => r.url().includes("/review") && r.request().method() === "PUT" && r.ok(),
    );
    await page.getByTestId("approve-btn").click();
    await reviewed;
    await expect(page.getByTestId("progress-text")).toContainText("1 reviewed");

    // ── 5. RELOAD — the review decision must survive a full page reload ─────
    await page.reload();
    await expect(page.getByTestId("view-switcher")).toBeVisible({ timeout: 30000 });
    await expect(page.getByTestId("visual-editor-card")).toBeVisible({ timeout: 30000 });

    // Server returned per-locale Target.Status for fr: the progress breakdown
    // counts it and the (still-selected-first) block's card shows the badge.
    await expect(page.getByTestId("progress-text")).toContainText("1 reviewed");
    await expect(page.getByTestId("target-display")).toContainText(FRENCH_TEXT);
    await expect(
      page.getByTestId("visual-editor-card").getByText("Reviewed", { exact: true }),
    ).toBeVisible();

    // ── 6. Export ────────────────────────────────────────────────────────────
    // Server-side merged-file export is deliberately unimplemented (source
    // bytes are not stored server-side since #136; `kapi pull` is the export
    // path — the Export button works in the desktop app, which has the local
    // file). The web contract is an EXPLICIT notice, never a silent no-op.
    await page.getByTestId("export-btn").click();
    await expect(page.getByText("Couldn't export the file")).toBeVisible();
    await expect(page.getByText(/kapi pull/)).toBeVisible();

    // Assert the merged content through the served API instead: the block
    // carries the edited fr target with per-locale reviewed status, and the
    // OTHER target locale (de) is untouched by the fr review (task 3).
    const blocksResp = await fetch(
      `${BASE_URL}/api/v1/${wsSlug}/${projectId}/blocks/main?item=${encodeURIComponent(FILE_NAME)}`,
      { headers: { Authorization: `Bearer ${token}` } },
    );
    expect(blocksResp.ok).toBe(true);
    const blocks = (await blocksResp.json()) as Array<{
      targets?: Record<string, { text?: string; status?: string }>;
    }>;
    const edited = blocks.find((b) => b.targets?.fr?.text === FRENCH_TEXT);
    expect(edited, "the edited block must be in the merged payload").toBeTruthy();
    expect(edited?.targets?.fr?.status).toBe("reviewed");
    expect(edited?.targets?.de?.status ?? "").toBe("");
  });
});
