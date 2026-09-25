/** Commands displayed by the terminal's built-in help and exercised by the WASM smoke test. */
export const CLI_EXAMPLES = [
  "kapi formats",
  "kapi stats messages.json",
  "kapi pseudo-translate messages.json -o out.json",
  "kapi stats messages.json --json",
  "kapi stats messages.json --jq '.total.words'",
] as const;

export const TERMINAL_HELP = [
  "kapi processes content in your browser.",
  "",
  "  kapi <command> …   run a kapi command (e.g. kapi formats)",
  "  <command> …        the leading 'kapi' is optional",
  "  Tab                complete commands, flags, and filenames",
  "",
  "Shell builtins (handled by this terminal):",
  "  ls [dir]   pwd   cd <dir>   cat <file>   rm <file>   clear   help",
  "",
  "Try:",
  ...CLI_EXAMPLES.map((command) => `  ${command}`),
].join("\n");
