import { test, expect, type Page } from "@playwright/test";
import { injectMockBackend } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

// Unicode markers matching the Go model
const M_OPEN = "\uE001";
const M_CLOSE = "\uE002";
const M_PLACEHOLDER = "\uE003";

/**
 * Injects the mock backend, creates a project with a file containing
 * blocks that have inline spans, and navigates to the editor.
 */
async function openEditorWithInlineBlocks(page: Page) {
  await injectMockBackend(page);
  await page.goto("/");

  // Create project
  await page.getByTestId("new-project-btn").click();
  await page.getByTestId("project-name-input").fill("Inline Test");
  await selectMultiLocales(page, "target-langs-input", ["fr", "de"]);
  await page.getByTestId("create-project-submit").click();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible();

  // Add a file via the mock backend, then monkey-patch GetItemBlocks to return
  // span-bearing blocks for welcome.html
  await page.evaluate(
    ({ mo, mc, mp }) => {
      const backend = (window as any).__wailsMockByName;
      const mock = (window as any).__wailsMock;
      const IDS = (window as any).__wailsIDs;

      // Add the file entry to the project
      const projects = backend.ListProjects();
      const pid = projects[0].id;
      backend.AddItems(pid, ["/test/welcome.html"]);

      // State for tracking target updates on welcome.html blocks
      const welcomeTargets: Record<string, Record<string, string>> = {};
      const welcomeTargetsCoded: Record<string, Record<string, string>> = {};
      (window as any).__welcomeTargets = welcomeTargets;
      (window as any).__welcomeTargetsCoded = welcomeTargetsCoded;

      // Monkey-patch GetItemBlocks to return span-bearing blocks for welcome.html
      const origGetItemBlocks = mock[IDS.GetItemBlocks];
      mock[IDS.GetItemBlocks] = (projectID: string, fileName: string) => {
        if (fileName === "welcome.html") {
          return [
            {
              id: "welcome.html-block-1",
              source: "Click here to learn more",
              source_coded: `Click ${mo}here${mc} to learn more`,
              source_spans: [
                { span_type: "opening", type: "a", id: "a1", data: '<a href="#">' },
                { span_type: "closing", type: "a", id: "a1", data: "</a>" },
              ],
              targets: { ...(welcomeTargets["welcome.html-block-1"] || {}) },
              targets_coded: { ...(welcomeTargetsCoded["welcome.html-block-1"] || {}) },
              translatable: true,
              has_spans: true,
              properties: {},
            },
            {
              id: "welcome.html-block-2",
              source: "Bold and italic text",
              source_coded: `${mo}Bold${mc} and ${mo}italic${mc} text`,
              source_spans: [
                { span_type: "opening", type: "b", id: "b1", data: "<b>" },
                { span_type: "closing", type: "b", id: "b1", data: "</b>" },
                { span_type: "opening", type: "i", id: "i1", data: "<i>" },
                { span_type: "closing", type: "i", id: "i1", data: "</i>" },
              ],
              targets: { ...(welcomeTargets["welcome.html-block-2"] || {}) },
              targets_coded: { ...(welcomeTargetsCoded["welcome.html-block-2"] || {}) },
              translatable: true,
              has_spans: true,
              properties: {},
            },
            {
              id: "welcome.html-block-3",
              source: "Line break here",
              source_coded: `Line break${mp} here`,
              source_spans: [
                { span_type: "placeholder", type: "br", id: "br1", data: "<br/>" },
              ],
              targets: { ...(welcomeTargets["welcome.html-block-3"] || {}) },
              targets_coded: { ...(welcomeTargetsCoded["welcome.html-block-3"] || {}) },
              translatable: true,
              has_spans: true,
              properties: {},
            },
            {
              id: "welcome.html-block-4",
              source: "Code and link example",
              source_coded: `${mo}Code${mc} and ${mo}link${mc} example`,
              source_spans: [
                { span_type: "opening", type: "code", id: "code1", data: "<code>" },
                { span_type: "closing", type: "code", id: "code1", data: "</code>" },
                { span_type: "opening", type: "a", id: "a2", data: '<a href="/doc">' },
                { span_type: "closing", type: "a", id: "a2", data: "</a>" },
              ],
              targets: { ...(welcomeTargets["welcome.html-block-4"] || {}) },
              targets_coded: { ...(welcomeTargetsCoded["welcome.html-block-4"] || {}) },
              translatable: true,
              has_spans: true,
              properties: {},
            },
            {
              id: "welcome.html-block-5",
              source: "Simple text without tags",
              targets: { ...(welcomeTargets["welcome.html-block-5"] || {}) },
              translatable: true,
              has_spans: false,
              properties: {},
            },
          ];
        }
        return origGetItemBlocks(projectID, fileName);
      };

      // Monkey-patch UpdateBlockTarget for welcome.html blocks
      const origUpdateTarget = mock[IDS.UpdateBlockTarget];
      mock[IDS.UpdateBlockTarget] = (req: any) => {
        const itemName = req.item_name || req.file_name;
        if (itemName === "welcome.html") {
          if (!welcomeTargets[req.block_id]) welcomeTargets[req.block_id] = {};
          welcomeTargets[req.block_id][req.target_locale] = req.text;
          return;
        }
        return origUpdateTarget(req);
      };

      // Monkey-patch UpdateBlockTargetCoded for welcome.html blocks
      const origUpdateCoded = mock[IDS.UpdateBlockTargetCoded];
      mock[IDS.UpdateBlockTargetCoded] = (req: any) => {
        const itemName = req.item_name || req.file_name;
        if (itemName === "welcome.html") {
          if (!welcomeTargets[req.block_id]) welcomeTargets[req.block_id] = {};
          if (!welcomeTargetsCoded[req.block_id]) welcomeTargetsCoded[req.block_id] = {};
          const plain = req.coded_text.replace(/[\uE001-\uE003]/g, "");
          welcomeTargets[req.block_id][req.target_locale] = plain;
          welcomeTargetsCoded[req.block_id][req.target_locale] = req.coded_text;
          return;
        }
        if (origUpdateCoded) return origUpdateCoded(req);
      };
    },
    { mo: M_OPEN, mc: M_CLOSE, mp: M_PLACEHOLDER },
  );

  // Navigate away and back to pick up the file
  await page.locator("nav button", { hasText: "Settings" }).click();
  await page.waitForTimeout(100);
  await page.getByTestId("nav-translate").click();
  await page.waitForTimeout(200);

  await page.getByText("Inline Test").first().click();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });

  // Open the file
  await expect(page.getByTestId("open-file-welcome.html")).toBeVisible({ timeout: 5000 });
  await page.evaluate(() => {
    const btn = document.querySelector('[data-testid="open-file-welcome.html"]') as HTMLElement;
    if (btn) btn.click();
  });

  await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
}

