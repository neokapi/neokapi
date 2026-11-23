import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

/** Every source file under src/, tests left out. */
function sourceFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return name === "__tests__" ? [] : sourceFiles(path);
    return /\.tsx?$/.test(path) ? [path] : [];
  });
}

/** The t( calls in a source whose arguments hold another t( call. */
function nestedCalls(source: string): string[] {
  const call = /(?<![\w.])t\(/g;
  const out: string[] = [];
  for (let m = call.exec(source); m; m = call.exec(source)) {
    let depth = 1;
    let i = m.index + m[0].length;
    while (i < source.length && depth > 0) {
      if (source[i] === "(") depth++;
      else if (source[i] === ")") depth--;
      i++;
    }
    const inner = source.slice(m.index + m[0].length, i - 1);
    if (/(?<![\w.])t\(/.test(inner)) out.push(source.slice(m.index, Math.min(i, m.index + 80)));
  }
  return out;
}

describe("t() calls", () => {
  // The i18n transform reads each call's own key: a call nested in another's
  // arguments is never extracted, and its string ships untranslated.
  it("never nest", () => {
    const src = join(__dirname, "..");
    const found = sourceFiles(src).flatMap((path) =>
      nestedCalls(readFileSync(path, "utf8")).map((c) => `${relative(src, path)}: ${c}`),
    );
    expect(found).toEqual([]);
  });

  it("are found when they nest", () => {
    expect(nestedCalls('t("a", { b: t("c") })')).toHaveLength(1);
    expect(nestedCalls('const c = t("c"); t("a", { b: c })')).toEqual([]);
  });
});
