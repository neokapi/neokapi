import fs from "node:fs";
import type { CaptureError, DemoCapture, DemoManifest, TimelineEvent } from "../types.ts";

/**
 * Patterns that mark a genuine kapi/tool failure (after bridge-warning noise is
 * stripped). Bridge startup warnings like "Could not create FilterInfo" are NOT here
 * — they are not failures.
 */
const ERROR_PATTERNS: RegExp[] = [
  /(^|\n)\s*Error:/,
  /\bException\b/,
  /Traceback \(most recent call last\)/,
  /(^|\n)panic:/,
  // Exit 3 is kapi's quality-gate code (brand --min-score / `kapi verify`): an expected
  // step in the produce→verify→fix loop, not a failure. Flag other non-zero exits.
  /Exit code (1|2|[4-9])\b/i,
  /exit status (1|2|[4-9])\b/,
  /no such function/,
  /authentication_error|invalid x-api-key|API error \d{3}/,
  /FileNotFoundException|OkapiException|OkapiIllegalFilterOperationException/,
  /unknown (flag|shorthand flag|command|flow)/,
  /<tool_use_error>/,
];

/** Detect kapi/tool failures in the normalized timeline (record-time audit). */
export function detectErrors(events: TimelineEvent[]): CaptureError[] {
  const titleById = new Map<string, string>();
  const toolById = new Map<string, string>();
  for (const e of events) {
    if (e.kind === "tool_use") {
      titleById.set(e.id, e.command ? `${e.tool}: ${e.command}` : `${e.tool} ${e.detail ?? ""}`.trim());
      toolById.set(e.id, e.tool);
    }
  }
  const out: CaptureError[] = [];
  for (const e of events) {
    if (e.kind !== "tool_result") continue;
    // kapi quality-gate failure is an expected step in the verify loop, even though the
    // shell marks the tool_result is_error. It surfaces as "Exit code 3" or, when piped
    // (so the exit code is masked), as the sentinel message "quality gate failed". Don't
    // count either as a capture error.
    if (/Exit code 3\b/.test(e.output) || /quality gate failed/i.test(e.output)) continue;
    const hard = e.isError;
    const matched = ERROR_PATTERNS.some((p) => p.test(e.output));
    if (!hard && !matched) continue;
    const snippet =
      e.output
        .split("\n")
        .map((l) => l.trim())
        .filter((l) => l && (hard || ERROR_PATTERNS.some((p) => p.test(l))))
        .slice(0, 2)
        .join(" | ")
        .slice(0, 240) || "(is_error, no output)";
    out.push({
      tool: toolById.get(e.forId) ?? "unknown",
      command: titleById.get(e.forId) ?? "(unknown command)",
      snippet,
      hardError: hard,
    });
  }
  return out;
}

/** Lines emitted by the okapi-bridge subprocess and JVM that are pure noise on screen. */
const BRIDGE_NOISE = [
  /^\[bridge\]/,
  /io\.netty\./,
  /^WARNING:/,
  /^[A-Z][a-z]{2} \d{1,2}, \d{4} .* (AM|PM) /, // java.util.logging timestamps
  /Could not create FilterInfo/,
  /SO_KEEPALIVE/,
  /declared in both/, // plugin discovered via two configured dirs (harmless)
  /remove the other to silence this warning/,
];

/** Strip subprocess noise + Read tool line-number prefixes so terminal output reads cleanly. */
export function cleanOutput(raw: string, tool?: string): string {
  // Drop Claude Code's injected <system-reminder> blocks — they are harness/tool meta,
  // not real command output, and shouldn't appear on screen.
  raw = raw.replace(/<system-reminder>[\s\S]*?<\/system-reminder>/g, "").trim();
  let lines = raw.split("\n");
  lines = lines.filter((l) => !BRIDGE_NOISE.some((re) => re.test(l)));
  if (tool === "Read") {
    // Read returns "   123\tcontent" — drop the cat -n style prefix.
    lines = lines.map((l) => l.replace(/^\s*\d+\t/, ""));
  }
  let out = lines.join("\n").replace(/\n{3,}/g, "\n\n").trim();
  const MAX = 4000;
  if (out.length > MAX) out = out.slice(0, MAX) + "\n… (truncated)";
  return out;
}

function toolTitle(tool: string, input: Record<string, unknown>): {
  title: string;
  command?: string;
  detail?: string;
} {
  switch (tool) {
    case "Bash":
      return {
        title: (input.description as string) || "Run command",
        command: input.command as string,
      };
    case "Read":
      return { title: "Read", detail: input.file_path as string };
    case "Write":
      return { title: "Write", detail: input.file_path as string };
    case "Edit":
    case "MultiEdit":
      return { title: "Edit", detail: input.file_path as string };
    case "Glob":
      return { title: "Glob", detail: input.pattern as string };
    case "Grep":
      return { title: "Grep", detail: (input.pattern as string) || "" };
    case "Skill":
      return { title: "Skill", detail: input.command as string };
    case "TodoWrite":
      return { title: "Plan", detail: "update task list" };
    default: {
      // MCP tools are named mcp__<server>__<tool> — show them as kapi tool cards.
      const mcp = tool.match(/^mcp__([^_]+)__(.+)$/);
      if (mcp) {
        const argStr = Object.values(input)
          .filter((v) => typeof v === "string")
          .join(" ");
        return { title: `${mcp[1]} · ${mcp[2]}`, detail: argStr.slice(0, 120) };
      }
      return { title: tool, detail: JSON.stringify(input).slice(0, 120) };
    }
  }
}

