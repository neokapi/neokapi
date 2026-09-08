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

const DESKTOP = `title: A walk\nsubtitle: one line\nkind: use-case\nneedsAi: false\nterminal: desktop\n`;

function loads(id: string, yaml: string): ReturnType<typeof loadManifest> {
  const root = demoTree(id, yaml);
  try {
    return loadManifest(id, root);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
}

function rejects(id: string, yaml: string, re: RegExp, name: string): void {
  const root = demoTree(id, yaml);
  try {
    assert.throws(() => loadManifest(id, root), re, name);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
}

test("loadManifest keeps the beat fields a desktop scene declares", () => {
  const m = loads(
    "walk",
    `${DESKTOP}narration:\n  - id: a\n    kind: desktop\n    beat: a\n    hold: 6.5\n    crop: { selector: '[data-x]' }\n    zoom: 1.8\n    highlight: { selector: '[data-y]' }\n  - id: b\n    kind: desktop\n    beat: b\n    crop: { x: 0.1, y: 0.2, w: 0.5, h: 0.4 }\n    highlight: { box: { x: 0, y: 0, w: 1, h: 0.5 } }\n`,
  );
  const [a, b] = m.narration;
  assert.equal(a!.hold, 6.5);
  assert.deepEqual(a!.crop, { selector: "[data-x]" });
  assert.equal(a!.zoom, 1.8);
  assert.deepEqual(a!.highlight, { selector: "[data-y]" });
  assert.deepEqual(b!.crop, { x: 0.1, y: 0.2, w: 0.5, h: 0.4 });
  assert.deepEqual(b!.highlight, { box: { x: 0, y: 0, w: 1, h: 0.5 } });
});

test("loadManifest normalizes a bare-string highlight and keeps through on a shell scene", () => {
  const m = loads(
    "shell",
    `${HEAD}script:\n  - comment: first\n  - command: kapi check\n  - command: cat x\nnarration:\n  - id: t\n    kind: terminal\n    text: hello\n    highlight: CRITICAL\n    through: 2\n  - id: u\n    kind: terminal\n    text: world\n    through: 3\n`,
  );
  assert.deepEqual(m.narration[0]!.highlight, { text: "CRITICAL" });
  assert.equal(m.narration[0]!.through, 2);
  assert.equal(m.narration[1]!.through, 3);
});

test("loadManifest keeps the outro card's two lines", () => {
  const m = loads("shell", `${HEAD}script:\n  - command: ls\noutro:\n  line: Try it on one file\n  pointer: kapi check --ship\n`);
  assert.deepEqual(m.outro, { line: "Try it on one file", pointer: "kapi check --ship" });
});

test("loadManifest rejects beat fields that do not fit the scene", () => {
  const cases: Array<{ name: string; yaml: string; re: RegExp }> = [
    { name: "negative hold", yaml: `${DESKTOP}narration:\n  - { id: a, kind: desktop, beat: a, hold: -1 }\n`, re: /hold must be/ },
    { name: "crop on a terminal", yaml: `${HEAD}script:\n  - command: ls\nnarration:\n  - { id: a, kind: terminal, text: x, crop: { x: 0, y: 0, w: 1, h: 1 } }\n`, re: /crop applies to desktop/ },
    { name: "crop with both forms", yaml: `${DESKTOP}narration:\n  - { id: a, kind: desktop, beat: a, crop: { selector: s, x: 0, y: 0, w: 1, h: 1 } }\n`, re: /never both/ },
    { name: "crop box outside the window", yaml: `${DESKTOP}narration:\n  - { id: a, kind: desktop, beat: a, crop: { x: 0.5, y: 0, w: 0.6, h: 1 } }\n`, re: /inside the window/ },
    { name: "zoom out of range", yaml: `${DESKTOP}narration:\n  - { id: a, kind: desktop, beat: a, zoom: 4 }\n`, re: /zoom must be/ },
    { name: "highlight with two forms", yaml: `${DESKTOP}narration:\n  - { id: a, kind: desktop, beat: a, highlight: { selector: s, box: { x: 0, y: 0, w: 1, h: 1 } } }\n`, re: /exactly one/ },
    { name: "text highlight on a desktop scene", yaml: `${DESKTOP}narration:\n  - { id: a, kind: desktop, beat: a, highlight: FAIL }\n`, re: /highlight.text applies to terminal/ },
    { name: "through past the script", yaml: `${HEAD}script:\n  - command: ls\nnarration:\n  - { id: a, kind: terminal, text: x, through: 2 }\n`, re: /from 1 to 1/ },
    { name: "through running backwards", yaml: `${HEAD}script:\n  - command: ls\n  - command: pwd\nnarration:\n  - { id: a, kind: terminal, text: x, through: 2 }\n  - { id: b, kind: terminal, text: y, through: 1 }\n`, re: /never runs backwards/ },
    { name: "through on a Claude demo", yaml: `title: A demo\nsubtitle: s\nkind: use-case\nneedsAi: false\nprompt: do it\nnarration:\n  - { id: a, kind: terminal, text: x, through: 1 }\n`, re: /scripted shell demos only/ },
    { name: "outro as a string", yaml: `${HEAD}script:\n  - command: ls\noutro: bye\n`, re: /outro must be a map/ },
  ];
  for (const c of cases) rejects("demo", c.yaml, c.re, c.name);
});
