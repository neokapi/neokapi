// The lab shell: runs one line against the engine's file system.
//
// The same module drives the player's terminal in the browser and the
// verifier in Node (scripts/learn-verify), so a chapter the verifier proves
// green runs the same way for a reader. It knows a handful of builtins (the
// ones the sample journeys use between kapi commands), pipes between them,
// and hands every other verb to kapi.
//
// Leaf module: it imports only the line reader beside it.

import { parseLine, type Segment } from "./tokenize.ts";

export interface ShellSinks {
  out: (text: string) => void;
  err: (text: string) => void;
}

/** What the shell needs from its host: one kapi runner and a file system. */
export interface ShellHost {
  /**
   * Run one kapi invocation (argv without the leading `kapi`), its output
   * through the sinks. Resolves the exit code.
   */
  runKapi(argv: string[], sinks: ShellSinks): Promise<number>;
  readFile(path: string): Uint8Array;
  writeFile(path: string, data: Uint8Array): void;
  exists(path: string): boolean;
  isDir(path: string): boolean;
  readdir(path: string): string[];
  mkdirp(path: string): void;
  remove(path: string): void;
  cwd(): string;
  chdir(dir: string): void;
  /** Clear the terminal; a host without a screen ignores it. */
  clear?(): void;
}

export const BUILTINS = [
  "cat",
  "cd",
  "clear",
  "echo",
  "grep",
  "head",
  "help",
  "ls",
  "mkdir",
  "printf",
  "pwd",
  "rm",
  "tail",
  "wc",
] as const;

export const SHELL_HELP = `This terminal runs kapi in your browser, on files that stay in this tab.

  kapi <command>        every browser-safe kapi command (try: kapi --help)
  ksed, kgrep, kcat, kconv, kdiff   the format-aware text tools
  ls, cat, cd, pwd, echo, printf, mkdir, rm, head, tail, grep, wc
  >, >>, 2>, 2>&1, |    redirections and pipes between those
  clear                 clear the screen

Files written here are yours to download from the files pane.
`;

const enc = new TextEncoder();
const dec = new TextDecoder();

/** Resolve a path against the host's working directory, folding `.` and `..`. */
export function resolvePath(cwd: string, p: string): string {
  const abs = p.startsWith("/") ? p : cwd.replace(/\/$/, "") + "/" + p;
  const out: string[] = [];
  for (const part of abs.split("/")) {
    if (part === "" || part === ".") continue;
    if (part === "..") out.pop();
    else out.push(part);
  }
  return "/" + out.join("/");
}

function readText(host: ShellHost, path: string): string {
  return dec.decode(host.readFile(path));
}

interface Stream {
  text: string;
}

type BuiltinResult = Promise<number> | number;

type Builtin = (
  host: ShellHost,
  argv: string[],
  stdin: Stream | null,
  out: (s: string) => void,
  err: (s: string) => void,
) => BuiltinResult;

function lines(text: string): string[] {
  const all = text.split("\n");
  if (all.length && all[all.length - 1] === "") all.pop();
  return all;
}

// Read the files named in argv, or standard input when none is named.
function inputText(
  host: ShellHost,
  files: string[],
  stdin: Stream | null,
  err: (s: string) => void,
  verb: string,
): string | null {
  if (files.length === 0) return stdin?.text ?? "";
  let text = "";
  for (const f of files) {
    const abs = resolvePath(host.cwd(), f);
    if (!host.exists(abs) || host.isDir(abs)) {
      err(`${verb}: ${f}: No such file\n`);
      return null;
    }
    text += readText(host, abs);
  }
  return text;
}

// Parse `-n N` / `-N` (head, tail) and return the count plus the files.
function countAndFiles(argv: string[], fallback: number): { n: number; files: string[] } {
  let n = fallback;
  const files: string[] = [];
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "-n" && i + 1 < argv.length) {
      n = parseInt(argv[++i], 10);
    } else if (/^-\d+$/.test(a)) {
      n = parseInt(a.slice(1), 10);
    } else if (/^-n\d+$/.test(a)) {
      n = parseInt(a.slice(2), 10);
    } else {
      files.push(a);
    }
  }
  if (!Number.isFinite(n) || n < 0) n = fallback;
  return { n, files };
}

