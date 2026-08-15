import { describe, expect, it } from "vitest";

import { isOnScreen } from "./useSectionSignals";

// The section_viewed funnel is the page's buyer test, and it is read per beat.
// The arrival rule therefore has to hold for a section taller than the screen,
// which is what the narrative beats become on a phone.

/** Section heights measured against the live page, in CSS pixels. */
const DESKTOP = { viewport: 900, hero: 754, rename: 702, how: 1076, loop: 849, proof: 1178 };
const PHONE = { viewport: 844, hero: 890, rename: 1038, how: 1907, loop: 1857, proof: 2010 };

describe("isOnScreen", () => {
  it("counts a section shorter than the viewport at half its own height", () => {
    expect(isOnScreen(376, DESKTOP.hero, DESKTOP.viewport)).toBe(false);
    expect(isOnScreen(378, DESKTOP.hero, DESKTOP.viewport)).toBe(true);
  });

  it("counts a section taller than the viewport at half the viewport", () => {
    // Half of `how` is 954px, which never fits an 844px phone screen; half the
    // viewport does, and is what a reader looking at the section actually sees.
    expect(isOnScreen(421, PHONE.how, PHONE.viewport)).toBe(false);
    expect(isOnScreen(422, PHONE.how, PHONE.viewport)).toBe(true);
  });

  it("is reachable for every narrative beat at both measured viewports", () => {
    for (const geometry of [DESKTOP, PHONE]) {
      const { viewport, ...sections } = geometry;
      for (const [name, height] of Object.entries(sections)) {
        // The most of a section that can ever be on screen at once.
        const maxVisible = Math.min(height, viewport);
        expect(isOnScreen(maxVisible, height, viewport), name).toBe(true);
      }
    }
  });

  it("does not count a section that is off screen", () => {
    expect(isOnScreen(0, PHONE.proof, PHONE.viewport)).toBe(false);
  });
});
