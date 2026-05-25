/**
 * Shared data contract for the kapi × Claude Code demo harness.
 *
 * Pipeline:  demo.yaml (authored)
 *              → capture (run real `claude` headless)  → capture.json
 *              → artifacts (Playwright screenshots)     → artifacts/*.png
 *              → narrate (TTS)                          → narration.json + audio/*.wav
 *              → render (Remotion)                      → out/<id>.mp4
 *
 * Everything the renderer needs lives under public/<id>/ so Remotion can read it
 * with staticFile().
 */

/** A single demo as authored under demos/<id>/demo.yaml. */
export interface DemoManifest {
  id: string;
  title: string;
  subtitle: string;
  /** One-line value proposition shown on the title card. */
  tagline?: string;
  /** Which kapi/skill aspects this demo exercises (shown in docs + outro). */
  aspects: string[];
  /** Difficulty / audience tag, e.g. "zero-to-hero", "use-case", "framework". */
  kind: string;
  /**
   * When set, this demo is published into the docs site as theme-matched videos:
   * `<publishAs>-light.webm` / `<publishAs>-dark.webm` under web/docs/static/video/kapi/.
   * Demos without it are previews only (not shipped on the website).
   */
  publishAs?: string;
  /** The task prompt handed to Claude Code. */
  prompt: string;
  /** Whether the demo needs the Gemini AI path (ai-translate / AI brand). */
  needsAi: boolean;
  /**
   * When true, run kapi as an MCP server (`kapi mcp`) instead of loading the skill
   * plugin, and forbid the Bash tool so Claude must drive kapi through MCP tool calls.
   * Demonstrates the MCP integration path.
   */
  mcp?: boolean;
  /** Optional model override for the capture (default: sonnet). */
  model?: string;
  /** Seconds before the headless claude run is killed. */
  captureTimeoutSec?: number;
  /** Extra setup commands run inside the sandbox before claude starts (e.g. seed a termbase). */
  setup?: string[];
  /** Optional extra context appended to the sandbox CLAUDE.md (per-demo environment notes). */
  claudeNote?: string;
  /** Visual artifacts to capture from the sandbox after the run. */
  artifacts: ArtifactSpec[];
  /** Ordered narration scenes that drive the video timeline. */
  narration: NarrationSpec[];
}

/** How to capture one visual artifact after the Claude run finishes. */
export interface ArtifactSpec {
  id: string;
  caption: string;
  /**
   * - "url": screenshot a running URL (a dev server started via `serve`).
   * - "html": screenshot a static HTML file (relative to sandbox).
   * - "report": render a kapi JSON/CSV/.properties output file into an HTML report card.
   * - "command": run a shell/kapi command in the sandbox snapshot and render its stdout
   *   as a report — REAL, deterministic kapi output, not a file Claude happened to save.
   * - "image": an image already present in the sandbox (copied as-is).
   * - "codediff": render `path` from the pristine fixture (before) and the post-run
   *   snapshot (after) side by side, for a before/after source comparison.
   */
  source: "url" | "html" | "report" | "command" | "image" | "docx" | "codediff";
  /** For url: the path to open; for html/report/image: a sandbox-relative file path. */
  path?: string;
  /** For "command": the command to run (kapi on PATH, isolated env). */
  command?: string;
  /**
   * For "command"/"html"/"report": which copy of the project to read.
   * - "snapshot" (default): the final sandbox state after the run (in-place edits applied).
   * - "fixture": the pristine starting files — use for a "before" card when the run edits
   *   the file in place (git-style), so before/after aren't the same post-run file.
   */
  from?: "snapshot" | "fixture";
  /** For "url": a shell command to start a server in the sandbox (background). */
  serve?: string;
  /** For "url": port to wait for. */
  port?: number;
  /** For "url": how long to wait for the server to respond (default 180s — covers a Next build+start). */
  serveTimeoutMs?: number;
  /** For "url": extra ms to wait after load before the screenshot (e.g. for a client locale swap). */
  settleMs?: number;
  /** For "report": the kind of kapi report to render. */
  report?: "brand" | "term-check" | "word-count" | "glossary" | "catalog" | "markdown" | "json" | "code";
  /** For "report" catalog/json: optional title + subtitle on the rendered card. */
  reportTitle?: string;
  reportSub?: string;
  /** Viewport. */
  width?: number;
  height?: number;
}

