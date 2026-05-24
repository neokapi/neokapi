// Shared helpers for the /commands reference page. Kept dependency-free so the
// grid stays cheap to render and the heavy detail/modal can import the same
// derivations.

import type { CommandEntry } from "@neokapi/reference-data";

/** The user-facing command name, e.g. "formats info" for id "formats.info". */
export function commandName(cmd: CommandEntry): string {
  return cmd.path.join(" ");
}

/** A one-line description, preferring the short summary over the long body. */
export function commandSummary(cmd: CommandEntry): string {
  if (cmd.short) return cmd.short;
  // Fall back to the first non-empty line of the long description.
  if (cmd.long) {
    for (const raw of cmd.long.split("\n")) {
      const line = raw.trim();
      if (line) return line;
    }
  }
  return "";
}

/**
 * The runnable command line for a snippet, e.g. "kapi formats list".
 *
 * Prefer the first authored example when present (examples already read as
 * complete invocations); otherwise synthesize from the command path. The
 * cobra `use` of a leaf often carries an argument placeholder (e.g.
 * "info <format>") which would not run, so for the synthesized form we use the
 * dotted path turned back into space-separated segments and append nothing —
 * the reader edits in their own arguments.
 *
 * @deprecated Prefer {@link firstRunnableExample} for the primary snippet;
 * fall back to synthesizing `kapi ${commandName(cmd)}` inline.
 */
export function runnableCommand(cmd: CommandEntry): string {
  const example = cmd.examples?.find((e) => e.trim().startsWith("kapi"));
  if (example) return example.trim();
  return `kapi ${commandName(cmd)}`;
}

/**
 * Returns the first authored example that starts with "kapi", already trimmed,
 * or `null` when `examples` is empty or contains no kapi invocations. Used by
 * CommandDetail as the primary Run snippet (the real, runnable command from the
 * binary's cobra Example string).
 */
export function firstRunnableExample(cmd: CommandEntry): string | null {
  const example = cmd.examples?.find((e) => e.trim().startsWith("kapi"));
  return example != null ? example.trim() : null;
}

// Fixture names provided by the playground kit (packages/kapi-playground
// fixtures.ts). Mirrored here so we can decide whether a synthesized run
// command should seed a sample file without importing the heavy kit on the
// inline path.
const FIXTURE_FILES: Record<string, string> = {
  json: "messages.json",
  xliff: "app.xliff",
  html: "page.html",
  md: "README.md",
  properties: "app.properties",
  xml: "strings.xml",
  xcstrings: "Localizable.xcstrings",
};

/**
 * Pick fixtures to seed for a runnable snippet. If the command already names a
 * file in its example, the seed for that extension is included. For the
 * synthesized "kapi <name>" form we seed a sensible default (messages.json) for
 * the file-processing commands so Run produces output rather than a usage
 * error; pure-management commands (no file argument) need no seed.
 */
export function seedFor(cmd: CommandEntry, command: string): string[] {
  const seeds = new Set<string>();
  for (const [ext, file] of Object.entries(FIXTURE_FILES)) {
    if (command.includes(`.${ext}`) || command.includes(file)) seeds.add(file);
  }
  if (seeds.size === 0 && takesFileArgument(cmd)) {
    seeds.add("messages.json");
  }
  return [...seeds];
}

/**
 * Whether the command's synopsis accepts a file/path argument — used to decide
 * if a synthesized Run should seed a sample file. Heuristic over the cobra
 * `use` string: "[files...]", "<file>", "[file]", "<path>" etc.
 */
function takesFileArgument(cmd: CommandEntry): boolean {
  const use = cmd.use.toLowerCase();
  return /\b(file|files|path|input|src)\b/.test(use) || /\[files?\.{3}\]/.test(use);
}
