/**
 * migrate-keys end-to-end: a dictionary compiled under the v1 key
 * scheme is rewritten so every entry resolves under the v2 keys the
 * current extractor produces, and .klf block hashes migrate in place
 * with targets preserved.
 */

import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";

import { extractDocument } from "../src/extract/walker.ts";
import { legacyHashKey } from "../src/migrate/legacy.ts";
import { runMigrateKeys } from "../src/commands/migrate-keys.ts";

const tmp: string[] = [];
afterEach(() => {
  for (const d of tmp.splice(0)) rmSync(d, { recursive: true, force: true });
});

function scratch(): string {
  const dir = mkdtempSync(join(tmpdir(), "kapi-migrate-"));
  tmp.push(dir);
  return dir;
}

const SOURCE = `export function Page() {
  return (
    <div>
      <section>
        <h1>Welcome back</h1>
        <p>Click <a href="/x">here</a> to continue.</p>
      </section>
      <button aria-label="Close dialog">Save changes</button>
      <input placeholder="Search..." />
    </div>
  );
}
`;

describe("migrate-keys", () => {
  it("collects (legacyHash, hash) pairs through the walker", () => {
    const pairs: Array<{ legacyHash: string | null; hash: string }> = [];
    extractDocument(SOURCE, { filename: "Page.tsx", onKeyPair: (p) => pairs.push(p) });
    expect(pairs.length).toBeGreaterThanOrEqual(4);
    // v1 hashed "Welcome back" under the full ancestor path.
    const v1Welcome = legacyHashKey("Welcome back", "div > section > h1");
    expect(pairs.map((p) => p.legacyHash)).toContain(v1Welcome);
    // Every pair maps to a distinct v2 hash.
    expect(new Set(pairs.map((p) => p.hash)).size).toBe(pairs.length);
  });

  it("rewrites a v1 dictionary so it resolves under v2 keys", async () => {
    const dir = scratch();
    mkdirSync(join(dir, "src"), { recursive: true });
    writeFileSync(join(dir, "src", "Page.tsx"), SOURCE);
    mkdirSync(join(dir, "translations"));

    // A dict exactly as a v1 compile would have produced it.
    const v1Dict = {
      [legacyHashKey("Welcome back", "div > section > h1")]: "Willkommen zurück",
      [legacyHashKey("Click {=m0}here{/=m0} to continue.", "div > section > p")]:
        "Klicken Sie {=m0}hier{/=m0}, um fortzufahren.",
      [legacyHashKey("Save changes", "div > button")]: "Änderungen speichern",
      [legacyHashKey("Close dialog", "div > button[aria-label]")]: "Dialog schließen",
      [legacyHashKey("Search...", "div > input[placeholder]")]: "Suchen...",
      zzzOrphan: "Verwaist",
    };
    writeFileSync(join(dir, "translations", "de.json"), JSON.stringify(v1Dict));

    const cwd = process.cwd();
    process.chdir(dir);
    try {
      await runMigrateKeys(["--dicts", "translations"]);
    } finally {
      process.chdir(cwd);
    }

    const migrated = JSON.parse(
      readFileSync(join(dir, "translations", "de.json"), "utf-8"),
    ) as Record<string, string>;

    // Every migrated entry must resolve under the hashes the current
    // extractor emits.
    const doc = extractDocument(SOURCE, { filename: "src/Page.tsx" })!;
    const byHash = new Map(doc.blocks.map((b) => [b.hash, b]));
    let resolved = 0;
    for (const block of doc.blocks) {
      if (migrated[block.hash]) resolved++;
    }
    expect(resolved).toBe(5);
    expect(byHash.size).toBe(5);
    // Orphan kept (default) for later TM rescue.
    expect(migrated.zzzOrphan).toBe("Verwaist");
  });

  it("rewrites .klf block hashes in place, preserving targets", async () => {
    const dir = scratch();
    mkdirSync(join(dir, "src"), { recursive: true });
    writeFileSync(join(dir, "src", "Page.tsx"), SOURCE);
    mkdirSync(join(dir, "i18n", "src"), { recursive: true });

    const v1Hash = legacyHashKey("Welcome back", "div > section > h1");
    const klf = {
      schemaVersion: "1.0",
      kind: "kapi-localization-format",
      documents: [
        {
          id: "src/Page.tsx",
          path: "src/Page.tsx",
          blocks: [
            {
              id: "src/Page.tsx:4:0",
              hash: v1Hash,
              translatable: true,
              type: "jsx:element",
              source: [{ text: "Welcome back" }],
              targets: { de: { runs: [{ text: "Willkommen zurück" }], status: "reviewed" } },
            },
          ],
        },
      ],
    };
    writeFileSync(join(dir, "i18n", "src", "Page.klf"), JSON.stringify(klf));

    const cwd = process.cwd();
    process.chdir(dir);
    try {
      await runMigrateKeys(["--klf", "i18n"]);
    } finally {
      process.chdir(cwd);
    }

    const rewritten = JSON.parse(readFileSync(join(dir, "i18n", "src", "Page.klf"), "utf-8"));
    const block = rewritten.documents[0].blocks[0];
    const doc = extractDocument(SOURCE, { filename: "src/Page.tsx" })!;
    const v2Welcome = doc.blocks.find(
      (b) => "text" in b.source[0] && (b.source[0] as { text: string }).text === "Welcome back",
    )!.hash;
    expect(block.hash).toBe(v2Welcome);
    // Targets ride along untouched.
    expect(block.targets.de.status).toBe("reviewed");
    expect(block.targets.de.runs[0].text).toBe("Willkommen zurück");
  });
});