const builtins: Record<(typeof BUILTINS)[number], Builtin> = {
  clear: (host) => {
    host.clear?.();
    return 0;
  },
  help: (_host, _argv, _stdin, out) => {
    out(SHELL_HELP);
    return 0;
  },
  pwd: (host, _argv, _stdin, out) => {
    out(host.cwd() + "\n");
    return 0;
  },
  cd: (host, argv, _stdin, _out, err) => {
    const target = resolvePath(host.cwd(), argv[0] ?? "/");
    if (!host.exists(target) || !host.isDir(target)) {
      err(`cd: ${argv[0] ?? target}: No such directory\n`);
      return 1;
    }
    host.chdir(target);
    return 0;
  },
  ls: (host, argv, _stdin, out, err) => {
    const flags = argv.filter((a) => a.startsWith("-"));
    const long = flags.some((f) => f.includes("l"));
    const targets = argv.filter((a) => !a.startsWith("-"));
    const dirs = targets.length ? targets : ["."];
    let code = 0;
    for (const d of dirs) {
      const abs = resolvePath(host.cwd(), d);
      if (!host.exists(abs)) {
        err(`ls: ${d}: No such file or directory\n`);
        code = 1;
        continue;
      }
      if (!host.isDir(abs)) {
        out(d + "\n");
        continue;
      }
      if (dirs.length > 1) out(`${d}:\n`);
      const names = host.readdir(abs).slice().sort();
      if (long) {
        for (const n of names) {
          const child = abs.replace(/\/$/, "") + "/" + n;
          const isDir = host.isDir(child);
          const size = isDir ? 0 : host.readFile(child).length;
          out(`${isDir ? "d" : "-"}  ${String(size).padStart(8)}  ${n}${isDir ? "/" : ""}\n`);
        }
      } else {
        out(
          names
            .map((n) => (host.isDir(abs.replace(/\/$/, "") + "/" + n) ? `${n}/` : n))
            .join("  ") + (names.length ? "\n" : ""),
        );
      }
    }
    return code;
  },
  cat: (host, argv, stdin, out, err) => {
    const text = inputText(host, argv, stdin, err, "cat");
    if (text === null) return 1;
    out(text);
    return 0;
  },
  echo: (_host, argv, _stdin, out) => {
    let noNewline = false;
    let args = argv;
    if (args[0] === "-n") {
      noNewline = true;
      args = args.slice(1);
    }
    out(args.join(" ") + (noNewline ? "" : "\n"));
    return 0;
  },
  printf: (_host, argv, _stdin, out, err) => {
    if (argv.length === 0) {
      err("printf: usage: printf FORMAT [ARG...]\n");
      return 2;
    }
    const format = argv[0].replace(/\\n/g, "\n").replace(/\\t/g, "\t").replace(/\\\\/g, "\\");
    const args = argv.slice(1);
    let i = 0;
    // The format is reused until every argument is consumed, as printf does.
    let result = "";
    do {
      result += format.replace(/%[sd%]/g, (m) => {
        if (m === "%%") return "%";
        return args[i++] ?? "";
      });
    } while (i < args.length && /%[sd]/.test(format));
    out(result);
    return 0;
  },
  head: (host, argv, stdin, out, err) => {
    const { n, files } = countAndFiles(argv, 10);
    const text = inputText(host, files, stdin, err, "head");
    if (text === null) return 1;
    const ls = lines(text).slice(0, n);
    out(ls.length ? ls.join("\n") + "\n" : "");
    return 0;
  },
  tail: (host, argv, stdin, out, err) => {
    const { n, files } = countAndFiles(argv, 10);
    const text = inputText(host, files, stdin, err, "tail");
    if (text === null) return 1;
    const all = lines(text);
    const ls = n === 0 ? [] : all.slice(-n);
    out(ls.length ? ls.join("\n") + "\n" : "");
    return 0;
  },
  grep: (host, argv, stdin, out, err) => {
    let ignoreCase = false;
    let invert = false;
    let count = false;
    let numbers = false;
    let after = 0;
    let pattern: string | null = null;
    const files: string[] = [];
    for (let i = 0; i < argv.length; i++) {
      const a = argv[i];
      if (pattern === null && a.startsWith("-") && a.length > 1) {
        if (a === "-A" && i + 1 < argv.length) {
          after = parseInt(argv[++i], 10) || 0;
          continue;
        }
        const attached = /^-A(\d+)$/.exec(a);
        if (attached) {
          after = parseInt(attached[1], 10);
          continue;
        }
        if (a === "-e" && i + 1 < argv.length) {
          pattern = argv[++i];
          continue;
        }
        for (const f of a.slice(1)) {
          if (f === "i") ignoreCase = true;
          else if (f === "v") invert = true;
          else if (f === "c") count = true;
          else if (f === "n") numbers = true;
          else if (f === "E" || f === "F") {
            /* regex is the default; literal patterns are regex-safe enough here */
          } else {
            err(`grep: unknown option -${f}\n`);
            return 2;
          }
        }
        continue;
      }
      if (pattern === null) pattern = a;
      else files.push(a);
    }
    if (pattern === null) {
      err("grep: usage: grep [-ivcn] [-A N] PATTERN [FILE...]\n");
      return 2;
    }
    let re: RegExp;
    try {
      re = new RegExp(pattern, ignoreCase ? "i" : "");
    } catch (e) {
      err(`grep: ${e instanceof Error ? e.message : String(e)}\n`);
      return 2;
    }
    const text = inputText(host, files, stdin, err, "grep");
    if (text === null) return 1;
    const all = lines(text);
    const hits: string[] = [];
    let matched = 0;
    let pending = 0;
    all.forEach((ln, idx) => {
      const hit = re.test(ln) !== invert;
      if (hit) {
        matched++;
        pending = after;
        hits.push(numbers ? `${idx + 1}:${ln}` : ln);
      } else if (pending > 0) {
        pending--;
        hits.push(numbers ? `${idx + 1}-${ln}` : ln);
      }
    });
    if (count) out(`${matched}\n`);
    else out(hits.length ? hits.join("\n") + "\n" : "");
    return matched > 0 ? 0 : 1;
  },
  wc: (host, argv, stdin, out, err) => {
    const flags = argv.filter((a) => a.startsWith("-")).join("");
    const files = argv.filter((a) => !a.startsWith("-"));
    const text = inputText(host, files, stdin, err, "wc");
    if (text === null) return 1;
    const l = lines(text).length;
    const w = text.split(/\s+/).filter(Boolean).length;
    const c = enc.encode(text).length;
    const parts: string[] = [];
    if (flags.includes("l")) parts.push(String(l));
    if (flags.includes("w")) parts.push(String(w));
    if (flags.includes("c")) parts.push(String(c));
    if (parts.length === 0) parts.push(String(l), String(w), String(c));
    out(
      parts.map((p) => p.padStart(8)).join("") + (files.length === 1 ? ` ${files[0]}` : "") + "\n",
    );
    return 0;
  },
  mkdir: (host, argv, _stdin, _out, err) => {
    const dirs = argv.filter((a) => a !== "-p");
    if (dirs.length === 0) {
      err("mkdir: usage: mkdir [-p] DIR...\n");
      return 2;
    }
    for (const d of dirs) host.mkdirp(resolvePath(host.cwd(), d));
    return 0;
  },
  rm: (host, argv, _stdin, _out, err) => {
    const targets = argv.filter((a) => !a.startsWith("-"));
    let code = 0;
    for (const t of targets) {
      const abs = resolvePath(host.cwd(), t);
      if (!host.exists(abs)) {
        err(`rm: ${t}: No such file or directory\n`);
        code = 1;
        continue;
      }
      host.remove(abs);
    }
    return code;
  },
};

