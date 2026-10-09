// The lab shell's line reader.
//
// A learning lab runs the same lines a reader would type in a real shell, and
// the published sample journeys (samples/*/README.md, harness/demos) lean on a
// few shell habits: `echo '{...}' > decisions.jsonl`, `kapi check --json >
// findings.json 2>/dev/null`, `kapi apply dutch.json 2>&1 | tail -3`. The
// browser has no shell, so this module reads exactly that much: words with
// single or double quotes, `|` pipes, and the `>`, `>>`, `2>`, `2>&1` and `<`
// redirections. Nothing else: no escapes, no variables, no globbing, no `&&`.
//
// Deliberately a leaf module (no imports) so the Node verifier can load it
// directly with --experimental-strip-types, as the docs verifier loads argv.ts.

/** One command of a pipeline, with the redirections that apply to it. */
export interface Segment {
  argv: string[];
  /** Where standard output goes when not piped: a file, else the terminal. */
  stdout?: { path: string; append: boolean };
  /** Where standard error goes: a file, or wherever standard output goes. */
  stderr?: { path: string; append: boolean } | "stdout";
  /** A file to feed as standard input. */
  stdin?: string;
}

export interface ParsedLine {
  segments: Segment[];
}

interface Word {
  text: string;
  quoted: boolean;
}

type Piece =
  | Word
  | { op: "|" }
  | { op: ">"; fd: 1 | 2; append: boolean }
  | { op: "<" }
  | { op: "2>&1" };

function isWord(p: Piece): p is Word {
  return (p as Word).text !== undefined;
}

/**
 * Split a line into words and operators. Quotes group and are removed; an
 * operator is recognized only outside quotes. `2>` is a redirection of
 * standard error when the `2` stands alone before the `>`.
 */
export function lexLine(line: string): Piece[] | { error: string } {
  const pieces: Piece[] = [];
  let cur = "";
  let quoted = false;
  let has = false;
  let quote: string | null = null;

  const flush = () => {
    if (has) pieces.push({ text: cur, quoted });
    cur = "";
    has = false;
    quoted = false;
  };

  for (let i = 0; i < line.length; i++) {
    const ch = line[i];
    if (quote) {
      if (ch === quote) quote = null;
      else cur += ch;
      continue;
    }
    if (ch === '"' || ch === "'") {
      quote = ch;
      quoted = true;
      has = true;
      continue;
    }
    if (ch === " " || ch === "\t") {
      flush();
      continue;
    }
    if (ch === "|") {
      flush();
      pieces.push({ op: "|" });
      continue;
    }
    if (ch === "<") {
      flush();
      pieces.push({ op: "<" });
      continue;
    }
    if (ch === ">") {
      // `2>` and `2>&1`: the fd is the word being built, when it is only "2".
      let fd: 1 | 2 = 1;
      if (has && !quoted && cur === "2") {
        fd = 2;
        cur = "";
        has = false;
      } else {
        flush();
      }
      if (line[i + 1] === "&" && line[i + 2] === "1" && fd === 2) {
        pieces.push({ op: "2>&1" });
        i += 2;
        continue;
      }
      const append = line[i + 1] === ">";
      if (append) i++;
      pieces.push({ op: ">", fd, append });
      continue;
    }
    cur += ch;
    has = true;
  }
  if (quote) return { error: `unterminated ${quote === '"' ? "double" : "single"} quote` };
  flush();
  return pieces;
}

/**
 * Read a line into its pipeline segments. A redirection takes the word after
 * it as its file; `2>&1` sends standard error where standard output goes.
 */
export function parseLine(line: string): ParsedLine | { error: string } {
  const lexed = lexLine(line);
  if (!Array.isArray(lexed)) return lexed;
  const segments: Segment[] = [];
  let seg: Segment = { argv: [] };
  for (let i = 0; i < lexed.length; i++) {
    const p = lexed[i];
    if (isWord(p)) {
      seg.argv.push(p.text);
      continue;
    }
    switch (p.op) {
      case "|": {
        if (seg.argv.length === 0) return { error: "syntax error near `|`" };
        segments.push(seg);
        seg = { argv: [] };
        break;
      }
      case "2>&1": {
        seg.stderr = "stdout";
        break;
      }
      case ">":
      case "<": {
        const target = lexed[i + 1];
        if (!target || !isWord(target)) {
          return {
            error: `syntax error: \`${p.op === "<" ? "<" : p.fd === 2 ? "2>" : ">"}\` needs a file`,
          };
        }
        i++;
        if (p.op === "<") seg.stdin = target.text;
        else if (p.fd === 2) seg.stderr = { path: target.text, append: p.append };
        else seg.stdout = { path: target.text, append: p.append };
        break;
      }
    }
  }
  if (seg.argv.length === 0 && segments.length > 0) return { error: "syntax error near `|`" };
  if (seg.argv.length > 0) segments.push(seg);
  return { segments };
}

/** The words of a line with no operators, as a terminal's completion needs them. */
export function words(line: string): string[] {
  const lexed = lexLine(line);
  if (!Array.isArray(lexed)) return [];
  return lexed.filter(isWord).map((w) => w.text);
}
