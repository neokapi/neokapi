import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import type { DemoCapture, ScriptStep, TimelineEvent } from "../types.ts";
import {
  assertCaptureSound,
  classifyExit,
  expectedExitCodes,
  runScriptSteps,
  unexpectedExitMessage,
} from "./script-run.ts";

const outputs = (events: TimelineEvent[]) =>
  events.filter((e): e is Extract<TimelineEvent, { kind: "output" }> => e.kind === "output");

test("expectedExitCodes defaults to success and normalizes a declaration", () => {
  const cases: Array<{ name: string; step: ScriptStep; want: number[] }> = [
    { name: "undeclared", step: { command: "ls" }, want: [0] },
    { name: "single code", step: { command: "kapi check --strict", expectExit: 3 }, want: [3] },
    { name: "list", step: { command: "kgrep x *", expectExit: [0, 1] }, want: [0, 1] },
    { name: "declared success", step: { command: "ls", expectExit: 0 }, want: [0] },
  ];
  for (const c of cases) {
    assert.deepEqual(expectedExitCodes(c.step), c.want, c.name);
  }
});

test("classifyExit holds the declaration in both directions", () => {
  const cases: Array<{ name: string; step: ScriptStep; code: number | null; ok: boolean; reason?: string }> = [
    { name: "undeclared success", step: { command: "ls" }, code: 0, ok: true },
    {
      name: "undeclared operational failure is an error",
      step: { command: "kapi up" },
      code: 1,
      ok: false,
      reason: "expected exit 0, got 1",
    },
    {
      name: "undeclared gate failure is an error",
      step: { command: "kapi check --strict" },
      code: 3,
      ok: false,
      reason: "expected exit 0, got 3",
    },
    { name: "declared gate failure is expected", step: { command: "kapi check --strict", expectExit: 3 }, code: 3, ok: true },
    {
      name: "declared gate failure that passes is an error",
      step: { command: "kapi check --strict", expectExit: 3 },
      code: 0,
      ok: false,
      reason: "expected exit 3, got 0",
    },
    {
      name: "declared gate failure that errors operationally is an error",
      step: { command: "kapi check --strict", expectExit: 3 },
      code: 1,
      ok: false,
      reason: "expected exit 3, got 1",
    },
    { name: "either code accepted", step: { command: "kgrep x *", expectExit: [0, 1] }, code: 1, ok: true },
    {
      name: "a code outside the list is an error",
      step: { command: "kgrep x *", expectExit: [0, 1] },
      code: 2,
      ok: false,
      reason: "expected exit 0 or 1, got 2",
    },
    {
      name: "a signalled command is never expected",
      step: { command: "kapi up", expectExit: [0, 1] },
      code: null,
      ok: false,
      reason: "expected exit 0 or 1, got no exit code (terminated by a signal)",
    },
  ];
  for (const c of cases) {
    const outcome = classifyExit(c.step, c.code);
    assert.equal(outcome.ok, c.ok, c.name);
    if (c.reason !== undefined) assert.equal(outcome.reason, c.reason, c.name);
    if (c.ok) assert.equal(outcome.reason, "", c.name);
  }
});

test("runScriptSteps records an undeclared failure as an error", async () => {
  const run = await runScriptSteps(
    [
      { comment: "a beat with no command" },
      { command: "echo fine" },
      { command: "sh -c 'echo broke >&2; exit 1'" },
    ],
    { cwd: os.tmpdir(), env: process.env, timeoutMs: 30_000 },
  );

  assert.equal(run.commandCount, 2);
  assert.equal(run.errors.length, 1);
  assert.equal(run.errors[0]!.command, "sh -c 'echo broke >&2; exit 1'");
  assert.match(run.errors[0]!.snippet, /expected exit 0, got 1 — broke/);
  assert.deepEqual(
    outputs(run.events).map((e) => [e.text, e.isError]),
    [
      ["fine", false],
      ["broke", true],
    ],
  );
});

test("runScriptSteps accepts a declared non-zero exit", async () => {
  const run = await runScriptSteps([{ command: "sh -c 'echo gate failed; exit 3'", expectExit: 3 }], {
    cwd: os.tmpdir(),
    env: process.env,
    timeoutMs: 30_000,
  });

  assert.deepEqual(run.errors, []);
  // The take is sound, and the beat still reads as the failure it is on screen.
  assert.deepEqual(
    outputs(run.events).map((e) => [e.text, e.isError]),
    [["gate failed", true]],
  );
});

test("runScriptSteps flags a declared non-zero exit that does not happen", async () => {
  const run = await runScriptSteps([{ command: "true", expectExit: 3 }], {
    cwd: os.tmpdir(),
    env: process.env,
    timeoutMs: 30_000,
  });

  assert.equal(run.errors.length, 1);
  assert.equal(run.errors[0]!.snippet, "expected exit 3, got 0");
  assert.deepEqual(outputs(run.events), []);
});

test("runScriptSteps applies the output rewrite before recording", async () => {
  const run = await runScriptSteps([{ command: "echo /tmp/sandbox/demo" }], {
    cwd: os.tmpdir(),
    env: process.env,
    timeoutMs: 30_000,
    clean: (out) => out.split("/tmp/sandbox/demo").join("~/project"),
  });

  assert.deepEqual(outputs(run.events).map((e) => e.text), ["~/project"]);
});

test("assertCaptureSound rejects a persisted take that recorded an error", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "harness-capture-"));
  const write = (name: string, errors: DemoCapture["meta"]["errors"]): string => {
    const file = path.join(dir, name);
    const cap: DemoCapture = {
      id: "demo",
      title: "Demo",
      subtitle: "",
      aspects: [],
      prompt: "",
      events: [],
      meta: { model: "shell", durationMs: 0, numTurns: 0, capturedAt: "", errors },
    };
    fs.writeFileSync(file, JSON.stringify(cap));
    return file;
  };

  try {
    assert.doesNotThrow(() => assertCaptureSound("demo", write("clean.json", [])));
    assert.throws(
      () =>
        assertCaptureSound(
          "demo",
          write("dirty.json", [
            { tool: "shell", command: "kapi up", snippet: "expected exit 0, got 1", hardError: true },
          ]),
        ),
      /did not exit as declared/,
    );
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test("unexpectedExitMessage names every failing command", () => {
  const message = unexpectedExitMessage("s0-northsea-governance", [
    { tool: "shell", command: "kapi up", snippet: "expected exit 0, got 1", hardError: true },
    { tool: "shell", command: "kapi check --strict", snippet: "expected exit 3, got 0", hardError: true },
  ]);
  assert.match(message, /^demo s0-northsea-governance: 2 command\(s\) did not exit as declared:/);
  assert.match(message, /kapi up ↳ expected exit 0, got 1/);
  assert.match(message, /kapi check --strict ↳ expected exit 3, got 0/);
  assert.match(message, /expectExit:/);
});