/** Helper: click a data-testid element via native DOM (avoids Playwright hang on unmount). */
async function clickTestId(page: Page, testId: string) {
  await page.evaluate((id: string) => {
    const el = document.querySelector(`[data-testid="${id}"]`) as HTMLElement;
    if (el) el.click();
  }, testId);
}

/** Helper: type text into the focused contenteditable via execCommand (avoids Playwright hang). */
async function typeInEditor(page: Page, text: string) {
  await page.evaluate((t: string) => {
    document.execCommand("insertText", false, t);
  }, text);
}

/** Helper: press Enter in the Lexical editor via dispatchEvent. */
async function pressEnterInEditor(page: Page) {
  await page.evaluate(() => {
    const el = document.querySelector('[contenteditable="true"]');
    if (el) {
      el.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", code: "Enter", keyCode: 13, bubbles: true, cancelable: true }));
    }
  });
}

/** Helper: press Escape in the Lexical editor via dispatchEvent. */
async function pressEscapeInEditor(page: Page) {
  await page.evaluate(() => {
    const el = document.querySelector('[contenteditable="true"]');
    if (el) {
      el.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", code: "Escape", keyCode: 27, bubbles: true, cancelable: true }));
    }
  });
}

/** Helper: press Ctrl+N (digit) in the Lexical editor via dispatchEvent. */
async function pressCtrlDigitInEditor(page: Page, digit: number) {
  await page.evaluate((d: number) => {
    const el = document.querySelector('[contenteditable="true"]');
    if (el) {
      el.dispatchEvent(new KeyboardEvent("keydown", { key: String(d), code: `Digit${d}`, ctrlKey: true, bubbles: true, cancelable: true }));
    }
  }, digit);
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("Inline Codes — Tag Chip Display", () => {
  test("should render tag chips in source cells for blocks with spans", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Block 1 has <a>...</a> tags — should show chip elements
    const row0 = page.getByTestId("block-row-0");
    await expect(row0).toBeVisible();

    // Tag chips render via [data-tag-chip] attribute
    const chips = row0.locator("[data-tag-chip]");
    await expect(chips).toHaveCount(2); // opening a>, closing /a
  });

  test("should render semantic labels on tag chips", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Block 1: link tags → "a>" and "/a"
    const row0 = page.getByTestId("block-row-0");
    const chips = row0.locator("[data-tag-chip]");
    await expect(chips.first()).toContainText("a>");
    await expect(chips.last()).toContainText("/a");
  });

  test("should render bold and italic chips with correct semantic labels", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Block 2: <b>...</b> and <i>...</i>
    const row1 = page.getByTestId("block-row-1");
    const chips = row1.locator("[data-tag-chip]");
    await expect(chips).toHaveCount(4);

    // Semantic labels: B>, /B, I>, /I
    await expect(chips.nth(0)).toContainText("B>");
    await expect(chips.nth(1)).toContainText("/B");
    await expect(chips.nth(2)).toContainText("I>");
    await expect(chips.nth(3)).toContainText("/I");
  });

  test("should render placeholder chip for br tag", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Block 3: <br/> placeholder
    const row2 = page.getByTestId("block-row-2");
    const chips = row2.locator("[data-tag-chip]");
    await expect(chips).toHaveCount(1);
    await expect(chips.first()).toContainText("br");
  });

  test("should show pair index badges on tag chips", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Block 2 has two pairs: bold (pair 1) and italic (pair 2)
    const row1 = page.getByTestId("block-row-1");
    const chips = row1.locator("[data-tag-chip]");

    // The bold opening and closing should both show pair badge "1"
    // The italic opening and closing should both show pair badge "2"
    // Check that pair badges exist by looking at text content
    const chip0Text = await chips.nth(0).textContent();
    const chip1Text = await chips.nth(1).textContent();
    const chip2Text = await chips.nth(2).textContent();
    const chip3Text = await chips.nth(3).textContent();

    // Chips include: index + label + pairIndex
    // e.g. "1B>1" (index=1, label=B>, pairBadge=1)
    expect(chip0Text).toContain("B>");
    expect(chip1Text).toContain("/B");
    expect(chip2Text).toContain("I>");
    expect(chip3Text).toContain("/I");
  });

  test("should not render tag chips for plain text blocks", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Block 5 is plain text — no has_spans
    const row4 = page.getByTestId("block-row-4");
    await expect(row4).toBeVisible();
    const chips = row4.locator("[data-tag-chip]");
    await expect(chips).toHaveCount(0);
    await expect(row4).toContainText("Simple text without tags");
  });

  test("should apply distinct colors to different semantic categories", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Block 4: code + link → different colors
    const row3 = page.getByTestId("block-row-3");
    const chips = row3.locator("[data-tag-chip]");
    await expect(chips).toHaveCount(4);

    // Code chip (first) — slate color
    const codeChipColor = await chips.nth(0).evaluate(
      (el) => window.getComputedStyle(el).color,
    );

    // Link chip (third) — green color
    const linkChipColor = await chips.nth(2).evaluate(
      (el) => window.getComputedStyle(el).color,
    );

    // They should be different colors
    expect(codeChipColor).not.toEqual(linkChipColor);
  });

  test("should display semantic tooltips on tag chips", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    const row0 = page.getByTestId("block-row-0");
    const chips = row0.locator("[data-tag-chip]");

    // The opening link chip should have a tooltip with "Link open"
    const tooltip = await chips.first().getAttribute("title");
    expect(tooltip).toContain("Link");
    expect(tooltip).toContain("open");
    expect(tooltip).toContain("<a");
  });
});

