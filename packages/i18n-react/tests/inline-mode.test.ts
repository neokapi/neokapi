/**
 * Inline-mode semantics: translated JSX reconstruction, ICU routing,
 * t() build-time resolution, and parse validity of every emitted
 * output. These cover the two historical critical bugs: paired
 * inline elements leaking `{/=mN}` tokens, and <Plural>/<Select>
 * producing invalid JSX in inline builds.
 */

import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { parseSync } from "@swc/core";
import { afterEach, describe, expect, it, vi } from "vitest";

import { transform } from "../src/plugin/transform.ts";
import { hashKey } from "../src/plugin/hash.ts";
import type { PluginOptions } from "../src/types.ts";

const tmpDirs: string[] = [];

afterEach(() => {
  for (const dir of tmpDirs.splice(0)) rmSync(dir, { recursive: true, force: true });
  vi.restoreAllMocks();
});

/** Write a {hash: translation} dict for `locale` into a fresh tmp dir. */
function dictDir(locale: string, entries: Record<string, string>): string {
  const dir = mkdtempSync(join(tmpdir(), "kapi-inline-"));
  tmpDirs.push(dir);
  writeFileSync(join(dir, `${locale}.json`), JSON.stringify(entries));
  return dir;
}

function inline(
  code: string,
  translationsDir: string | null,
  options: Partial<PluginOptions> = {},
): string | null {
  return (
    transform(code, "Test.tsx", {
      locale: "de",
      translationsDir: translationsDir ?? "./nonexistent",
      strict: false,
      ...options,
    })?.code ?? null
  );
}

/** Assert the transformed output is still parseable TSX. */
function expectParses(code: string): void {
  expect(() => parseSync(code, { syntax: "typescript", tsx: true })).not.toThrow();
}

describe("inline mode — paired inline elements", () => {
  const src = '<p>Click <a href="/x">here</a> to continue.</p>';
  const hash = hashKey("Click {=m0}here{/=m0} to continue.", "p");

  it("reconstructs the anchor around the translated inner text", () => {
    const dir = dictDir("de", { [hash]: "Klicken Sie {=m0}hier{/=m0}, um fortzufahren." });
    const out = inline(src, dir);
    expect(out).not.toBeNull();
    expect(out).toContain('<a href="/x">hier</a>');
    expect(out).not.toContain("{/=m0}");
    expect(out).not.toContain("{=m0}");
    expectParses(out as string);
  });

  it("identity build (missing translation) reproduces the source structure", () => {
    const out = inline(src, null);
    // Reconstruction from the source template must keep the inner
    // text inside the anchor and leak no tokens.
    expect(out).toContain('<a href="/x">here</a>');
    expect(out).toContain("Click ");
    expect(out).toContain(" to continue.");
    expect(out).not.toContain("{/=m0}");
    expect(out).not.toContain("{=m0}");
    expectParses(out as string);
  });

  it("identity build with a partial dict keeps other blocks intact", () => {
    const otherHash = hashKey("Save", "button");
    const dir = dictDir("de", { [otherHash]: "Speichern" });
    const out = inline(`<div>${src}<button>Save</button></div>`, dir);
    expect(out).not.toBeNull();
    expect(out).toContain('<a href="/x">here</a>');
    expect(out).not.toContain("Klicken");
    expect(out).toContain(">Speichern<");
    expect(out).not.toContain("{/=m0}");
    expectParses(out as string);
  });

  it("handles nested paired elements with reordering", () => {
    const nested = "<p>A <strong>B <em>C</em></strong> D</p>";
    const h = hashKey("A {=m0}B {=m1}C{/=m1}{/=m0} D", "p");
    const dir = dictDir("de", { [h]: "D2 {=m0}{=m1}C2{/=m1} B2{/=m0} A2" });
    const out = inline(nested, dir);
    expect(out).toContain("<strong><em>C2</em> B2</strong>");
    expect(out).toContain("D2 ");
    expect(out).not.toContain("{=m");
    expectParses(out as string);
  });

  it("substitutes standalone element markers wherever the translator puts them", () => {
    const withIcon = "<p>Press <Icon/> now</p>";
    const h = hashKey("Press {=m0} now", "p");
    const dir = dictDir("de", { [h]: "Jetzt {=m0} drücken" });
    const out = inline(withIcon, dir);
    expect(out).toContain("Jetzt <Icon/> drücken");
    expectParses(out as string);
  });

  it("escapes unknown translator tokens instead of leaking raw braces", () => {
    const h = hashKey("Hello", "p");
    const dir = dictDir("de", { [h]: "Hallo {bogus}" });
    const out = inline("<p>Hello</p>", dir);
    expect(out).toContain("Hallo &#123;bogus&#125;");
    expect(out).not.toContain("{bogus}");
    expectParses(out as string);
  });

  it("escapes angle brackets in translated text", () => {
    const h = hashKey("Hello", "p");
    const dir = dictDir("de", { [h]: "a < b > c" });
    const out = inline("<p>Hello</p>", dir);
    expect(out).toContain("a &lt; b &gt; c");
    expectParses(out as string);
  });

  it("substitutes named params as expression containers", () => {
    const withVar = "<p>Hello, {user.name}!</p>";
    const h = hashKey("Hello, {user.name}!", "p");
    const dir = dictDir("de", { [h]: "Hallo, {user.name}!" });
    const out = inline(withVar, dir);
    expect(out).toContain("Hallo, {user.name}!");
    expectParses(out as string);
  });
});

