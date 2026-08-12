import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { listDemoIds, loadManifest } from "./manifest.ts";

const HEAD = `title: A demo\nsubtitle: one line\nkind: use-case\nneedsAi: false\nterminal: shell\n`;

/** Write a one-demo tree and return its root, so a bad manifest fails in isolation. */
function demoTree(id: string, yaml: string): string {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "harness-manifest-"));
  fs.mkdirSync(path.join(root, id), { recursive: true });
  fs.writeFileSync(path.join(root, id, "demo.yaml"), yaml);
  return root;
}

test("listDemoIds skips directories that hold no manifest of their own", () => {
  const root = demoTree("01-alpha", `${HEAD}script:\n  - command: ls\n`);
  try {
    fs.mkdirSync(path.join(root, "_retired", "00-old"), { recursive: true });
    fs.writeFileSync(path.join(root, "_retired", "00-old", "demo.yaml"), `${HEAD}script:\n  - command: ls\n`);
    fs.mkdirSync(path.join(root, "fixtures"), { recursive: true });
    assert.deepEqual(listDemoIds(root), ["01-alpha"]);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("loadManifest keeps a step's declared exit codes", () => {
  const root = demoTree(
    "01-alpha",
    `${HEAD}script:\n  - command: ls\n  - command: kapi check --strict\n    expectExit: 3\n  - command: kgrep x *\n    expectExit: [0, 1]\n`,
  );
  try {
    const script = loadManifest("01-alpha", root).script!;
    assert.equal(script[0]!.expectExit, undefined);
    assert.equal(script[1]!.expectExit, 3);
    assert.deepEqual(script[2]!.expectExit, [0, 1]);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("loadManifest rejects an exit declaration that is not exit codes", () => {
  const cases: Array<{ name: string; declared: string }> = [
    { name: "empty list", declared: "[]" },
    { name: "a word", declared: '"gate"' },
    { name: "a fraction", declared: "1.5" },
    { name: "a list with a word in it", declared: "[0, ok]" },
  ];
  for (const c of cases) {
    const root = demoTree("01-alpha", `${HEAD}script:\n  - command: ls\n    expectExit: ${c.declared}\n`);
    try {
      assert.throws(() => loadManifest("01-alpha", root), /script step 1 declares expectExit/, c.name);
    } finally {
      fs.rmSync(root, { recursive: true, force: true });
    }
  }
});