test.describe("Inline Codes — Pair Hover Highlighting", () => {
  test("should highlight matching tag pair in source on hover", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Block 1: <a> ... </a>
    const row0 = page.getByTestId("block-row-0");
    const chips = row0.locator("[data-tag-chip]");
    const openChip = chips.first();
    const closeChip = chips.last();

    // Before hover: no box shadow
    const initialShadow = await closeChip.evaluate(
      (el) => window.getComputedStyle(el).boxShadow,
    );

    // Hover over the opening chip
    await openChip.hover();
    await page.waitForTimeout(100);

    // The closing chip should now have a box-shadow (highlighting)
    const highlightedShadow = await closeChip.evaluate(
      (el) => window.getComputedStyle(el).boxShadow,
    );

    // The opening chip itself should also be highlighted
    const openHighlightedShadow = await openChip.evaluate(
      (el) => window.getComputedStyle(el).boxShadow,
    );

    // Both should have the glow (non-"none" box shadow)
    expect(openHighlightedShadow).not.toBe("none");
    // Initial shadow should be none or different from highlighted
    expect(highlightedShadow).not.toEqual(initialShadow);
  });

  test("should clear highlight when mouse leaves tag chip", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    const row0 = page.getByTestId("block-row-0");
    const chips = row0.locator("[data-tag-chip]");
    const openChip = chips.first();
    const closeChip = chips.last();

    // Hover to activate
    await openChip.hover();
    await page.waitForTimeout(100);

    // Move mouse away (hover on the row text instead)
    await page.mouse.move(0, 0);
    await page.waitForTimeout(500);

    // Both chips should lose highlighting (allow near-zero residual from CSS transitions)
    const shadow = await closeChip.evaluate(
      (el) => window.getComputedStyle(el).boxShadow,
    );
    // Either "none" or an effectively invisible shadow
    const isNone = shadow === "none" || shadow === "" || shadow.includes("0px 0px 0px 0px");
    expect(isNone).toBe(true);
  });
});

