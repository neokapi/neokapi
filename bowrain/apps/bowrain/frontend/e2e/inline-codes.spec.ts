import { test, expect, type Page } from "@playwright/test";
import { setupLocalApp } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

// Unicode markers matching the Go model
const M_OPEN = "\uE001";
const M_CLOSE = "\uE002";
const M_PLACEHOLDER = "\uE003";

/**
 * Injects the mock backend, creates a project with a file containing
 * blocks that have inline spans, and navigates to the editor.
 *
 * Span types use vocabulary identifiers so the UI renders with proper
 * formatting, labels, and colors (e.g. "fmt:bold" → Bold, "link:hyperlink" → Hyperlink).
 */
async function openEditorWithInlineBlocks(page: Page) {
  await setupLocalApp(page);

  // Create project
  await page.getByText("Upload files").click();
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
                { span_type: "opening", type: "link:hyperlink", id: "a1", data: '<a href="#">' },
                { span_type: "closing", type: "link:hyperlink", id: "a1", data: "</a>" },
              ],
              targets: { ...welcomeTargets["welcome.html-block-1"] },
              targets_coded: { ...welcomeTargetsCoded["welcome.html-block-1"] },
              translatable: true,
              has_spans: true,
              properties: {},
            },
            {
              id: "welcome.html-block-2",
              source: "Bold and italic text",
              source_coded: `${mo}Bold${mc} and ${mo}italic${mc} text`,
              source_spans: [
                { span_type: "opening", type: "fmt:bold", id: "b1", data: "<b>" },
                { span_type: "closing", type: "fmt:bold", id: "b1", data: "</b>" },
                { span_type: "opening", type: "fmt:italic", id: "i1", data: "<i>" },
                { span_type: "closing", type: "fmt:italic", id: "i1", data: "</i>" },
              ],
              targets: { ...welcomeTargets["welcome.html-block-2"] },
              targets_coded: { ...welcomeTargetsCoded["welcome.html-block-2"] },
              translatable: true,
              has_spans: true,
              properties: {},
            },
            {
              id: "welcome.html-block-3",
              source: "Line break here",
              source_coded: `Line break${mp} here`,
              source_spans: [
                { span_type: "placeholder", type: "struct:break", id: "br1", data: "<br/>" },
              ],
              targets: { ...welcomeTargets["welcome.html-block-3"] },
              targets_coded: { ...welcomeTargetsCoded["welcome.html-block-3"] },
              translatable: true,
              has_spans: true,
              properties: {},
            },
            {
              id: "welcome.html-block-4",
              source: "Code and link example",
              source_coded: `${mo}Code${mc} and ${mo}link${mc} example`,
              source_spans: [
                { span_type: "opening", type: "fmt:code", id: "code1", data: "<code>" },
                { span_type: "closing", type: "fmt:code", id: "code1", data: "</code>" },
                { span_type: "opening", type: "link:hyperlink", id: "a2", data: '<a href="/doc">' },
                { span_type: "closing", type: "link:hyperlink", id: "a2", data: "</a>" },
              ],
              targets: { ...welcomeTargets["welcome.html-block-4"] },
              targets_coded: { ...welcomeTargetsCoded["welcome.html-block-4"] },
              translatable: true,
              has_spans: true,
              properties: {},
            },
            {
              id: "welcome.html-block-5",
              source: "Simple text without tags",
              targets: { ...welcomeTargets["welcome.html-block-5"] },
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

      // Monkey-patch UpdateBlockTargetRuns for welcome.html blocks. The
      // @neokapi/ui editor authors coded text + spans; the desktop adapter
      // converts to an RFC 0001 Run sequence before calling the backend,
      // so the request carries `runs` (not coded_text). Reconstruct the
      // plain + coded forms from the runs so the source-block stub above
      // can echo edits back into the editor.
      const origUpdateRuns = mock[IDS.UpdateBlockTargetRuns];
      mock[IDS.UpdateBlockTargetRuns] = (req: any) => {
        const itemName = req.item_name || req.file_name;
        if (itemName === "welcome.html") {
          if (!welcomeTargets[req.block_id]) welcomeTargets[req.block_id] = {};
          if (!welcomeTargetsCoded[req.block_id]) welcomeTargetsCoded[req.block_id] = {};
          let plain = "";
          let coded = "";
          for (const run of req.runs ?? []) {
            if (run.text) {
              plain += run.text.text;
              coded += run.text.text;
            } else if (run.pcOpen) {
              coded += mo;
            } else if (run.pcClose) {
              coded += mc;
            } else if (run.ph) {
              coded += mp;
            }
          }
          welcomeTargets[req.block_id][req.target_locale] = plain;
          welcomeTargetsCoded[req.block_id][req.target_locale] = coded;
          return;
        }
        if (origUpdateRuns) return origUpdateRuns(req);
      };
    },
    { mo: M_OPEN, mc: M_CLOSE, mp: M_PLACEHOLDER },
  );

  // Navigate back to projects list and re-enter to refresh
  await page.getByTestId("back-to-projects").click();
  await page.waitForTimeout(200);

  await page.getByText("Inline Test").first().click();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });

  // Open the file
  await expect(page.getByTestId("open-file-welcome.html")).toBeVisible({ timeout: 5000 });
  await page.evaluate(() => {
    const btn = document.querySelector('[data-testid="open-file-welcome.html"]') as HTMLElement;
    if (btn) btn.click();
  });

  await expect(page.getByTestId("view-switcher")).toBeVisible({ timeout: 5000 });
  await page.evaluate(() => {
    (document.querySelector('[data-testid="view-table"]') as HTMLElement)?.click();
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
      el.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: "Enter",
          code: "Enter",
          keyCode: 13,
          bubbles: true,
          cancelable: true,
        }),
      );
    }
  });
}