/** One narration scene. Its audio duration drives how long the scene is on screen. */
export interface NarrationSpec {
  id: string;
  /**
   * - "title": opening title card.
   * - "prompt": full-screen the user's actual request (what they typed).
   * - "terminal": shows the Claude Code session replay (the real transcript).
   * - "artifact": full-screen a captured artifact.
   * - "outro": closing recap card.
   */
  kind: "title" | "prompt" | "terminal" | "artifact" | "outro";
  /** The spoken narration for this scene. */
  text: string;
  /** On-screen caption (defaults to a trimmed version of text for non-title scenes). */
  caption?: string;
  /** For kind="artifact": which ArtifactSpec.id to show. */
  artifact?: string;
  /** Optional minimum seconds (padding) added after the narration audio. */
  holdSec?: number;
}

// ──────────────────────────────────────────────────────────────────────────
// Capture output (written to public/<id>/capture.json)
// ──────────────────────────────────────────────────────────────────────────

export type TimelineEvent =
  | { i: number; kind: "prompt"; text: string }
  | { i: number; kind: "thinking"; text: string }
  | { i: number; kind: "text"; text: string }
  | { i: number; kind: "tool_use"; id: string; tool: string; title: string; command?: string; detail?: string }
  | { i: number; kind: "tool_result"; forId: string; output: string; isError: boolean }
  | { i: number; kind: "skill"; name: string }
  // The kapi verify Stop hook fired and blocked Claude from finishing, with the
  // gate findings to fix. `findings` are the parsed "ERROR/WARNING [gate] …" lines.
  | { i: number; kind: "hook_block"; reason: string; findings: string[] }
  // A later Stop fired and the gates passed — Claude is allowed to finish.
  | { i: number; kind: "hook_pass" }
  | { i: number; kind: "result"; text: string };

export interface DemoCapture {
  id: string;
  title: string;
  subtitle: string;
  tagline?: string;
  aspects: string[];
  prompt: string;
  events: TimelineEvent[];
  meta: {
    model: string;
    durationMs: number;
    numTurns: number;
    costUsd?: number;
    capturedAt: string;
    /** kapi/tool failures detected in this capture (record-time audit). Empty = clean. */
    errors: CaptureError[];
  };
}

/** A failed tool result surfaced to Claude during the session (e.g. a kapi command that errored). */
export interface CaptureError {
  /** The tool that produced it (Bash, mcp__kapi__pseudo_translate, …). */
  tool: string;
  /** The command or tool title that failed. */
  command: string;
  /** First meaningful line(s) of the error. */
  snippet: string;
  /** True if the tool_result was flagged is_error; false if matched by pattern only. */
  hardError: boolean;
}

// ──────────────────────────────────────────────────────────────────────────
// Narration output (written to public/<id>/narration.json)
// ──────────────────────────────────────────────────────────────────────────

export interface NarrationScene {
  id: string;
  kind: NarrationSpec["kind"];
  text: string;
  caption: string;
  artifact?: string;
  /** staticFile-relative path to the audio, e.g. "audio/intro.wav". */
  audio?: string;
  /** Audio duration in seconds (0 for silent scenes). */
  durationSec: number;
  /** Extra hold after audio. */
  holdSec: number;
}

export interface NarrationManifest {
  id: string;
  backend: string;
  voice: string;
  scenes: NarrationScene[];
}

/** Per-artifact metadata written to public/<id>/artifacts.json. */
export interface CapturedArtifact {
  id: string;
  caption: string;
  /** staticFile-relative path, e.g. "artifacts/app-fr.png". */
  image: string;
  width: number;
  height: number;
}

/** Top-level registry written to public/registry.json for the Remotion Root. */
export interface DemoRegistryEntry {
  id: string;
  title: string;
  hasCapture: boolean;
  hasNarration: boolean;
}
