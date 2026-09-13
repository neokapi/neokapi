import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import type { DemoManifest, TimelineEvent } from "../types.ts";
import { normalizeTranscript, parseHookResponse } from "./normalize.ts";

// The notices below are the ones host/hook.go writes: hookUnheard for gates that
// did not run, hookUntrusted for a checker that failed its canary. Each is the
// JSON a hook encodes, a systemMessage and no decision.
const notice = (message: string) => JSON.stringify({ systemMessage: message }) + "\n";
const NOTHING_TO_CHECK = notice(
  "kapi hook stop: the guard did not run: the gates for /tmp/p/kapi.yaml did not run: there was nothing in scope to check (nothing_to_check). " +
    "Allowing, because a kapi hook fails open. Treat the result as unverified: the guard never ran.",
);
const CHECKER_INVALID = notice(
  "kapi hook stop: a checker is broken, so this result cannot be trusted (checker_invalid): the gates for /tmp/p/kapi.yaml: " +
    "term-check reported no finding on its canary, so its result cannot be trusted. " +
    "Allowing, because a kapi hook fails open. Do not report the content as checked until the checker is fixed.",
);
const UNHEARD = notice(
  "kapi hook stop: the guard did not run: the project could not be located: permission denied. " +
    "Allowing, because a kapi hook fails open. Treat the result as unverified: the guard never ran.",
);
const BLOCK = JSON.stringify({ decision: "block", reason: "kapi check failed:\nERROR [term-check] a.md: use sign in\n" }) + "\n";
const PASS = "";

const manifest = { id: "t", title: "t", subtitle: "", aspects: [], prompt: "p" } as unknown as DemoManifest;

/** Normalize a transcript whose only stream events are Stop hook responses with these stdouts. */
function hookTimeline(outputs: string[]): TimelineEvent[] {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "normalize-test-"));
  try {
    const file = path.join(dir, "capture.jsonl");
    const lines = outputs.map((output) =>
      JSON.stringify({ type: "system", subtype: "hook_response", hook_name: "Stop", output }),
    );
    fs.writeFileSync(file, lines.join("\n") + "\n");
    return normalizeTranscript(file, manifest).events.filter((e) => e.kind.startsWith("hook_"));
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

const kinds = (events: TimelineEvent[]) => events.map((e) => e.kind);

test("parseHookResponse tells a did-not-run notice from a block and a pass", () => {
  assert.deepEqual(parseHookResponse(PASS), { kind: "pass" });
  assert.equal(parseHookResponse(BLOCK)?.kind, "block");
  assert.deepEqual(
    [NOTHING_TO_CHECK, CHECKER_INVALID, UNHEARD].map((o) => {
      const r = parseHookResponse(o);
      return r?.kind === "did_not_run" ? { kind: r.kind, cause: r.cause } : r;
    }),
    [
      { kind: "did_not_run", cause: "nothing_to_check" },
      { kind: "did_not_run", cause: "checker_invalid" },
      { kind: "did_not_run", cause: undefined },
    ],
  );
  assert.equal(parseHookResponse("not json"), null);
});

test("a did-not-run notice after a block is not a pass", () => {
  const events = hookTimeline([BLOCK, NOTHING_TO_CHECK]);
  assert.deepEqual(kinds(events), ["hook_block", "hook_did_not_run"]);
  const dnr = events[1] as Extract<TimelineEvent, { kind: "hook_did_not_run" }>;
  assert.equal(dnr.cause, "nothing_to_check");
});

test("a did-not-run notice with no block before it is kept", () => {
  const cases: Array<{ name: string; output: string; cause?: string }> = [
    { name: "checker_invalid", output: CHECKER_INVALID, cause: "checker_invalid" },
    { name: "nothing_to_check", output: NOTHING_TO_CHECK, cause: "nothing_to_check" },
    { name: "a guard that never ran names no cause", output: UNHEARD, cause: undefined },
  ];
  for (const c of cases) {
    const events = hookTimeline([c.output]);
    assert.deepEqual(kinds(events), ["hook_did_not_run"], c.name);
    const dnr = events[0] as Extract<TimelineEvent, { kind: "hook_did_not_run" }>;
    assert.equal(dnr.cause, c.cause, c.name);
    assert.match(dnr.message, /^kapi hook stop: /, c.name);
  }
});

test("a pass after a block is still the resolution", () => {
  assert.deepEqual(kinds(hookTimeline([BLOCK, PASS])), ["hook_block", "hook_pass"]);
  // A notice between them leaves the block unresolved until the gates pass.
  assert.deepEqual(kinds(hookTimeline([BLOCK, NOTHING_TO_CHECK, PASS])), [
    "hook_block",
    "hook_did_not_run",
    "hook_pass",
  ]);
  // A pass with no block before it adds nothing.
  assert.deepEqual(kinds(hookTimeline([PASS, NOTHING_TO_CHECK, PASS])), ["hook_did_not_run"]);
});
