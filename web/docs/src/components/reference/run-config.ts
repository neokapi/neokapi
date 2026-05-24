// Helpers for the "Run this config" action in ReferenceDetail.
//
// Strategy:
//   - **Formats (built-in)**: `kapi word-count --format <id> <fixture>` —
//     proves the format reader works and produces real segment output. Only
//     offered when a real fixture matches one of the format's extensions.
//   - **Tools (built-in, direct command, default params)**: `kapi <tool-id>
//     <fixture>` — the simplest invocation.
//   - **Tools (built-in, no direct command, or with non-default params)**:
//     write a minimal `project.kapi` (single-step flow using the tool's current
//     YAML config) into the session via `files`, then `kapi run my-flow -p
//     project.kapi -i <fixture>`.
//   - **Okapi-source entries**: not offline-capable (need the Java bridge);
//     the button is hidden.
//
// The form values are passed in so the generated project file carries the
// user's live configuration, not just the defaults.

import type { ReferenceEntry, ComponentSchema } from "@neokapi/reference-data";
import type { OpenKapiOptions } from "@neokapi/kapi-playground";
import { buildToolYamlLines } from "./yaml";

// ── Fixture affinity ───────────────────────────────────────────────────────

/**
 * Extension → fixture name, ordered most-specific first. Used to pick the
 * best sample file for a given format or tool.
 */
const EXT_TO_FIXTURE: Record<string, string> = {
  ".xliff": "app.xliff",
  ".xlf": "app.xliff",
  ".xliff2": "app.xliff",
  ".html": "page.html",
  ".htm": "page.html",
  ".xhtml": "page.html",
  ".md": "README.md",
  ".markdown": "README.md",
  ".properties": "app.properties",
  ".xml": "strings.xml",
  ".xcstrings": "Localizable.xcstrings",
  ".json": "messages.json",
};

/** The simplest, most broadly-accepted sample for format-agnostic tools. */
const DEFAULT_TOOL_FIXTURE = "messages.json";

/**
 * Choose the best fixture name for an entry.
 *
 * For formats: iterate the entry's declared extensions and return the first
 * match. Returns `null` when no fixture genuinely matches (e.g. binary or
 * unsupported formats like mif/idml/epub/openxml) — the caller hides the Run
 * action so it never errors on an incompatible sample.
 *
 * For tools: use the entry's `inputs` hints, otherwise fall back to
 * `messages.json` since most tools are format-agnostic.
 */
export function pickFixture(entry: ReferenceEntry): string | null {
  if (entry.kind === "format") {
    for (const ext of entry.extensions ?? []) {
      const fix = EXT_TO_FIXTURE[ext.toLowerCase()];
      if (fix) return fix;
    }
    // No compatible fixture for this format — do not offer Run.
    return null;
  }

  // For tools, check if the tool's inputs hint at a format preference.
  const inputs = entry.inputs ?? [];
  for (const input of inputs) {
    const lower = input.toLowerCase();
    if (lower.includes("xliff")) return "app.xliff";
    if (lower.includes("html")) return "page.html";
    if (lower.includes("markdown") || lower.includes(".md")) return "README.md";
    if (lower.includes("properties")) return "app.properties";
    if (lower.includes("xml")) return "strings.xml";
    if (lower.includes("xcstrings")) return "Localizable.xcstrings";
  }
  // Most tools are format-agnostic; JSON is the simplest sample.
  return DEFAULT_TOOL_FIXTURE;
}

// ── Offline capability ─────────────────────────────────────────────────────

/**
 * Tool IDs (and aliases) that have a dedicated offline-capable CLI command.
 * Derived from commands.json `offlineCapable: true` entries that correspond
 * to tool operations (excludes management/admin commands).
 */
const TOOL_COMMANDS = new Set([
  "case-transform",
  "char-count",
  "chars-check",
  "chars-listing",
  "diff-leverage",
  "encoding-detect",
  "inconsistency-check",
  "length-check",
  "pattern-check",
  "pseudo-translate",
  "qa-check",
  "redact",
  "repetition-analysis",
  "scoping-report",
  "script",
  "search-replace",
  "segment-count",
  "segmentation",
  "term-check",
  "tm-leverage",
  "translation-comparison",
  "unredact",
  "word-count",
]);

/**
 * Returns true when the entry can be executed offline in the WASM playground.
 *
 * - Okapi-source entries need the Java bridge subprocess: not offline capable.
 * - AI-powered tools need network credentials: not offline capable.
 * - Formats with no compatible fixture (binary/unsupported): the Run action
 *   would error, so it is hidden.
 * - Everything else: built-in, offline-capable.
 */
