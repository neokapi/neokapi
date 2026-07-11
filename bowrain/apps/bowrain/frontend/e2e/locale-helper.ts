import { type Page, type Locator, expect } from "@playwright/test";

/**
 * Helpers for the Combobox-based LocaleSelect / MultiLocaleSelect components
 * (bowrain/packages/ui/src/components/LocaleSelect.tsx). Both render a
 * searchable combobox input inside a `data-testid={testId}` container; the
 * option list opens in a portal while typing. MultiLocaleSelect additionally
 * renders selected chips with `${testId}-remove-${code}` remove buttons and
 * gives its input/options `${testId}-search` / `${testId}-option-${code}`
 * test ids.
 */

function comboboxInput(page: Page, testId: string): Locator {
  return page.getByTestId(testId).getByRole("combobox");
}

/**
 * Select a single locale from a LocaleSelect component by typing into its
 * combobox and picking the matching "Name (code)" option.
 */
export async function selectLocale(page: Page, testId: string, code: string) {
  const input = comboboxInput(page, testId);
  await input.click();
  await input.fill(code);
  await page
    .getByRole("option", { name: `(${code})` })
    .first()
    .click();
}

/**
 * Add locales to a MultiLocaleSelect component. Each add remounts the picker
 * (search resets, popup closes), so open + type + pick per code.
 */
export async function selectMultiLocales(page: Page, testId: string, codes: string[]) {
  for (const code of codes) {
    // Already selected (e.g. the form's default target) — the picker excludes
    // selected locales from its options, so adding again would find nothing.
    if (await page.getByTestId(`${testId}-remove-${code}`).isVisible()) continue;
    const search = page.getByTestId(`${testId}-search`);
    await search.click();
    await search.fill(code);
    await page.getByTestId(`${testId}-option-${code}`).click();
    // The chip appearing confirms the selection landed before the next add.
    await expect(page.getByTestId(`${testId}-remove-${code}`)).toBeVisible();
  }
}

/**
 * Select multiple locales with human-like typing for recordings.
 */
export async function selectMultiLocalesHuman(
  page: Page,
  testId: string,
  codes: string[],
  humanTypeFn: (page: Page, locator: Locator, text: string) => Promise<void>,
) {
  for (const code of codes) {
    const search = page.getByTestId(`${testId}-search`);
    await search.click();
    await humanTypeFn(page, search, code);
    await page.getByTestId(`${testId}-option-${code}`).click();
    await expect(page.getByTestId(`${testId}-remove-${code}`)).toBeVisible();
  }
}

/**
 * Select a single locale with human-like typing for recordings.
 */
export async function selectLocaleHuman(
  page: Page,
  testId: string,
  code: string,
  humanTypeFn: (page: Page, locator: Locator, text: string) => Promise<void>,
) {
  const input = comboboxInput(page, testId);
  await input.click();
  await humanTypeFn(page, input, code);
  await page
    .getByRole("option", { name: `(${code})` })
    .first()
    .click();
}

/**
 * Remove all existing locale chips from a MultiLocaleSelect component.
 */
export async function clearMultiLocales(page: Page, testId: string) {
  const chips = page.locator(`[data-testid^="${testId}-remove-"]`);
  while ((await chips.count()) > 0) {
    await chips.first().click();
  }
}

/**
 * Set exact locales on a MultiLocaleSelect: clears existing chips, then adds the desired ones.
 */
export async function setMultiLocales(page: Page, testId: string, codes: string[]) {
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
  humanTypeFn: (page: Page, locator: Locator, text: string) => Promise<void>,
) {
  await clearMultiLocales(page, testId);
  await selectMultiLocalesHuman(page, testId, codes, humanTypeFn);
}

/**
 * Assert that exactly the given locale chips are visible.
 */
export async function expectLocaleChips(page: Page, testId: string, codes: string[]) {
  for (const code of codes) {
    await expect(page.getByTestId(`${testId}-remove-${code}`)).toBeVisible();
  }
  // Ensure no extra chips exist
  const count = await page.locator(`[data-testid^="${testId}-remove-"]`).count();
  expect(count).toBe(codes.length);
}

/**
 * Assert that a single-locale combobox displays the expected locale code.
 */
export async function expectSourceLocale(page: Page, testId: string, code: string) {
  await expect(comboboxInput(page, testId)).toHaveValue(new RegExp(code, "i"));
}