function isBuiltin(name: string): name is (typeof BUILTINS)[number] {
  return (BUILTINS as readonly string[]).includes(name);
}

/**
 * Run one segment with its standard input, collecting its output through the
 * given writers. Everything that is not a builtin is a kapi verb: the toolbox
 * utilities (`ksed`, `kgrep`, …) are subcommands of the one binary, so a bare
 * `ksed` and `kapi ksed` run the same code.
 */
async function runSegment(
  host: ShellHost,
  seg: Segment,
  stdin: Stream | null,
  out: (s: string) => void,
  err: (s: string) => void,
): Promise<number> {
  const [verb, ...rest] = seg.argv;
  if (isBuiltin(verb)) return builtins[verb](host, rest, stdin, out, err);
  if (stdin && stdin.text.length > 0) {
    err("this terminal cannot pipe into kapi; write the input to a file and name it instead\n");
    return 1;
  }
  const argv = verb === "kapi" ? rest : seg.argv;
  return host.runKapi(argv, { out, err });
}

/**
 * Run a line. Output reaches the sinks as it is produced, except what a pipe
 * or a redirection takes. Resolves the last segment's exit code; a line the
 * shell cannot read reports the reason on standard error and resolves 2.
 */
export async function runLine(host: ShellHost, line: string, sinks: ShellSinks): Promise<number> {
  const parsed = parseLine(line);
  if ("error" in parsed) {
    sinks.err(`shell: ${parsed.error}\n`);
    return 2;
  }
  const { segments } = parsed;
  if (segments.length === 0) return 0;

  let stdin: Stream | null = null;
  let code = 0;
  for (let i = 0; i < segments.length; i++) {
    const seg = segments[i];
    const last = i === segments.length - 1;
    const captured: Stream = { text: "" };
    const files = new Map<string, { chunks: string[]; append: boolean }>();

    const fileWriter = (path: string, append: boolean) => {
      const abs = resolvePath(host.cwd(), path);
      if (abs === "/dev/null") return () => {};
      let entry = files.get(abs);
      if (!entry) {
        entry = { chunks: [], append };
        files.set(abs, entry);
      }
      const e = entry;
      return (s: string) => {
        e.chunks.push(s);
      };
    };

    let out: (s: string) => void;
    if (!last) out = (s) => (captured.text += s);
    else if (seg.stdout) out = fileWriter(seg.stdout.path, seg.stdout.append);
    else out = sinks.out;

    let err: (s: string) => void;
    if (seg.stderr === "stdout") err = out;
    else if (seg.stderr) err = fileWriter(seg.stderr.path, seg.stderr.append);
    else err = sinks.err;

    let input = stdin;
    if (seg.stdin) {
      const abs = resolvePath(host.cwd(), seg.stdin);
      if (!host.exists(abs) || host.isDir(abs)) {
        sinks.err(`shell: ${seg.stdin}: No such file\n`);
        return 1;
      }
      input = { text: readText(host, abs) };
    }

    try {
      code = await runSegment(host, seg, input, out, err);
    } catch (e) {
      sinks.err(`${e instanceof Error ? e.message : String(e)}\n`);
      code = 1;
    }

    // Redirected streams land in their files once the segment is done, so a
    // command that reads the file it writes sees the previous contents.
    for (const [abs, entry] of files) {
      const slash = abs.lastIndexOf("/");
      if (slash > 0) host.mkdirp(abs.slice(0, slash));
      const fresh = entry.chunks.join("");
      const previous = entry.append && host.exists(abs) ? readText(host, abs) : "";
      host.writeFile(abs, enc.encode(previous + fresh));
    }
    stdin = last ? null : captured;
  }
  return code;
}