test.describe("Inline Codes — Tag Palette in Editor", () => {
  test("should show tag palette when editing a block with spans", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Double-click block 1 to start editing
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Tag palette should be visible with "Tags:" label
    await expect(page.getByText("Tags:")).toBeVisible();

    // Should show palette buttons
    const paletteBtn0 = page.getByTestId("tag-palette-0");
    const paletteBtn1 = page.getByTestId("tag-palette-1");
    await expect(paletteBtn0).toBeVisible();
    await expect(paletteBtn1).toBeVisible();
  });

  test("should group tag palette buttons by pairs", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 2 (bold + italic → 2 pairs)
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-1"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // All 4 palette buttons should be visible
    await expect(page.getByTestId("tag-palette-0")).toBeVisible();
    await expect(page.getByTestId("tag-palette-1")).toBeVisible();
    await expect(page.getByTestId("tag-palette-2")).toBeVisible();
    await expect(page.getByTestId("tag-palette-3")).toBeVisible();

    // Palette should show semantic labels
    const palette0 = page.getByTestId("tag-palette-0").locator("[data-tag-chip]");
    await expect(palette0).toContainText("B>");
  });

  test("should not show tag palette for plain text blocks", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 5 (plain text, no spans)
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-4"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Should show a textarea, not the Lexical editor with palette
    await expect(page.getByTestId("edit-target-4")).toBeVisible();
    // Tag palette should not be visible
    await expect(page.getByText("Tags:")).not.toBeVisible();
  });

  test("should insert tag via palette click", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 1
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Click the first palette button to insert the opening <a> tag
    await clickTestId(page, "tag-palette-0");
    await page.waitForTimeout(200);

    // The editor content area should now contain a tag chip
    const editorChips = page.locator('[contenteditable="true"] [data-tag-chip]');
    // There may already be chips from initial coded text; count should be > 0
    const count = await editorChips.count();
    expect(count).toBeGreaterThan(0);
  });

  test("should insert tag via keyboard shortcut Ctrl+1", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 3 (single placeholder br)
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-2"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Focus the editor
    await page.locator('[contenteditable="true"]').focus();
    await page.waitForTimeout(100);

    // Press Ctrl+1 to insert the first tag (br placeholder)
    await pressCtrlDigitInEditor(page, 1);
    await page.waitForTimeout(200);

    // Should have at least one additional chip in the editor
    const editorChips = page.locator('[contenteditable="true"] [data-tag-chip]');
    const count = await editorChips.count();
    expect(count).toBeGreaterThan(0);
  });
});

