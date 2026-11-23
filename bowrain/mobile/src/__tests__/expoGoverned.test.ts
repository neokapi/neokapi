import { readFileSync } from "node:fs";
import { join } from "node:path";

/**
 * Dependabot leaves the packages the Expo SDK pins to an SDK update
 * (`.github/dependabot.yml`, the mobile entry's ignore list). The list must be
 * the SDK's set as this app uses it: the dependencies that the installed
 * Expo's bundledNativeModules.json names. A package the app adds from that
 * set, or one an SDK update adds to it, fails this test until the list names
 * it, so Dependabot never moves one of them on its own.
 */

const root = join(__dirname, "..", "..", "..", "..");

function governed(): string[] {
  const bundled = JSON.parse(
    readFileSync(require.resolve("expo/bundledNativeModules.json"), "utf8"),
  ) as Record<string, string>;
  const pkg = JSON.parse(readFileSync(join(__dirname, "..", "..", "package.json"), "utf8")) as {
    dependencies?: Record<string, string>;
    devDependencies?: Record<string, string>;
  };
  const used = new Set([
    ...Object.keys(pkg.dependencies ?? {}),
    ...Object.keys(pkg.devDependencies ?? {}),
  ]);
  return Object.keys(bundled)
    .filter((name) => used.has(name))
    .sort();
}

/** The names between the expo-governed markers of the mobile entry. */
function ignored(): string[] {
  const lines = readFileSync(join(root, ".github", "dependabot.yml"), "utf8").split("\n");
  const begin = lines.findIndex((l) => l.trim() === "# expo-governed: begin");
  const end = lines.findIndex((l) => l.trim() === "# expo-governed: end");
  expect(begin).toBeGreaterThan(-1);
  expect(end).toBeGreaterThan(begin);
  return lines
    .slice(begin + 1, end)
    .map((l) => {
      const m = /^\s*- dependency-name: "([^"]+)"\s*$/.exec(l);
      if (!m) throw new Error(`unexpected line in the expo-governed list: ${l}`);
      return m[1];
    })
    .sort();
}

describe("the Expo-governed Dependabot list", () => {
  it("names exactly the packages the installed SDK pins that the app uses", () => {
    expect(ignored()).toEqual(governed());
  });

  it("leaves expo itself to Dependabot", () => {
    expect(ignored()).not.toContain("expo");
  });
});