describe("inline mode — ICU routes through the runtime", () => {
  it("<Plural> emits a runtime call with the translated template baked in — never raw ICU in JSX", () => {
    const src = `import { Plural, One, Other } from "@neokapi/i18n-react/runtime";
export function Cart({ n }: { n: number }) {
  return <p><Plural count={n}><One>1 item</One><Other>{n} items</Other></Plural></p>;
}`;
    const template = "{n, plural, one {1 item} other {{n} items}}";
    const h = hashKey(template, "p");
    const de = "{n, plural, one {1 Artikel} other {{n} Artikel}}";
    const dir = dictDir("de", { [h]: de });
    const out = inline(src, dir);
    expect(out).not.toBeNull();
    expect(out).toContain("__t(");
    expect(out).toContain(JSON.stringify(de));
    // The ICU template must not appear as bare JSX children.
    expect(out).not.toMatch(/>\s*\{n, plural/);
    expectParses(out as string);
  });

  it("translator-driven ICU (no <Plural> in source) also routes through the runtime", () => {
    const src = "<p>{count} messages</p>";
    const h = hashKey("{count} messages", "p");
    const de = "{count, plural, one {{count} Nachricht} other {{count} Nachrichten}}";
    const dir = dictDir("de", { [h]: de });
    const out = inline(src, dir);
    expect(out).toContain("__t(");
    expect(out).toContain(JSON.stringify(de));
    expectParses(out as string);
  });
});

describe("inline mode — t() build-time resolution", () => {
  const IMPORT = 'import { t } from "@neokapi/i18n-react/runtime";\n';

  it("collapses a literal t() call to a plain string literal", () => {
    const h = hashKey("Light", `t\x1F`);
    const dir = dictDir("de", { [h]: "Hell" });
    const out = inline(`${IMPORT}const label = t("Light");`, dir);
    expect(out).toContain('const label = "Hell"');
    expect(out).not.toContain("__t(");
    expectParses(out as string);
  });

  it("keeps a __t call with baked translation when params are present", () => {
    const h = hashKey("Hello, {name}!", `t\x1F`);
    const dir = dictDir("de", { [h]: "Hallo, {name}!" });
    const out = inline(`${IMPORT}const g = t("Hello, {name}!", { name });`, dir);
    expect(out).toContain('__t("' + h + '", "Hallo, {name}!", { name })');
    expectParses(out as string);
  });

  it("falls back to source text and warns when the translation is missing", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const dir = dictDir("de", {});
    const out = inline(`${IMPORT}const label = t("Light");`, dir, { strict: "warn" });
    expect(out).toContain('const label = "Light"');
    expect(warn).toHaveBeenCalledWith(expect.stringContaining("Missing translation"));
  });
});

describe("inline mode — attributes", () => {
  it("escapes double quotes in translated attribute values as entities", () => {
    const h = hashKey("Search...", "input[placeholder]");
    const dir = dictDir("de", { [h]: 'Suche "hier"...' });
    const out = inline('<input placeholder="Search..." />', dir);
    expect(out).toContain('placeholder="Suche &quot;hier&quot;..."');
    expectParses(out as string);
  });
});
