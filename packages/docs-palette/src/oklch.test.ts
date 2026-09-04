import { describe, expect, it } from "vitest";

import {
  accent,
  contrastRatio,
  forContrast,
  luminance,
  parseOklch,
  toGamut,
  toHex,
  toRgba,
  withLightness,
} from "./oklch.ts";

describe("parseOklch", () => {
  it("reads the plain three-number form", () => {
    expect(parseOklch("oklch(0.58 0.16 250)")).toEqual({ l: 0.58, c: 0.16, h: 250 });
  });

  it("reads percentages and a degree unit", () => {
    expect(parseOklch("oklch(50% 25% 250deg)")).toEqual({ l: 0.5, c: 0.1, h: 250 });
  });

  it("drops an alpha channel", () => {
    expect(parseOklch("oklch(0.62 0.14 250 / 0.16)")).toEqual({ l: 0.62, c: 0.14, h: 250 });
  });

  it("returns null for anything that is not an oklch colour", () => {
    for (const value of ["#2e8555", "var(--primary)", "0.5rem", "rgb(1 2 3)"]) {
      expect(parseOklch(value)).toBeNull();
    }
  });
});

describe("toHex", () => {
  it("renders the achromatic ends of the scale", () => {
    expect(toHex({ l: 1, c: 0, h: 0 })).toBe("#ffffff");
    expect(toHex({ l: 0, c: 0, h: 0 })).toBe("#000000");
  });

  it("renders the sRGB primaries from their own OKLCH coordinates", () => {
    expect(toHex({ l: 0.62796, c: 0.25768, h: 29.234 })).toBe("#ff0000");
    expect(toHex({ l: 0.86644, c: 0.29483, h: 142.495 })).toBe("#00ff00");
    expect(toHex({ l: 0.45201, c: 0.31321, h: 264.052 })).toBe("#0000ff");
  });

  it("is stable across repeated calls", () => {
    const color = { l: 0.58, c: 0.16, h: 250 };
    expect(toHex(color)).toBe(toHex(color));
  });
});

describe("toGamut", () => {
  it("leaves a printable colour alone", () => {
    const color = { l: 0.5, c: 0.1, h: 250 };
    expect(toGamut(color)).toEqual(color);
  });

  it("reduces chroma until the colour prints, keeping lightness and hue", () => {
    const mapped = toGamut({ l: 0.5, c: 0.4, h: 250 });
    expect(mapped.l).toBe(0.5);
    expect(mapped.h).toBe(250);
    expect(mapped.c).toBeLessThan(0.4);
    expect(mapped.c).toBeGreaterThan(0);
  });
});

describe("contrastRatio", () => {
  it("puts black on white at the top of the scale", () => {
    const ratio = contrastRatio({ l: 0, c: 0, h: 0 }, { l: 1, c: 0, h: 0 });
    expect(ratio).toBeCloseTo(21, 1);
  });

  it("gives a colour against itself the bottom of the scale", () => {
    expect(contrastRatio({ l: 0.5, c: 0.1, h: 250 }, { l: 0.5, c: 0.1, h: 250 })).toBe(1);
  });

  it("does not depend on the order of its arguments", () => {
    const a = { l: 0.3, c: 0.05, h: 20 };
    const b = { l: 0.9, c: 0.02, h: 200 };
    expect(contrastRatio(a, b)).toBe(contrastRatio(b, a));
  });
});

describe("forContrast", () => {
  const white = { l: 1, c: 0, h: 0 };
  const black = { l: 0.15, c: 0, h: 250 };

  it("darkens a colour that is too light for the page it sits on", () => {
    const brand = { l: 0.75, c: 0.16, h: 250 };
    const fixed = forContrast(brand, white, 4.5);
    expect(fixed.l).toBeLessThan(brand.l);
    expect(contrastRatio(fixed, white)).toBeGreaterThanOrEqual(4.5);
  });

  it("lightens a colour that is too dark for the page it sits on", () => {
    const brand = { l: 0.35, c: 0.16, h: 250 };
    const fixed = forContrast(brand, black, 4.5);
    expect(fixed.l).toBeGreaterThan(brand.l);
    expect(contrastRatio(fixed, black)).toBeGreaterThanOrEqual(4.5);
  });

  it("leaves a colour that already clears the bar exactly where it was", () => {
    const brand = { l: 0.4, c: 0.16, h: 250 };
    expect(forContrast(brand, white, 4.5)).toEqual(brand);
  });

  it("keeps the hue and chroma it was given", () => {
    const brand = { l: 0.75, c: 0.16, h: 250 };
    const fixed = forContrast(brand, white, 7);
    expect(fixed.c).toBe(brand.c);
    expect(fixed.h).toBe(brand.h);
  });
});

describe("withLightness and accent", () => {
  it("clamps lightness into a printable range", () => {
    expect(withLightness({ l: 0.5, c: 0.1, h: 20 }, 2).l).toBe(0.99);
    expect(withLightness({ l: 0.5, c: 0.1, h: 20 }, -1).l).toBe(0.02);
  });

  it("takes only the hue from the colour it is given", () => {
    expect(accent({ l: 0.945, c: 0.035, h: 300 }, 0.5, 0.15)).toEqual({ l: 0.5, c: 0.15, h: 300 });
  });
});

describe("toRgba and luminance", () => {
  it("prints an alpha to two decimals", () => {
    expect(toRgba({ l: 0, c: 0, h: 0 }, 0.08)).toBe("rgba(0, 0, 0, 0.08)");
  });

  it("reads luminance off the rounded channels the file carries", () => {
    expect(luminance({ l: 1, c: 0, h: 0 })).toBeCloseTo(1, 5);
    expect(luminance({ l: 0, c: 0, h: 0 })).toBe(0);
  });
});
