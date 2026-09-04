import { spawnSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

import { AA_TEXT, deriveBridge, textContrasts } from "./derive.ts";
import { check, loadCanonical, renderTarget } from "./generate.ts";
import { hexContrast, hexLuminance } from "./oklch.ts";
import { declaration, renderBridge } from "./render.ts";
import { MAKE_TARGET, TARGETS } from "./targets.ts";
import { readPalette } from "./tokens.ts";

const REPO_ROOT = fileURLToPath(new URL("../../..", import.meta.url));

/** The two canonical brand palettes, each with everything it imports. */
const BRANDS = [...new Set(TARGETS.map((t) => t.source))].map((source) => ({
  source,
  palette: loadCanonical(join(REPO_ROOT, source)),
}));

const RAMP_STEPS = [
  "ifm-color-primary-darkest",
  "ifm-color-primary-darker",
  "ifm-color-primary-dark",
  "ifm-color-primary",
  "ifm-color-primary-light",
  "ifm-color-primary-lighter",
  "ifm-color-primary-lightest",
];

function declared(css: string, name: string): string {
  const match = new RegExp(`--${name}:\\s*([^;]+);`).exec(css);
  if (match === null) throw new Error(`the bridge declares no --${name}`);
  return match[1];
}

describe.each(BRANDS)("$source", ({ palette }) => {
  const bridge = deriveBridge(palette);

  it.each(["light", "dark"] as const)("has a %s primary ramp that climbs in lightness", (theme) => {
    const values = new Map(bridge[theme].site.map((d) => [d.name, d.value]));
    const lightnesses = RAMP_STEPS.map((step) => {
      const hex = values.get(step);
      if (hex === undefined) throw new Error(`the ${theme} ramp has no ${step}`);
      return hexLuminance(hex);
    });
    for (let i = 1; i < lightnesses.length; i++) {
      expect(lightnesses[i]).toBeGreaterThan(lightnesses[i - 1]);
    }
  });

  it("clears WCAG AA on every text pair it is answerable for", () => {
    const failing = textContrasts(palette)
      .filter((pair) => pair.ratio < AA_TEXT)
      .map((pair) => `${pair.theme} ${pair.what} ${pair.ratio.toFixed(2)}:1`);
    expect(failing).toEqual([]);
  });

  it.each(["light", "dark"] as const)(
    "puts %s links and body text over AA against both the page and a card",
    (theme) => {
      const values = new Map(bridge[theme].site.map((d) => [d.name, d.value]));
      const page = values.get("ifm-background-color")!;
      const card = values.get("ifm-background-surface-color")!;
      for (const ink of ["ifm-color-primary", "ifm-font-color-base", "ifm-heading-color"]) {
        expect(hexContrast(values.get(ink)!, page)).toBeGreaterThanOrEqual(AA_TEXT);
        expect(hexContrast(values.get(ink)!, card)).toBeGreaterThanOrEqual(AA_TEXT);
      }
    },
  );

  it.each(["light", "dark"] as const)("gives every %s diagram role its own colour", (theme) => {
    const roles = ["annotate", "check", "io", "plugin", "resource", "translate"].map(
      (role) => bridge[theme].diagram.find((d) => d.name === `kdx-${role}`)!.value,
    );
    expect(new Set(roles).size).toBe(roles.length);
  });

  it.each(["light", "dark"] as const)(
    "keeps %s diagram accents legible on both grounds",
    (theme) => {
      const values = new Map(bridge[theme].diagram.map((d) => [d.name, d.value]));
      for (const [name, value] of values) {
        if (!name.startsWith("kdx-") || !value.startsWith("#")) continue;
        if (["kdx-surface", "kdx-surface-2", "kdx-border", "kdx-channel"].includes(name)) continue;
        expect(hexContrast(value, values.get("kdx-surface")!)).toBeGreaterThanOrEqual(AA_TEXT);
        expect(hexContrast(value, values.get("kdx-surface-2")!)).toBeGreaterThanOrEqual(AA_TEXT);
      }
    },
  );
});

describe("the committed bridges", () => {
  it.each(TARGETS)("$out matches its brand tokens", (target) => {
    const committed = readFileSync(join(REPO_ROOT, target.out), "utf8");
    expect(renderTarget(REPO_ROOT, target)).toBe(committed);
  });

  it("renders byte-identically on a second run", () => {
    for (const target of TARGETS) {
      expect(renderTarget(REPO_ROOT, target)).toBe(renderTarget(REPO_ROOT, target));
    }
  });

  it("reports every target as current, which is what the drift gate reads", () => {
    expect(check(REPO_ROOT).every((result) => result.current)).toBe(true);
  });

  it("carries no timestamp and no path from outside the repo", () => {
    for (const target of TARGETS) {
      const css = readFileSync(join(REPO_ROOT, target.out), "utf8");
      expect(css).not.toMatch(/\/(Users|home)\//);
      expect(css).not.toMatch(/\b(19|20)\d{2}-\d{2}-\d{2}\b/);
    }
  });

  it("names the make target that rewrites it", () => {
    for (const target of TARGETS) {
      expect(readFileSync(join(REPO_ROOT, target.out), "utf8")).toContain(`make ${MAKE_TARGET}`);
    }
  });

  it("keeps the kapi documentation tree clear of the platform's name", () => {
    const kapiSite = TARGETS.find((t) => t.out.startsWith("web/"))!;
    const css = readFileSync(join(REPO_ROOT, kapiSite.out), "utf8");
    expect(css.toLowerCase()).not.toContain("bowrain");
  });
});

describe("the generator entry point", () => {
  const ENTRY = "packages/docs-palette/cli/gen-docs-palette.ts";

  it("exists where the make target runs it from", () => {
    expect(existsSync(join(REPO_ROOT, ENTRY))).toBe(true);
    const makefile = readFileSync(join(REPO_ROOT, "Makefile"), "utf8");
    expect(makefile).toContain(`${ENTRY}\n`);
    expect(makefile).toContain(`${ENTRY} -check`);
  });

  it("sits outside every gitignored directory", () => {
    // `bin/` is ignored repo-wide, so a generator under one runs locally and is
    // missing from a fresh checkout: the drift gate fails on a file CI cannot
    // see, with nothing in the working tree to explain it.
    const ignored = spawnSync("git", ["check-ignore", "--quiet", ENTRY], { cwd: REPO_ROOT });
    expect(ignored.status).not.toBe(0);
  });
});

describe("declaration", () => {
  it("keeps a short declaration on one line", () => {
    expect(declaration("ifm-color-primary", "#0065b0")).toBe("  --ifm-color-primary: #0065b0;");
  });

  it("moves a long value onto its own indented lines, filled on the commas", () => {
    const stack =
      '"Inter Variable", "Inter", ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, sans-serif';
    expect(declaration("ifm-font-family-base", stack)).toBe(
      [
        "  --ifm-font-family-base:",
        '    "Inter Variable", "Inter", ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont,',
        "    sans-serif;",
      ].join("\n"),
    );
  });

  it("keeps every line it emits inside the formatter's width", () => {
    const stack = Array.from({ length: 12 }, (_, i) => `family-number-${i}`).join(", ");
    for (const line of declaration("ifm-font-family-base", stack).split("\n")) {
      expect(line.length).toBeLessThanOrEqual(100);
    }
  });
});

describe("renderBridge", () => {
  const LIGHT = `
    --background: oklch(0.98 0.003 240);
    --foreground: oklch(0.25 0.02 255);
    --card: oklch(1 0 0);
    --muted: oklch(0.95 0.008 240);
    --muted-foreground: oklch(0.52 0.02 250);
    --border: oklch(0.91 0.01 240);
    --primary: oklch(0.5 0.17 255);
    --success: oklch(0.53 0.14 150);
    --warning: oklch(0.75 0.16 70);
    --axis-product: oklch(0.945 0.035 15);
    --axis-brand: oklch(0.945 0.035 300);
    --axis-language: oklch(0.945 0.03 195);
  `;
  const DARK = `
    --background: oklch(0.2 0.015 255);
    --foreground: oklch(0.95 0.007 240);
    --card: oklch(0.23 0.015 255);
    --muted: oklch(0.27 0.018 255);
    --muted-foreground: oklch(0.68 0.015 245);
    --border: oklch(0.3 0.02 255);
    --primary: oklch(0.7 0.14 250);
    --success: oklch(0.64 0.15 148);
    --warning: oklch(0.78 0.14 70);
    --axis-product: oklch(0.3 0.045 15);
    --axis-brand: oklch(0.3 0.045 300);
    --axis-language: oklch(0.3 0.04 195);
  `;
  const palette = (light = LIGHT) => readPalette(`:root { ${light} }\n.dark { ${DARK} }`);
  const fixture = palette();
  const options = {
    source: "packages/ui/src/styles/example.css",
    target: MAKE_TARGET,
    summary: "An example.",
  } as const;

  it("writes a site bridge under the selectors Docusaurus sets", () => {
    const css = renderBridge(deriveBridge(fixture), { ...options, kind: "site" });
    expect(css).toContain(":root {");
    expect(css).toContain(':root[data-theme="dark"] {');
    expect(css).toContain(":root .kdx {");
    expect(css).toContain(':root[data-theme="dark"] .kdx {');
    expect(css).toContain("GENERATED FILE, DO NOT EDIT");
    expect(css.endsWith("}\n")).toBe(true);
  });

  it("writes a kit bridge under the class Storybook sets as well", () => {
    const css = renderBridge(deriveBridge(fixture), { ...options, kind: "diagram" });
    expect(css).toContain(".kdx {");
    expect(css).toContain('.dark .kdx,\n[data-theme="dark"] .kdx {');
    expect(css).not.toContain("--ifm-");
  });

  it("omits a font family the palette does not declare", () => {
    const css = renderBridge(deriveBridge(fixture), { ...options, kind: "site" });
    expect(css).not.toContain("--ifm-font-family-base");
  });

  it("emits the font families a palette does declare", () => {
    const withFonts = palette(
      `--brand-font-sans: "Inter", sans-serif;
       --brand-font-mono: "JetBrains Mono", monospace;
       ${LIGHT}`,
    );
    const css = renderBridge(deriveBridge(withFonts), { ...options, kind: "site" });
    expect(declared(css, "ifm-font-family-base")).toBe('"Inter", sans-serif');
    expect(declared(css, "ifm-font-family-monospace")).toBe('"JetBrains Mono", monospace');
    expect(declared(css, "kdx-mono")).toBe('"JetBrains Mono", monospace');
  });

  it("names the token the palette is missing rather than failing on a colour parse", () => {
    const short = palette(LIGHT.replace(/--primary:[^;]+;/, ""));
    expect(() => deriveBridge(short)).toThrow(/the light palette declares no --primary/);
  });

  it("fails loudly when a token the bridge needs is not a colour", () => {
    const wrong = palette(LIGHT.replace(/--background:[^;]+;/, "--background: var(--page);"));
    expect(() => deriveBridge(wrong)).toThrow(/not an oklch\(\) colour/);
  });
});
