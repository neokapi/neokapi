import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";
import { HARNESS_ROOT } from "../lib/paths.ts";
import { registryEntries, registrySource, type RegistryEntry } from "./registry.ts";

/** A demos/ tree with no captures anywhere — the state of a fresh checkout. */
function fixtureDemos(): string {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "harness-demos-"));
  const demo = (id: string, body: string) => {
    fs.mkdirSync(path.join(root, id), { recursive: true });
    fs.writeFileSync(path.join(root, id, "demo.yaml"), body);
  };
  demo(
    "02-beta",
    `id: 02-beta\ntitle: Beta — a Claude session\nsubtitle: second\nkind: use-case\nneedsAi: false\nprompt: do a thing\n`,
  );
  demo(
    "01-alpha",
    `id: 01-alpha\ntitle: Alpha — a scripted shell run\nsubtitle: first\nkind: use-case\nneedsAi: false\nterminal: shell\nscript:\n  - command: ls\n`,
  );
  // A retired demo keeps its manifest a level deeper, and a plain directory has
  // none at all; neither is a demo.
  fs.mkdirSync(path.join(root, "_retired", "00-old"), { recursive: true });
  fs.writeFileSync(
    path.join(root, "_retired", "00-old", "demo.yaml"),
    `id: 00-old\ntitle: Retired\nsubtitle: gone\nkind: use-case\nneedsAi: false\nprompt: x\n`,
  );
  fs.mkdirSync(path.join(root, "assets"), { recursive: true });
  return root;
}

test("registryEntries lists every authored demo, captured or not", () => {
  const root = fixtureDemos();
  try {
    assert.deepEqual(registryEntries(root), [
      { id: "01-alpha", title: "Alpha — a scripted shell run" },
      { id: "02-beta", title: "Beta — a Claude session" },
    ]);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("registryEntries is unchanged by a capture appearing next to the demos", () => {
  const root = fixtureDemos();
  try {
    const before = registryEntries(root);
    // Whatever a machine has recorded lives under public/, never under demos/.
    const pub = path.join(root, "..", path.basename(root) + "-public", "01-alpha");
    fs.mkdirSync(pub, { recursive: true });
    fs.writeFileSync(path.join(pub, "capture.json"), JSON.stringify({ id: "01-alpha", title: "A stale title" }));
    try {
      assert.deepEqual(registryEntries(root), before);
      assert.equal(before.length, 2);
    } finally {
      fs.rmSync(path.dirname(pub), { recursive: true, force: true });
    }
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("registrySource emits a module that evaluates to the entries", async () => {
  const entries: RegistryEntry[] = [
    { id: "01-alpha", title: 'Alpha "quoted" — em dash' },
    { id: "02-beta", title: "Beta" },
  ];
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "harness-registry-"));
  try {
    const file = path.join(dir, "registry.generated.ts");
    fs.writeFileSync(file, registrySource(entries));
    const mod = (await import(pathToFileURL(file).href)) as { DEMOS: RegistryEntry[] };
    assert.deepEqual(mod.DEMOS, entries);
    assert.match(fs.readFileSync(file, "utf8"), /^\/\/ Auto-generated from demos\/\*\/demo\.yaml/);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test("the tracked registry matches the manifests", () => {
  const file = path.join(HARNESS_ROOT, "src", "remotion", "registry.generated.ts");
  assert.equal(fs.readFileSync(file, "utf8"), registrySource(registryEntries()));
});
