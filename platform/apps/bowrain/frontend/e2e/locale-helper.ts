import { type Page, expect } from "@playwright/test";

/**
 * Select a single locale from a LocaleSelect component.
 * The component has a trigger button that opens a dropdown with searchable options.
 * Clicking an option automatically closes the dropdown.
 */
export async function selectLocale(page: Page, testId: string, code: string) {
  await page.evaluate(
    ({ testId }: { testId: string }) => {
      const trigger = document.querySelector(
        `[data-testid="${testId}-trigger"]`,
      ) as HTMLElement;
      if (trigger) trigger.click();
    },
    { testId },
  );
  await page.waitForTimeout(50);
  await page.evaluate(
    ({ testId, code }: { testId: string; code: string }) => {
      const option = document.querySelector(
        `[data-testid="${testId}-option-${code}"]`,
      ) as HTMLElement;
      if (option) option.click();
    },
    { testId, code },
  );
  await page.waitForTimeout(50);
}

/**
 * Select multiple locales from a MultiLocaleSelect component.
 * The dropdown stays open between selections (by design), so we
 * dismiss it after all selections by clicking outside.
 */
export async function selectMultiLocales(
  page: Page,
  testId: string,
  codes: string[],
) {
  // Open the dropdown
  await page.evaluate(
    ({ testId }: { testId: string }) => {
      const chips = document.querySelector(
        `[data-testid="${testId}-chips"]`,
      ) as HTMLElement;
      if (chips) chips.click();
    },
    { testId },
  );
  await page.waitForTimeout(50);

  // Click each option in sequence
  for (const code of codes) {
    await page.evaluate(
      ({ testId, code }: { testId: string; code: string }) => {
        const option = document.querySelector(
          `[data-testid="${testId}-option-${code}"]`,
        ) as HTMLElement;
        if (option) option.click();
      },
      { testId, code },
    );
    await page.waitForTimeout(50);
  }

  // Close the dropdown by dispatching a mousedown outside the wrapper
  await page.evaluate(
    ({ testId: _testId }: { testId: string }) => {
      // Dispatch mousedown on body, outside the wrapper, to trigger click-outside close
      document.body.dispatchEvent(
        new MouseEvent("mousedown", { bubbles: true }),
      );
    },
    { testId },
  );
  await page.waitForTimeout(50);
}

/**
 * Select multiple locales with human-like typing for recordings.
 */
export async function selectMultiLocalesHuman(
  page: Page,
  testId: string,
  codes: string[],
  humanTypeFn: (page: Page, locator: any, text: string) => Promise<void>,
) {
  for (const code of codes) {
    // Open dropdown if not already open
    const search = page.getByTestId(`${testId}-search`);
    if (!(await search.isVisible().catch(() => false))) {
      const chips = page.getByTestId(`${testId}-chips`);
      await chips.click();
      await page.waitForTimeout(200);
    }

    // Type to search
    await humanTypeFn(page, search, code);
    await page.waitForTimeout(200);

    // Click option
    await page.evaluate(
      ({ testId, code }: { testId: string; code: string }) => {
        const option = document.querySelector(
          `[data-testid="${testId}-option-${code}"]`,
        ) as HTMLElement;
        if (option) option.click();
      },
      { testId, code },
    );
    await page.waitForTimeout(200);
  }

  // Close dropdown
  await page.evaluate(() => {
    document.body.dispatchEvent(
      new MouseEvent("mousedown", { bubbles: true }),
    );
  });
  await page.waitForTimeout(100);
}

/**
 * Select a single locale with human-like typing for recordings.
 */
export async function selectLocaleHuman(
  page: Page,
  testId: string,
  code: string,
  humanTypeFn: (page: Page, locator: any, text: string) => Promise<void>,
) {
  await page.getByTestId(`${testId}-trigger`).click();
  await page.waitForTimeout(200);

  const search = page.getByTestId(`${testId}-search`);
  await humanTypeFn(page, search, code);
  await page.waitForTimeout(200);

  await page.evaluate(
    ({ testId, code }: { testId: string; code: string }) => {
      const option = document.querySelector(
        `[data-testid="${testId}-option-${code}"]`,
      ) as HTMLElement;
      if (option) option.click();
    },
    { testId, code },
  );
  await page.waitForTimeout(200);
}

/**
 * Remove all existing locale chips from a MultiLocaleSelect component.
 */
export async function clearMultiLocales(page: Page, testId: string) {
  const removed = await page.evaluate(
    ({ testId }: { testId: string }) => {
      const buttons = document.querySelectorAll(
        `[data-testid^="${testId}-remove-"]`,
      );
      // Click in reverse order so indices stay stable
      const arr = Array.from(buttons) as HTMLElement[];
      for (let i = arr.length - 1; i >= 0; i--) {
        arr[i].click();
      }
      return arr.length;
    },
    { testId },
  );
  if (removed > 0) {
    await page.waitForTimeout(50);
  }
}

/**
 * Set exact locales on a MultiLocaleSelect: clears existing chips, then adds the desired ones.
 */
export async function setMultiLocales(
  page: Page,
  testId: string,
  codes: string[],
) {
  await clearMultiLocales(page, testId);
  await selectMultiLocales(page, testId, codes);
}

/**
 * Set exact locales with human-like typing for recordings.
 * Clears existing chips visually, then adds the desired ones.
 */
export async function setMultiLocalesHuman(
  page: Page,
  testId: string,
  codes: string[],
  humanTypeFn: (page: Page, locator: any, text: string) => Promise<void>,
) {
  await clearMultiLocales(page, testId);
  await selectMultiLocalesHuman(page, testId, codes, humanTypeFn);
}

/**
 * Assert that exactly the given locale chips are visible.
 */
export async function expectLocaleChips(
  page: Page,
  testId: string,
  codes: string[],
) {
  for (const code of codes) {
    await expect(
      page.getByTestId(`${testId}-remove-${code}`),
    ).toBeVisible();
  }
  // Ensure no extra chips exist
  const count = await page.locator(`[data-testid^="${testId}-remove-"]`).count();
  expect(count).toBe(codes.length);
}

/**
 * Assert that a single-locale trigger displays the expected locale.
 */
export async function expectSourceLocale(
  page: Page,
  testId: string,
  code: string,
) {
  await expect(
    page.getByTestId(`${testId}-trigger`),
  ).toContainText(code, { ignoreCase: true });
}