/** Helper: press Escape in the Lexical editor via dispatchEvent. */
async function pressEscapeInEditor(page: Page) {
  await page.evaluate(() => {
    const el = document.querySelector('[contenteditable="true"]');
    if (el) {
      el.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: "Escape",
          code: "Escape",
          keyCode: 27,
          bubbles: true,
          cancelable: true,
        }),
      );
    }
  });
}

/** Helper: press Ctrl+N (digit) in the Lexical editor via dispatchEvent. */
async function pressCtrlDigitInEditor(page: Page, digit: number) {
  await page.evaluate((d: number) => {
    const el = document.querySelector('[contenteditable="true"]');
    if (el) {
      el.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: String(d),
          code: `Digit${d}`,
          ctrlKey: true,
          bubbles: true,
          cancelable: true,
        }),
      );
    }
  }, digit);
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("Inline Codes — Formatted Source Display", () => {
  test("should render formatted text for blocks with link spans", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Block 1 has <a>...</a> link tags — source text should be visible
    const row0 = page.getByTestId("block-row-0");
    await expect(row0).toBeVisible();
    await expect(row0).toContainText("Click");
    await expect(row0).toContainText("here");
    await expect(row0).toContainText("to learn more");

    // The "here" text should have underline formatting from the link vocabulary
    const hereStyle = await row0.evaluate(() => {
      // Find the span containing "here" text within the source cell
      const sourceCell = document.querySelector('[data-testid="block-row-0"] div.pr-4');
      if (!sourceCell) return null;
      const spans = sourceCell.querySelectorAll("span");
      for (const span of spans) {
        if (span.textContent === "here") {
          const cs = window.getComputedStyle(span);
          return { textDecoration: cs.textDecoration, color: cs.color };
        }
      }
      return null;
    });
    expect(hereStyle).not.toBeNull();
    expect(hereStyle!.textDecoration).toContain("underline");
  });

  test("should apply bold and italic formatting", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Block 2: <b>Bold</b> and <i>italic</i> text
    const row1 = page.getByTestId("block-row-1");
    await expect(row1).toContainText("Bold");
    await expect(row1).toContainText("italic");
    await expect(row1).toContainText("text");

    // Check CSS styles applied by FormattedSourceDisplay
    const styles = await row1.evaluate(() => {
      const sourceCell = document.querySelector('[data-testid="block-row-1"] div.pr-4');
      if (!sourceCell) return null;
      const spans = sourceCell.querySelectorAll("span");
      let boldWeight = "";
      let italicStyle = "";
      for (const span of spans) {
        const cs = window.getComputedStyle(span);
        if (span.textContent === "Bold") boldWeight = cs.fontWeight;
        if (span.textContent === "italic") italicStyle = cs.fontStyle;
      }
      return { boldWeight, italicStyle };
    });
    expect(styles).not.toBeNull();
    expect(styles!.boldWeight).toBe("700");
    expect(styles!.italicStyle).toBe("italic");
  });

  test("should render line break indicator for struct:break", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Block 3: Line break<br/> here — should show ↩ symbol
    const row2 = page.getByTestId("block-row-2");
    await expect(row2).toContainText("Line break");
    await expect(row2).toContainText("here");

    // The ↩ symbol should be present (rendered by FormattedSourceDisplay for struct:break)
    const hasReturnSymbol = await row2.evaluate(() => {
      const sourceCell = document.querySelector('[data-testid="block-row-2"] div.pr-4');
      return sourceCell?.textContent?.includes("\u23CE") ?? false;
    });
    expect(hasReturnSymbol).toBe(true);
  });

  test("should apply distinct formatting to code and link spans", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Block 4: <code>Code</code> and <a>link</a> example
    const row3 = page.getByTestId("block-row-3");
    await expect(row3).toContainText("Code");
    await expect(row3).toContainText("link");
    await expect(row3).toContainText("example");

    // Code should have monospace font, link should have underline
    const styles = await row3.evaluate(() => {
      const sourceCell = document.querySelector('[data-testid="block-row-3"] div.pr-4');
      if (!sourceCell) return null;
      const spans = sourceCell.querySelectorAll("span");
      let codeFontFamily = "";
      let linkTextDecoration = "";
      for (const span of spans) {
        const cs = window.getComputedStyle(span);
        if (span.textContent === "Code") codeFontFamily = cs.fontFamily;
        if (span.textContent === "link") linkTextDecoration = cs.textDecoration;
      }
      return { codeFontFamily, linkTextDecoration };
    });
    expect(styles).not.toBeNull();
    expect(styles!.codeFontFamily).toContain("monospace");
    expect(styles!.linkTextDecoration).toContain("underline");
  });

  test("should render plain text without special formatting", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Block 5 is plain text — no formatting
    const row4 = page.getByTestId("block-row-4");
    await expect(row4).toBeVisible();
    await expect(row4).toContainText("Simple text without tags");
  });

  test("should show tooltips on formatted spans", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Block 1: the "here" text span should have a tooltip from the vocabulary
    const tooltip = await page.evaluate(() => {
      const sourceCell = document.querySelector('[data-testid="block-row-0"] div.pr-4');
      if (!sourceCell) return null;
      const spans = sourceCell.querySelectorAll("span");
      for (const span of spans) {
        if (span.textContent === "here" && span.title) {
          return span.title;
        }
      }
      return null;
    });
    expect(tooltip).not.toBeNull();
    expect(tooltip).toContain("Hyperlink");
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

  test("should show vocabulary-based labels on palette tag chips", async ({ page }) => {
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

    // Palette chip labels should come from vocabulary: B>, /B, I>, /I
    const chip0 = page.getByTestId("tag-palette-0").locator("[data-tag-chip]");
    const chip1 = page.getByTestId("tag-palette-1").locator("[data-tag-chip]");
    const chip2 = page.getByTestId("tag-palette-2").locator("[data-tag-chip]");
    const chip3 = page.getByTestId("tag-palette-3").locator("[data-tag-chip]");
    await expect(chip0).toContainText("B>");
    await expect(chip1).toContainText("/B");
    await expect(chip2).toContainText("I>");
    await expect(chip3).toContainText("/I");
  });

  test("should not show tag palette for plain text blocks", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 5 (plain text, no spans → uses textarea, not Lexical)
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

    // Click the first palette button to insert the opening link tag
    await clickTestId(page, "tag-palette-0");
    await page.waitForTimeout(200);

    // The editor content area should now contain a tag chip
    const editorChips = page.locator('[contenteditable="true"] [data-tag-chip]');
    const count = await editorChips.count();
    expect(count).toBeGreaterThan(0);
  });

  test("should insert tag via keyboard shortcut Ctrl+1", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 3 (single placeholder struct:break)
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-2"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Focus the editor
    await page.locator('[contenteditable="true"]').focus();
    await page.waitForTimeout(100);

    // Press Ctrl+1 to insert the first tag (struct:break placeholder)
    await pressCtrlDigitInEditor(page, 1);
    await page.waitForTimeout(200);

    // Should have at least one chip in the editor
    const editorChips = page.locator('[contenteditable="true"] [data-tag-chip]');
    const count = await editorChips.count();
    expect(count).toBeGreaterThan(0);
  });

  test("should show category separators for mixed-category blocks", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 4 (fmt:code = formatting + link:hyperlink = linking → 2 categories)
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-3"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Both format and link category separators should be visible
    await expect(page.getByText("Format")).toBeVisible();
    await expect(page.getByText("Links")).toBeVisible();
  });
});

