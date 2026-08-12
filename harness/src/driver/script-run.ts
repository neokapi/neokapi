/**
 * Driving a scripted shell demo's `script` steps and judging what came back.
 *
 * Kept apart from the sandbox setup in capture-script.ts so the loop that
 * decides whether a take is sound can be exercised without a sandbox, a kapi
 * binary or the public/ tree.
 */
import fs from "node:fs";
import type { CaptureError, DemoCapture, ScriptStep, TimelineEvent } from "../types.ts";
import { sh } from "../lib/exec.ts";

/** The verdict on one driven command: the code it produced against the codes its step declared. */
export interface ExitOutcome {
  /** The observed code is one the step declared. */
  ok: boolean;
  /** The codes the step declared. */
  expected: number[];
  /** Why the outcome is wrong ("expected exit 3, got 0"); empty when ok. */
  reason: string;
}

/** The exit codes a step declares as its expected outcome; 0 when it declares none. */
export function expectedExitCodes(step: ScriptStep): number[] {
  if (step.expectExit === undefined) return [0];
  return Array.isArray(step.expectExit) ? step.expectExit : [step.expectExit];
}

function describeExpected(codes: number[]): string {
  if (codes.length === 1) return `exit ${codes[0]}`;
  return `exit ${codes.slice(0, -1).join(", ")} or ${codes[codes.length - 1]}`;
}

/**
 * Judge one command's exit code against its step's declaration. A null code
 * means the child was killed by a signal (a timeout, typically) and never
 * reported one, which no declaration can make expected.
 */
export function classifyExit(step: ScriptStep, code: number | null): ExitOutcome {
  const expected = expectedExitCodes(step);
  if (code !== null && expected.includes(code)) return { ok: true, expected, reason: "" };
  const got = code === null ? "no exit code (terminated by a signal)" : String(code);
  return { ok: false, expected, reason: `expected ${describeExpected(expected)}, got ${got}` };
}

/** What one pass over a demo's script produced. */
export interface ScriptRun {
  /** The recorded timeline: a comment beat, or a command and its real output. */
  events: TimelineEvent[];
  /** Commands whose exit code was not the one their step declared. Empty = sound take. */
  errors: CaptureError[];
  /** How many steps actually ran a command (comments excluded). */
  commandCount: number;
}

export interface ScriptRunOptions {
  cwd: string;
  env: NodeJS.ProcessEnv;
  /** Per-command timeout. */
  timeoutMs: number;
  /** Rewrite a command's combined output before it is recorded. */
  clean?: (out: string) => string;
}

/**
 * Run each script step for real and record it. The output event is marked as an
 * error whenever the command exited non-zero, declared or not — the screencast
 * shows what happened, and a gate that failed on camera should look like one.
 * The declaration decides something else: whether the take is sound enough to
 * narrate and ship.
 */
export async function runScriptSteps(steps: ScriptStep[], opts: ScriptRunOptions): Promise<ScriptRun> {
  const events: TimelineEvent[] = [];
  const errors: CaptureError[] = [];
  let i = 0;
  let commandCount = 0;

  for (const step of steps) {
    if (step.comment !== undefined) {
      events.push({ i: i++, kind: "comment", text: step.comment });
      continue;
    }
    const command = step.command;
    if (!command) continue;
    commandCount++;
    events.push({ i: i++, kind: "command", text: command });

    const r = await sh(command, { cwd: opts.cwd, env: opts.env, timeoutMs: opts.timeoutMs });
    // Both streams in arrival order, so a command that reports progress on
    // stderr and its result on stdout records the way it read in the terminal.
    let out = r.combined.replace(/\n+$/, "");
    if (opts.clean) out = opts.clean(out);

    const failed = r.code !== 0;
    if (out !== "" || failed) events.push({ i: i++, kind: "output", text: out, isError: failed });

    const outcome = classifyExit(step, r.code);
    if (!outcome.ok) {
      const tail = (r.stderr || r.stdout)
        .split("\n")
        .map((l) => l.trim())
        .filter(Boolean)
        .slice(0, 2)
        .join(" ");
      errors.push({
        tool: "shell",
        command: command.slice(0, 100),
        snippet: [outcome.reason, tail].filter(Boolean).join(" — ").slice(0, 200),
        hardError: true,
      });
    }
  }

  return { events, errors, commandCount };
}

/** The message naming every command that did not exit the way its step declared. */
export function unexpectedExitMessage(id: string, errors: CaptureError[]): string {
  const lines = errors.map((e) => `  ✗ ${e.command} ↳ ${e.snippet}`).join("\n");
  return (
    `demo ${id}: ${errors.length} command(s) did not exit as declared:\n${lines}\n` +
    `Fix the demo, or declare the outcome on the step with "expectExit:".`
  );
}

/**
 * Re-assert a persisted take's integrity. The capture stage skips a demo whose
 * capture.json is already on disk, so without this a broken take would fail the
 * run that recorded it and pass every run after.
 */
export function assertCaptureSound(id: string, captureJson: string): void {
  const cap = JSON.parse(fs.readFileSync(captureJson, "utf8")) as DemoCapture;
  const errors = cap.meta?.errors ?? [];
  if (errors.length > 0) throw new Error(unexpectedExitMessage(id, errors));
}
