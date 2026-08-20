/**
 * `--source-root`: the directory every recorded source path is relative to.
 *
 * A surface whose `--src` reaches outside its own package records, by default,
 * a path relative to wherever the extract ran — `../../apps/bowrain/frontend/
 * src/App.tsx`. That names a real file only to someone who knows which
 * directory that was, and a surface holding the catalog (bowrain, an ocean away
 * from the checkout) does not: it showed a reviewer a row reading
 * `apps/bowrain/frontend/src/App.kbf.json`, which looks like a repository path
 * and is not one.
 *
 * The root is declared rather than derived, because this path is the document's
 * identity as well as what a reviewer reads — it spells `path`, `id` and every
 * block id under it. A root inferred from whatever globs matched would re-key a
 * whole collection the day someone added a `--src`.
 */

import { describe, expect, it } from "vitest";
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import type { File } from "@neokapi/kapi-format";
import { runExtract } from "../src/commands/extract.ts";

/**
 * A workspace shaped like the real one: a package that is extracted from, and a
 * sibling package whose components it also ships.
 *
 *   <root>/packages/app/src/Local.tsx
 *   <root>/packages/ui/src/Shared.tsx
 */
function tempWorkspace() {
  const root = mkdtempSync(join(tmpdir(), "i18n-react-source-root-"));
  mkdirSync(join(root, "packages", "app", "src"), { recursive: true });
  mkdirSync(join(root, "packages", "ui", "src"), { recursive: true });
  writeFileSync(join(root, "packages", "app", "src", "Local.tsx"), "<h1>Local heading</h1>;");
  writeFileSync(join(root, "packages", "ui", "src", "Shared.tsx"), "<h1>Shared heading</h1>;");
  return root;
}

async function extractFrom(root: string, args: string[]) {
  const cwd = process.cwd();
  process.chdir(join(root, "packages", "app"));
  try {
    await runExtract(args);
  } finally {
    process.chdir(cwd);
  }
}

function bundle(path: string): File {
  return JSON.parse(readFileSync(path, "utf8")) as File;
}

const SRC = ["--src", "src/**/*.tsx", "--src", "../ui/src/**/*.tsx"];

describe("extract --source-root", () => {
  it("records every source against the declared root", async () => {
    const root = tempWorkspace();
    await extractFrom(root, [...SRC, "--out", "i18n", "--source-root", "../.."]);

    const app = join(root, "packages", "app", "i18n");
    expect(bundle(join(app, "packages/app/src/Local.kbf.json")).documents[0].path).toBe(
      "packages/app/src/Local.tsx",
    );
    // The sibling package reads as where it lives, not as how it was reached.
    expect(bundle(join(app, "packages/ui/src/Shared.kbf.json")).documents[0].path).toBe(
      "packages/ui/src/Shared.tsx",
    );
  });

  it("keeps the catalog tree a mirror of the declared paths", async () => {
    const root = tempWorkspace();
    await extractFrom(root, [...SRC, "--out", "i18n", "--source-root", "../.."]);

    // No `../` to strip, so no two roots can flatten onto one another and every
    // catalog sits where its source does.
    const shared = bundle(join(root, "packages", "app", "i18n", "packages/ui/src/Shared.kbf.json"));
    expect(shared.documents[0].blocks[0].properties?.file).toBe("packages/ui/src/Shared.tsx");
  });

  it("records cwd-relative paths when no root is declared", async () => {
    const root = tempWorkspace();
    await extractFrom(root, [...SRC, "--out", "i18n"]);

    const app = join(root, "packages", "app", "i18n");
    // Unchanged for a surface that declares nothing: its own src reads the same,
    // and a sibling still climbs out — which is the situation the flag exists for.
    expect(bundle(join(app, "src/Local.kbf.json")).documents[0].path).toBe("src/Local.tsx");
    expect(bundle(join(app, "ui/src/Shared.kbf.json")).documents[0].path).toBe(
      "../ui/src/Shared.tsx",
    );
  });

  // The path spells the block ids, which is exactly why the root is declared
  // and not derived: this is the churn a shifting root would cause silently.
  it("re-keys blocks when the declared root changes", async () => {
    const root = tempWorkspace();
    await extractFrom(root, [...SRC, "--out", "cwd"]);
    await extractFrom(root, [...SRC, "--out", "rooted", "--source-root", "../.."]);

    const app = join(root, "packages", "app");
    const cwdID = bundle(join(app, "cwd", "ui/src/Shared.kbf.json")).documents[0].blocks[0].id;
    const rootedID = bundle(join(app, "rooted", "packages/ui/src/Shared.kbf.json")).documents[0]
      .blocks[0].id;

    expect(cwdID).not.toBe(rootedID);
    expect(cwdID).toContain("../ui/src/Shared.tsx");
    expect(rootedID).toContain("packages/ui/src/Shared.tsx");
  });

  // The hash is what the running component looks its string up by, and it is
  // taken over the text and its descriptor — never over the path. So moving the
  // root re-keys the store without orphaning a single translated catalog entry.
  it("leaves the message hash alone", async () => {
    const root = tempWorkspace();
    await extractFrom(root, [...SRC, "--out", "cwd"]);
    await extractFrom(root, [...SRC, "--out", "rooted", "--source-root", "../.."]);

    const app = join(root, "packages", "app");
    const cwdHash = bundle(join(app, "cwd", "ui/src/Shared.kbf.json")).documents[0].blocks[0].hash;
    const rootedHash = bundle(join(app, "rooted", "packages/ui/src/Shared.kbf.json")).documents[0]
      .blocks[0].hash;

    expect(rootedHash).toBe(cwdHash);
  });
});