interface StreamEvent {
  type: string;
  subtype?: string;
  message?: { content?: unknown[]; role?: string };
  cwd?: string;
  model?: string;
  duration_ms?: number;
  num_turns?: number;
  total_cost_usd?: number;
  result?: string;
  /** system/hook_response carries the hook's stdout (a decision JSON, or empty). */
  hook_name?: string;
  output?: string;
  stdout?: string;
}

/**
 * Parse a Stop-hook response's stdout. `{"decision":"block","reason":…}` → a block
 * with the findings (the "ERROR/WARNING [gate] …" lines pulled out of the reason);
 * empty output → an allow (the gates passed). Anything else → null (ignore).
 */
function parseHookResponse(raw?: string): { block: boolean; reason: string; findings: string[] } | null {
  const text = (raw ?? "").trim();
  if (!text) return { block: false, reason: "", findings: [] };
  try {
    const o = JSON.parse(text) as { decision?: string; reason?: string };
    if (o.decision !== "block") return { block: false, reason: "", findings: [] };
    const reason = String(o.reason ?? "");
    const findings = reason
      .split("\n")
      .map((l) => l.trim())
      .filter((l) => /^(ERROR|WARNING)\b/i.test(l));
    return { block: true, reason, findings };
  } catch {
    return null;
  }
}

/** Convert a captured stream-json (.jsonl) into the renderer's DemoCapture timeline. */
export function normalizeTranscript(jsonlPath: string, m: DemoManifest): DemoCapture {
  const raw = fs.readFileSync(jsonlPath, "utf8");
  const stream: StreamEvent[] = raw
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean)
    .map((l) => {
      try {
        return JSON.parse(l) as StreamEvent;
      } catch {
        return null;
      }
    })
    .filter((x): x is StreamEvent => x !== null);

  const events: TimelineEvent[] = [];
  let i = 0;
  // Distributive Omit preserves each variant's shape (a plain Omit<Union,"i"> collapses to common keys).
  type NoIndex<T> = T extends unknown ? Omit<T, "i"> : never;
  const push = (e: NoIndex<TimelineEvent>) => events.push({ ...e, i: i++ } as TimelineEvent);

  let model = m.model ?? "sonnet";
  let durationMs = 0;
  let numTurns = 0;
  let costUsd: number | undefined;

  push({ kind: "prompt", text: m.prompt });

  // Track Stop-hook state so we emit one block per distinct verdict and a single
  // pass once the gates clear after a block (the "fixed it" beat).
  let lastBlockReason = "";
  let sawBlock = false;

  for (const ev of stream) {
    if (ev.type === "system" && ev.subtype === "init") {
      if (ev.model) model = ev.model;
      continue;
    }
    if (ev.type === "system" && ev.subtype === "hook_response" && ev.hook_name === "Stop") {
      const hr = parseHookResponse(ev.output ?? ev.stdout);
      if (!hr) continue;
      if (hr.block) {
        if (hr.reason !== lastBlockReason) {
          push({ kind: "hook_block", reason: hr.reason, findings: hr.findings });
          lastBlockReason = hr.reason;
          sawBlock = true;
        }
      } else if (sawBlock) {
        // Gates passed after a block — the resolution. Emit once.
        push({ kind: "hook_pass" });
        sawBlock = false;
        lastBlockReason = "";
      }
      continue;
    }
    if (ev.type === "assistant" && Array.isArray(ev.message?.content)) {
      for (const c of ev.message!.content as Array<Record<string, any>>) {
        if (c.type === "text" && c.text?.trim()) {
          push({ kind: "text", text: c.text.trim() });
        } else if (c.type === "thinking" && c.thinking?.trim()) {
          push({ kind: "thinking", text: c.thinking.trim() });
        } else if (c.type === "tool_use") {
          if (c.name === "Skill") {
            push({ kind: "skill", name: (c.input?.command as string) || "kapi" });
          }
          const meta = toolTitle(c.name, c.input ?? {});
          push({ kind: "tool_use", id: c.id, tool: c.name, ...meta });
        }
      }
    } else if (ev.type === "user" && Array.isArray(ev.message?.content)) {
      for (const c of ev.message!.content as Array<Record<string, any>>) {
        if (c.type === "tool_result") {
          let out = c.content;
          if (Array.isArray(out)) {
            out = out.map((x: any) => (typeof x === "string" ? x : x?.text ?? "")).join("");
          }
          // Find the tool name for cleaning (Read prefix stripping).
          const useEv = events.find(
            (e) => e.kind === "tool_use" && (e as any).id === c.tool_use_id,
          ) as Extract<TimelineEvent, { kind: "tool_use" }> | undefined;
          push({
            kind: "tool_result",
            forId: c.tool_use_id,
            output: cleanOutput(String(out ?? ""), useEv?.tool),
            isError: Boolean(c.is_error),
          });
        }
      }
    } else if (ev.type === "result") {
      durationMs = ev.duration_ms ?? 0;
      numTurns = ev.num_turns ?? 0;
      costUsd = ev.total_cost_usd;
      if (ev.result?.trim()) push({ kind: "result", text: ev.result.trim() });
    }
  }

  return {
    id: m.id,
    title: m.title,
    subtitle: m.subtitle,
    tagline: m.tagline,
    aspects: m.aspects,
    prompt: m.prompt,
    events,
    meta: { model, durationMs, numTurns, costUsd, capturedAt: new Date().toISOString(), errors: detectErrors(events) },
  };
}