export function isRunnable(entry: ReferenceEntry): boolean {
  if (entry.source === "okapi") return false;
  // AI-powered tools require credentials / network
  const tags = entry.tags ?? [];
  if (tags.includes("ai-powered")) return false;
  // Tools that explicitly require credentials or a running service
  const requires = entry.requires ?? [];
  if (requires.includes("credentials")) return false;
  // Formats with no genuine sample to run against would only error.
  if (entry.kind === "format" && pickFixture(entry) === null) return false;
  return true;
}

// ── Project file generation ────────────────────────────────────────────────

/**
 * Whether a tool has its own top-level CLI command (vs. needing `kapi run`).
 */
function hasDirectCommand(entry: ReferenceEntry): boolean {
  if (TOOL_COMMANDS.has(entry.id)) return true;
  for (const alias of entry.aliases ?? []) {
    if (TOOL_COMMANDS.has(alias)) return true;
  }
  return false;
}

/**
 * Render current form values as a YAML `config:` block for a flow step,
 * indented at two extra spaces (under `config:`). Returns "" when there are
 * no non-default values.
 */
function configBlock(values: Record<string, unknown>, schema: ComponentSchema | undefined): string {
  const lines = buildToolYamlLines(values, schema);
  // buildToolYamlLines returns `[{ text: "# (default configuration)" }]` for
  // all-default values — treat that as no config.
  if (lines.length === 1 && lines[0].text.startsWith("#")) return "";
  // Indent each line by 4 spaces (under `config:`, which itself is under the step)
  return lines.map((l) => `    ${l.text}`).join("\n");
}

/**
 * Generate a minimal `.kapi` project file that declares a single-step flow
 * applying `entry` with the current form values. Returned as a string to be
 * seeded into the session.
 */
function buildProjectFile(
  entry: ReferenceEntry,
  values: Record<string, unknown>,
  schema: ComponentSchema | undefined,
): string {
  const block = configBlock(values, schema);
  const configSection = block ? `\n      config:\n${block}` : "";
  return [
    "version: v1",
    `name: ref-${entry.id}`,
    "flows:",
    `  my-flow:`,
    "    steps:",
    `      - tool: ${entry.id}${configSection}`,
  ].join("\n");
}

// ── Main API ───────────────────────────────────────────────────────────────

/**
 * Build the `openKapi(...)` options for the "Run this config" action.
 *
 * Returns `null` when the entry is not runnable in the playground (the caller
 * should hide the button or show a hint instead).
 */
export function buildRunOptions(
  entry: ReferenceEntry,
  values: Record<string, unknown>,
  schema: ComponentSchema | undefined,
): OpenKapiOptions | null {
  if (!isRunnable(entry)) return null;

  const fixture = pickFixture(entry);
  if (fixture === null) return null; // no compatible sample (defensive)
  const seed = [fixture];

  // ── Format entries ──────────────────────────────────────────────────────
  if (entry.kind === "format") {
    // Show format parsing by running word-count with the format forced.
    const cmd = `kapi word-count --format ${entry.id} ${fixture}`;
    return { cmd, seed, autoRun: true };
  }

  // ── Tool entries ────────────────────────────────────────────────────────

  // Does the user have non-default params? If so, or if the tool has no direct
  // command, use a project file so the config is applied faithfully.
  const yamlLines = buildToolYamlLines(values, schema);
  const hasConfig = yamlLines.length > 0 && !yamlLines[0].text.startsWith("#");

  if (!hasConfig && hasDirectCommand(entry)) {
    // Simple case: defaults + direct command available → invoke the tool
    // command directly with the fixture as a positional arg.
    const toolId = TOOL_COMMANDS.has(entry.id)
      ? entry.id
      : (entry.aliases ?? []).find((a) => TOOL_COMMANDS.has(a))!;
    const cmd = `kapi ${toolId} ${fixture}`;
    return { cmd, seed, autoRun: true };
  }

  // Otherwise write a single-step flow project carrying the form's YAML config
  // and run it. The project file goes in via `files` (the playground shell has
  // no echo/printf/redirection); the fixture goes in via `seed`.
  const projectFile = "project.kapi";
  const files = [{ path: projectFile, content: buildProjectFile(entry, values, schema) }];
  const cmd = `kapi run my-flow -p ${projectFile} -i ${fixture}`;
  return { cmd, seed, files, autoRun: true };
}

/**
 * Return a reason string when the entry is not runnable, for display in a
 * tooltip or inline hint. Returns null when it is runnable.
 */
export function notRunnableReason(entry: ReferenceEntry): string | null {
  if (entry.source === "okapi") {
    return "Requires the Okapi bridge — not available in the browser playground.";
  }
  const tags = entry.tags ?? [];
  if (tags.includes("ai-powered")) {
    return "Requires AI credentials — run locally with your API key.";
  }
  const requires = entry.requires ?? [];
  if (requires.includes("credentials")) {
    return "Requires credentials — run locally with your API key.";
  }
  return null;
}