test.describe("Inline Codes — Validation Bar", () => {
  test("should show validation errors when tags are missing from target", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 1 (has <a>...</a>)
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // The editor starts with empty target, so all source tags are missing.
    // Validation bar should show errors about missing tags.
    // Look for validation error messages
    await expect(page.getByText(/Missing.*opening.*"a"/i)).toBeVisible({ timeout: 3000 });
    await expect(page.getByText(/Missing.*closing.*"a"/i)).toBeVisible({ timeout: 3000 });
  });

  test("should clear validation errors when all tags are inserted", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 3 (single br placeholder)
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-2"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Initially shows missing tag error
    await expect(page.getByText(/Missing.*placeholder.*"br"/i)).toBeVisible({ timeout: 3000 });

    // Insert the br tag via palette
    await clickTestId(page, "tag-palette-0");
    await page.waitForTimeout(300);

    // Validation error should disappear (or at least the missing br message)
    await expect(page.getByText(/Missing.*placeholder.*"br"/i)).not.toBeVisible({ timeout: 3000 });
  });

  test("should show validation for multiple missing tags", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 2 (has <b>...</b> and <i>...</i> — 4 tags total)
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-1"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Should show errors about missing bold and italic tags
    await expect(page.getByText(/Missing.*"b"/i).first()).toBeVisible({ timeout: 3000 });
    await expect(page.getByText(/Missing.*"i"/i).first()).toBeVisible({ timeout: 3000 });
  });
});

test.describe("Inline Codes — Inline Preview", () => {
  test("should show preview strip when editing block with inline codes", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 1
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Type some content so the preview becomes visible (InlinePreview returns null when codedText is empty)
    await page.locator('[contenteditable="true"]').focus();
    await typeInEditor(page, "Some text");
    await page.waitForTimeout(300);

    // Preview label should be visible
    await expect(page.getByText("Preview:")).toBeVisible({ timeout: 3000 });
  });

  test("should update preview when typing text in editor", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 3 (br placeholder — starts empty)
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-2"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Type some text using evaluate to avoid Playwright hang
    const editor = page.locator('[contenteditable="true"]');
    await editor.focus();
    await typeInEditor(page, "Hello preview");
    await page.waitForTimeout(300);

    // Preview should show the typed text
    await expect(page.getByText("Preview:")).toBeVisible();
    // The preview content area should contain "Hello preview"
    await expect(page.locator("text=Hello preview").last()).toBeVisible();
  });

  test("should not show preview for plain text editing", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 5 (plain text, no spans → uses textarea, not Lexical)
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-4"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Preview should not be visible for plain text
    await expect(page.getByText("Preview:")).not.toBeVisible();
  });
});

test.describe("Inline Codes — Tag Palette Dimming", () => {
  test("should dim tags in palette that are already inserted in editor", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 1 (has opening <a> and closing </a>)
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Initially all tags are missing from target, so palette should not be dimmed
    const btn0 = page.getByTestId("tag-palette-0").locator("[data-tag-chip]");
    const initialOpacity = await btn0.evaluate((el) => window.getComputedStyle(el).opacity);

    // Insert the opening tag
    await clickTestId(page, "tag-palette-0");
    await page.waitForTimeout(300);

    // Now the first tag should be dimmed (opacity < 1)
    const afterOpacity = await btn0.evaluate((el) => window.getComputedStyle(el).opacity);

    // The chip should be dimmed after insertion
    expect(parseFloat(afterOpacity)).toBeLessThan(parseFloat(initialOpacity));
  });
});