test.describe("Inline Codes — Validation Bar", () => {
  test("should show validation errors when tags are missing from target", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 1 (has link:hyperlink opening + closing)
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // The editor starts with empty target, so all source tags are missing.
    // Validation should show errors with vocabulary labels.
    await expect(page.getByText(/Missing.*opening.*"Hyperlink"/i)).toBeVisible({ timeout: 3000 });
    await expect(page.getByText(/Missing.*closing.*"Hyperlink"/i)).toBeVisible({ timeout: 3000 });
  });

  test("should clear validation errors when all tags are inserted", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 3 (single struct:break placeholder — non-deletable)
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-2"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Initially shows missing tag error (struct:break is non-deletable)
    await expect(page.getByText(/Missing.*"Line Break"/i)).toBeVisible({ timeout: 3000 });

    // Insert the br tag via palette
    await clickTestId(page, "tag-palette-0");
    await page.waitForTimeout(300);

    // Validation error should disappear
    await expect(page.getByText(/Missing.*"Line Break"/i)).not.toBeVisible({ timeout: 3000 });
  });

  test("should show validation for multiple missing tags", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 2 (has fmt:bold + fmt:italic — 4 tags total)
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-1"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Should show errors about missing bold and italic tags with vocabulary labels
    await expect(page.getByText(/Missing.*"Bold"/i).first()).toBeVisible({ timeout: 3000 });
    await expect(page.getByText(/Missing.*"Italic"/i).first()).toBeVisible({ timeout: 3000 });
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

    // Edit block 3 (struct:break placeholder — starts empty)
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-2"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Type some text
    const editor = page.locator('[contenteditable="true"]');
    await editor.focus();
    await typeInEditor(page, "Hello preview");
    await page.waitForTimeout(300);

    // Preview should show the typed text
    await expect(page.getByText("Preview:")).toBeVisible();
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

    // Edit block 1 (has opening + closing link:hyperlink)
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

test.describe("Inline Codes — Target Cell Formatted Display", () => {
  test("should display formatted text in target cell after saving coded text", async ({ page }) => {
    await openEditorWithInlineBlocks(page);

    // Edit block 1 and insert tags + text
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(300);

    // Type text, then insert tags
    const editor = page.locator('[contenteditable="true"]');
    await editor.focus();
    await typeInEditor(page, "Cliquez ");
    await clickTestId(page, "tag-palette-0"); // insert opening link
    await page.waitForTimeout(100);
    await typeInEditor(page, "ici");
    await clickTestId(page, "tag-palette-1"); // insert closing link
    await page.waitForTimeout(100);
    await typeInEditor(page, " pour en savoir plus");
    await page.waitForTimeout(200);

    // Save by pressing Enter
    await pressEnterInEditor(page);
    await page.waitForTimeout(500);

    // After saving, the target cell should display formatted text (not tag chips)
    const targetCell = page.getByTestId("target-text-0");
    await expect(targetCell).toBeVisible();

    // Should contain the translated text
    await expect(targetCell).toContainText("Cliquez");
    await expect(targetCell).toContainText("ici");
    await expect(targetCell).toContainText("pour en savoir plus");

    // "ici" should have link formatting (underline) applied by FormattedSourceDisplay
    const iciStyle = await targetCell.evaluate((cell) => {
      const spans = cell.querySelectorAll("span");
      for (const span of spans) {
        if (span.textContent === "ici") {
          return window.getComputedStyle(span).textDecoration;
        }
      }
      return null;
    });
    expect(iciStyle).toContain("underline");
  });
});

test.describe("Inline Codes — Row Validation Warning", () => {
  test("should show warning icon when target has mismatched tags", async ({ page }) => {
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

    // Should show a warning icon (AlertTriangle SVG) in the target cell
    const warningIcon = page.getByTestId("target-text-0").locator('[data-testid="tag-warning"]');
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
    await clickTestId(page, "tag-palette-0"); // opening link
    await page.waitForTimeout(100);
    await typeInEditor(page, "lien");
    await clickTestId(page, "tag-palette-1"); // closing link
    await page.waitForTimeout(100);

    // Save
    await pressEnterInEditor(page);
    await page.waitForTimeout(500);

    // No warning icon should appear
    const warningIcons = page.getByTestId("target-text-0").locator('[data-testid="tag-warning"]');
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

    // The warning should have a title attribute with vocabulary-based label
    const warningEl = page.getByTestId("target-text-0").locator('[data-testid="tag-warning"]');
    await expect(warningEl).toBeVisible({ timeout: 10000 });
    const title = await warningEl.getAttribute("title");
    expect(title).toBeTruthy();
    // Title should mention missing closing Hyperlink tag
    expect(title!).toContain("Missing");
    expect(title!).toContain("Hyperlink");
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

    // Verify saved: target cell should show formatted text with "lien"
    await expect(page.getByTestId("target-text-0")).toContainText("lien");

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

    // Block 0 has formatted source text (link spans)
    const row0 = page.getByTestId("block-row-0");
    await expect(row0).toContainText("Click");
    await expect(row0).toContainText("here");

    // Block 4 has plain text (no spans)
    const row4 = page.getByTestId("block-row-4");
    await expect(row4).toContainText("Simple text without tags");

    // Click block 0 first to focus the grid
    await clickTestId(page, "block-row-0");
    await page.waitForTimeout(100);

    // Navigate to block 4 by clicking directly
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
