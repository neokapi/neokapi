import { getFixture } from "@neokapi/kapi-playground";
import type { KapiRuntime } from "@neokapi/kapi-playground";

// Seeding helpers for the curated views.
//
// The kit keeps its seeding logic private to <KapiEmbed> (it drives an xterm
// terminal). The curated views don't use a terminal — they boot the runtime
// directly and call preview/run — so they need their own small seeding helpers.
// These mirror the kit's gap-filling behavior: write a file only if it isn't
// already present, so re-seeding never clobbers output a command produced.

const enc = new TextEncoder();
const dec = new TextDecoder();

/** An inline sample (name + UTF-8 content), used when a bundled fixture is not enough. */
export interface InlineSample {
  name: string;
  content: string;
}

/** Resolve `name` relative to the runtime cwd, returning an absolute path. */
export function resolveInCwd(runtime: KapiRuntime, name: string): string {
  if (name.startsWith("/")) return name;
  return runtime.cwd().replace(/\/$/, "") + "/" + name;
}

/**
 * Ensure a sample file exists in the cwd. `sample` is either a bundled fixture
 * name (looked up via getFixture) or an inline {name, content}. Returns the
 * absolute path it lives at. Idempotent: an existing file is left untouched.
 */
export function ensureSample(runtime: KapiRuntime, sample: string | InlineSample): string {
  const resolved: InlineSample | undefined =
    typeof sample === "string" ? getFixture(sample) : sample;
  if (!resolved) {
    throw new Error(
      `Unknown fixture "${typeof sample === "string" ? sample : sample.name}". Pass a bundled fixture name (see fixtureNames) or an inline {name, content}.`,
    );
  }
  const path = resolveInCwd(runtime, resolved.name);
  if (!runtime.vol.exists(path)) {
    const slash = path.lastIndexOf("/");
    if (slash > 0) runtime.vol.mkdirp(path.slice(0, slash));
    runtime.vol.writeFile(path, enc.encode(resolved.content));
  }
  return path;
}

/**
 * Force-write a sample into the cwd, overwriting any existing file. Used for the
 * input pane of a before/after run so a re-run always starts from the pristine
 * source rather than a previously transformed copy.
 */
export function writeSample(runtime: KapiRuntime, sample: string | InlineSample): string {
  const resolved: InlineSample | undefined =
    typeof sample === "string" ? getFixture(sample) : sample;
  if (!resolved) {
    throw new Error(`Unknown fixture "${typeof sample === "string" ? sample : sample.name}".`);
  }
  const path = resolveInCwd(runtime, resolved.name);
  const slash = path.lastIndexOf("/");
  if (slash > 0) runtime.vol.mkdirp(path.slice(0, slash));
  runtime.vol.writeFile(path, enc.encode(resolved.content));
  return path;
}

/** Read a file out of the volume as UTF-8 text. Returns "" if it does not exist. */
export function readText(runtime: KapiRuntime, path: string): string {
  const abs = resolveInCwd(runtime, path);
  if (!runtime.vol.exists(abs)) return "";
  try {
    return dec.decode(runtime.vol.readFile(abs));
  } catch {
    return "";
  }
}

/**
 * Split a command string into an argv array. The leading "kapi" is optional and
 * stripped. Simple whitespace tokenizer with support for single- and
 * double-quoted arguments (so flags like --search "Hello, World!" work).
 */
export function parseCommand(cmd: string): string[] {
  const argv: string[] = [];
  const re = /"([^"]*)"|'([^']*)'|(\S+)/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(cmd)) !== null) {
    argv.push(m[1] ?? m[2] ?? m[3] ?? "");
  }
  if (argv[0] === "kapi") argv.shift();
  return argv;
}