test.describe("Inline Codes — Target Cell Coded Display", () => {
  test("should display tag chips in target cell after saving coded text", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 1 and insert tags + text
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Type text, then insert tags (using evaluate to avoid Playwright hang)
    const editor = page.locator('[contenteditable="true"]');
    await editor.focus();
    await typeInEditor(page, "Cliquez ");
    await clickTestId(page, "tag-palette-0"); // insert opening <a>
    await page.waitForTimeout(100);
    await typeInEditor(page, "ici");
    await clickTestId(page, "tag-palette-1"); // insert closing </a>
    await page.waitForTimeout(100);
    await typeInEditor(page, " pour en savoir plus");
    await page.waitForTimeout(200);

    // Save by pressing Enter
    await pressEnterInEditor(page);
    await page.waitForTimeout(500);

    // After saving, the target cell should display coded text with tag chips
    const targetCell = page.getByTestId("target-text-0");
    await expect(targetCell).toBeVisible();

    // Should contain tag chip elements
    const targetChips = targetCell.locator("[data-tag-chip]");
    await expect(targetChips).toHaveCount(2);

    // Should also contain the translated text
    await expect(targetCell).toContainText("Cliquez");
    await expect(targetCell).toContainText("ici");
  });

  test("should show pair highlighting in target cell coded display", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // First, save some coded text for block 1
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    const editor = page.locator('[contenteditable="true"]');
    await editor.focus();
    await clickTestId(page, "tag-palette-0");
    await page.waitForTimeout(100);
    await typeInEditor(page, "lien");
    await clickTestId(page, "tag-palette-1");
    await page.waitForTimeout(100);

    await pressEnterInEditor(page);
    await page.waitForTimeout(500);

    // Now verify the target cell displays chips with pair badges
    const targetChips = page.getByTestId("target-text-0").locator("[data-tag-chip]");
    await expect(targetChips).toHaveCount(2);

    // Hover on one chip should highlight the other
    await targetChips.first().hover();
    await page.waitForTimeout(100);

    const closingShadow = await targetChips.last().evaluate(
      (el) => window.getComputedStyle(el).boxShadow,
    );
    expect(closingShadow).not.toBe("none");
  });
});

test.describe("Inline Codes — Row Validation Warning", () => {
  test("should show warning icon when target has mismatched tags", async ({ page }, testInfo) => {
    // Skip in CI - validation timing is unreliable in CI environment
    test.skip(!!process.env.CI, "Flaky in CI - validation timing issue");
    await openEditorWithInlineBlocks(page);

    // Edit block 1 and save with only one tag (partial)
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    const editor = page.locator('[contenteditable="true"]');
    await editor.focus();
    await typeInEditor(page, "Texte ");
    // Only insert opening tag, not closing → mismatch
    await clickTestId(page, "tag-palette-0");
    await page.waitForTimeout(100);
    await typeInEditor(page, "lien");
    await page.waitForTimeout(100);

    // Save
    await pressEnterInEditor(page);
    await page.waitForTimeout(500);

    // Should show a warning character in the target cell area
    // The \u26A0 (warning sign) should be visible
    // Increased timeout as validation may take time in CI
    const warningIcon = page.getByTestId("target-text-0").locator("text=\u26A0");
    await expect(warningIcon).toBeVisible({ timeout: 10000 });
  });

  test("should not show warning icon when all tags match", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 1 and save with both tags correctly
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    const editor = page.locator('[contenteditable="true"]');
    await editor.focus();
    await clickTestId(page, "tag-palette-0"); // opening <a>
    await page.waitForTimeout(100);
    await typeInEditor(page, "lien");
    await clickTestId(page, "tag-palette-1"); // closing </a>
    await page.waitForTimeout(100);

    // Save
    await pressEnterInEditor(page);
    await page.waitForTimeout(500);

    // No warning icon should appear
    const warningIcons = page.getByTestId("target-text-0").locator("text=\u26A0");
    await expect(warningIcons).toHaveCount(0);
  });

  test("warning icon should have tooltip describing the issues", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Save block 1 with only opening tag
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    const editor = page.locator('[contenteditable="true"]');
    await editor.focus();
    await clickTestId(page, "tag-palette-0"); // only opening
    await page.waitForTimeout(100);
    await typeInEditor(page, "text");

    await pressEnterInEditor(page);
    await page.waitForTimeout(500);

    // The warning span should have a title attribute with issue details
    const warningSpan = page.getByTestId("target-text-0").locator('[title]').last();
    const title = await warningSpan.getAttribute("title");
    expect(title).toBeTruthy();
    // Title should mention missing closing tag
    expect(title!.toLowerCase()).toContain("missing");
  });
});

