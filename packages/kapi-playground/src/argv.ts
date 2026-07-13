// The one command-line tokenizer.
//
// The browser terminal, the docs snippet embeds and the CI snippet verifier all
// have to agree on what `kapi translate --instruction "Keep it informal"` means.
// When they didn't, the verifier split on whitespace and handed kapi a bare
// `informal"` as a positional argument — so a documented command failed in CI
// that worked perfectly in the browser the docs were verifying.
//
// Deliberately a leaf module: no imports, no framework. The verifier is a plain
// node script (`--experimental-strip-types`) and loads this file directly, which
// only stays possible while it depends on nothing.
//
// Shell-ish, not a shell: single and double quotes group, and that is all. No
// escapes, no variable expansion, no globbing.

/** Split a command line into argv, honoring single- and double-quoted arguments. */
export function parseArgv(line: string): string[] {
  const out: string[] = [];
  let cur = "";
  let quote: string | null = null;
  let has = false;

  for (const ch of line) {
    if (quote) {
      if (ch === quote) quote = null;
      else cur += ch;
    } else if (ch === '"' || ch === "'") {
      quote = ch;
      has = true;
    } else if (ch === " " || ch === "\t") {
      if (has) {
        out.push(cur);
        cur = "";
        has = false;
      }
    } else {
      cur += ch;
      has = true;
    }
  }
  if (has) out.push(cur);

  return out;
}

/**
 * parseArgv, with a leading `kapi` stripped. The docs write commands as the user
 * would type them (`kapi stats file.json`), but the wasm entry point takes argv
 * without the binary name.
 */
export function parseCommand(cmd: string): string[] {
  const argv = parseArgv(cmd);
  if (argv[0] === "kapi") argv.shift();
  return argv;
}
