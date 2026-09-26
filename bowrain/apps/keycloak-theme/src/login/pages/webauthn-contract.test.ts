/**
 * The WebAuthn pages render their own markup but run Keycloak's scripts,
 * which keycloakify ships and which write the ceremony's result into hidden
 * inputs by id. A script that writes to an id the page lacks throws before
 * it submits, and Keycloak records the attempt as a failed registration: no
 * user can enrol a passkey, and the build, type check and lint all pass.
 *
 * keycloakify's own page for each template renders what its scripts expect,
 * so every id a script looks up that the stock page also renders must be
 * rendered here too. The rule compares against the installed keycloakify, so
 * a dependency bump that adds a field fails this test instead of sign-up.
 */
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { describe, expect, it } from "vite-plus/test";

const require = createRequire(import.meta.url);
const keycloakifyDir = dirname(require.resolve("keycloakify/package.json"));
const scriptsDir = join(keycloakifyDir, "res/public/keycloakify-dev-resources/login/js");

const cases = [
  {
    page: "WebauthnRegister.tsx",
    stockPage: "WebauthnRegister.js",
    scripts: ["webauthnRegister.js"],
  },
  {
    page: "WebauthnAuthenticate.tsx",
    stockPage: "WebauthnAuthenticate.js",
    scripts: ["webauthnAuthenticate.js"],
  },
  {
    page: "LoginPasskeysConditionalAuthenticate.tsx",
    stockPage: "LoginPasskeysConditionalAuthenticate.js",
    scripts: ["passkeysConditionalAuth.js", "webauthnAuthenticate.js"],
  },
];

function matches(source: string, pattern: RegExp): Set<string> {
  return new Set(Array.from(source.matchAll(pattern), (m) => m[1]));
}

describe("WebAuthn pages render every element Keycloak's scripts write to", () => {
  for (const { page, stockPage, scripts } of cases) {
    it(page, () => {
      const scriptIds = new Set<string>();
      for (const script of scripts) {
        const source = readFileSync(join(scriptsDir, script), "utf8");
        for (const id of matches(source, /getElementById\(\s*["']([\w-]+)["']\s*\)/g)) {
          scriptIds.add(id);
        }
        for (const id of matches(source, /document\.forms\[\s*["']([\w-]+)["']\s*\]/g)) {
          scriptIds.add(id);
        }
      }
      expect(scriptIds.size).toBeGreaterThan(0);

      const stock = readFileSync(join(keycloakifyDir, "login/pages", stockPage), "utf8");
      const stockIds = matches(stock, /\bid: "([\w-]+)"/g);
      const ours = matches(
        readFileSync(new URL(page, import.meta.url), "utf8"),
        /\bid="([\w-]+)"/g,
      );

      const required = [...scriptIds].filter((id) => stockIds.has(id)).sort();
      const missing = required.filter((id) => !ours.has(id));
      expect(missing, `${page} lacks elements its Keycloak script writes to`).toEqual([]);
    });
  }
});