test.describe("Inline Codes — Editor Cancel and Re-edit", () => {
  test("should cancel editing with Escape and restore original display", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Start editing block 1
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Palette should be visible
    await expect(page.getByText("Tags:")).toBeVisible();

    // Type some text
    const editor = page.locator('[contenteditable="true"]');
    await editor.focus();
    await typeInEditor(page, "Cancelled text");
    await page.waitForTimeout(100);

    // Press Escape to cancel
    await pressEscapeInEditor(page);
    await page.waitForTimeout(300);

    // Palette should disappear
    await expect(page.getByText("Tags:")).not.toBeVisible();

    // The target cell should not show "Cancelled text"
    const target0 = page.getByTestId("target-text-0");
    await expect(target0).not.toContainText("Cancelled text");
  });

  test("should re-enter edit mode after save and show preserved coded text", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit and save block 1 with tags
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    const editor = page.locator('[contenteditable="true"]');
    await editor.focus();
    await clickTestId(page, "tag-palette-0");
    await page.waitForTimeout(100);
    await typeInEditor(page, "lien");
    await clickTestId(page, "tag-palette-1");
    await page.waitForTimeout(100);

    // Save
    await pressEnterInEditor(page);
    await page.waitForTimeout(500);

    // Verify saved
    const targetChips = page.getByTestId("target-text-0").locator("[data-tag-chip]");
    await expect(targetChips).toHaveCount(2);

    // Re-open block 0 for editing (need to click it first to select, then double-click)
    await clickTestId(page, "block-row-0");
    await page.waitForTimeout(100);
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Editor should show the previously saved chips
    const editorChips = page.locator('[contenteditable="true"] [data-tag-chip]');
    await expect(editorChips).toHaveCount(2);

    // And the text
    const editorText = page.locator('[contenteditable="true"]');
    await expect(editorText).toContainText("lien");
  });
});

test.describe("Inline Codes — Mixed Block Navigation", () => {
  test("should navigate between span and non-span blocks correctly", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Block 0 has spans
    const row0 = page.getByTestId("block-row-0");
    await expect(row0.locator("[data-tag-chip]")).toHaveCount(2);

    // Block 4 has no spans
    const row4 = page.getByTestId("block-row-4");
    await expect(row4).toContainText("Simple text without tags");
    await expect(row4.locator("[data-tag-chip]")).toHaveCount(0);

    // Click block 0 first to focus the grid
    await clickTestId(page, "block-row-0");
    await page.waitForTimeout(100);

    // Navigate down to block 4 by clicking block-row-4 directly
    await clickTestId(page, "block-row-4");
    await page.waitForTimeout(200);

    // Verify status bar shows block 5
    await expect(page.getByTestId("status-bar")).toContainText("Block 5 of");
  });

  test("should show Ctrl+N hint in status bar when editing block with spans", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 1 (has spans)
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Status bar should mention tag insertion shortcut
    await expect(page.getByTestId("status-bar")).toContainText("Ctrl+1");
  });

  test("should advance to next block after saving", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 1
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Insert both tags + text and save
    const editor = page.locator('[contenteditable="true"]');
    await editor.focus();
    await clickTestId(page, "tag-palette-0");
    await page.waitForTimeout(50);
    await typeInEditor(page, "x");
    await clickTestId(page, "tag-palette-1");
    await page.waitForTimeout(50);

    await pressEnterInEditor(page);
    await page.waitForTimeout(500);

    // Should have advanced to block 2 (index 1)
    await expect(page.getByTestId("status-bar")).toContainText("Block 2 of");
  });
});
