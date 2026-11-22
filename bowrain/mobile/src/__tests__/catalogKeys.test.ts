import { readFileSync } from "node:fs";
import { join } from "node:path";

/**
 * Keys that appear twice in one object of the catalog. JSON.parse keeps the
 * last, so a second "context" block once replaced the context deck's strings
 * and every one of them rendered as its key.
 */
function duplicateKeys(text: string): string[] {
  const dups: string[] = [];
  const stack: Set<string>[] = [];
  const path: string[] = [];
  let last = "";
  const re = /"((?:[^"\\]|\\.)*)"\s*(:)?|([{}[\]])/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text))) {
    if (m[3] === "{") {
      stack.push(new Set());
      path.push(last);
    } else if (m[3] === "}") {
      stack.pop();
      path.pop();
    } else if (m[2] && stack.length > 0) {
      const keys = stack[stack.length - 1];
      if (keys.has(m[1])) dups.push([...path.slice(1), m[1]].join("."));
      keys.add(m[1]);
      last = m[1];
    }
  }
  return dups;
}

describe("the English catalog", () => {
  it("names no key twice in one object", () => {
    const text = readFileSync(join(__dirname, "..", "..", "i18n", "en.json"), "utf8");
    expect(duplicateKeys(text)).toEqual([]);
  });

  it("catches a key named twice", () => {
    expect(duplicateKeys('{"a": {"x": "1"}, "a": {"y": "2"}}')).toEqual(["a"]);
  });
});
