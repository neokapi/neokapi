import React, { useEffect, useRef } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import type { KapiCli } from "./_wasmCli";
import { setSinks } from "./_wasmCli";
import styles from "./styles.module.css";

// Split a command line into argv, honoring single/double quotes. Good enough
// for a demo shell — no escapes, no globbing.
function parseArgv(line: string): string[] {
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
      if (has) { out.push(cur); cur = ""; has = false; }
    } else {
      cur += ch;
      has = true;
    }
  }
  if (has) out.push(cur);
  return out;
}

function longestCommonPrefix(items: string[]): string {
  if (items.length === 0) return "";
  let p = items[0];
  for (const s of items) {
    while (!s.startsWith(p)) p = p.slice(0, -1);
    if (!p) break;
  }
  return p;
}

function join(dir: string, name: string): string {
  return dir.replace(/\/$/, "") + "/" + name;
}

const BUILTINS = ["help", "clear", "ls", "cd", "cat", "pwd", "rm", "kapi"];

const HELP = [
  "kapi browser terminal — the kapi CLI compiled to WebAssembly.",
  "",
  "  kapi <command> …   run a kapi command (e.g. kapi formats list)",
  "  <command> …        the leading 'kapi' is optional",
  "  Tab                complete commands, flags, and filenames",
  "",
  "Shell builtins (run in the browser, not kapi):",
  "  ls [dir]   pwd   cd <dir>   cat <file>   rm <file>   clear   help",
  "",
  "Try:",
  "  kapi formats list",
  "  kapi word-count messages.json",
  "  kapi pseudo-translate messages.json -o out.json --target-lang fr",
  "  kapi jq out.json            # colorized, pretty JSON",
  "  kapi jq '.greeting' out.json",
].join("\n");

export default function KapiTerminal({ cli, onFsChange }: { cli: KapiCli; onFsChange: () => void }) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const term = new Terminal({
      convertEol: true,
      cursorBlink: true,
      fontSize: 13,
      fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
      theme: { background: "#1e1e2e", foreground: "#cdd6f4", cursor: "#cdd6f4" },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(ref.current!);
    try { fit.fit(); } catch { /* layout not ready */ }

    const writeOut = (s: string) => term.write(s);
    const writeErr = (s: string) => term.write(`\x1b[31m${s}\x1b[0m`);
    setSinks(writeOut, writeErr);

    let line = "";
    let cursor = 0; // insertion point within `line`
    const history: string[] = [];
    let histIdx = 0;
    let running = false;
    let topCommands: string[] | null = null; // cached kapi top-level command names

    const promptStr = () => `\x1b[32m${cli.cwd()}\x1b[0m $ `;
    const prompt = () => term.write(`\r\n${promptStr()}`);

    function resolveDir(arg: string): string {
      if (!arg || arg === ".") return cli.cwd();
      if (arg.startsWith("/")) return arg;
      return join(cli.cwd(), arg);
    }

    // Run cobra's hidden __complete with output captured (not echoed to the
    // terminal). Returns the candidate values and the ShellCompDirective.
    async function runComplete(args: string[]): Promise<{ candidates: string[]; directive: number }> {
      let buf = "";
      setSinks((s) => { buf += s; }, () => {});
      try {
        await cli.run(["__complete", ...args]);
      } finally {
        setSinks(writeOut, writeErr);
      }
      const candidates: string[] = [];
      let directive = 0;
      for (const ln of buf.split("\n")) {
        if (ln.startsWith(":")) directive = parseInt(ln.slice(1), 10) || 0;
        else if (ln.length) candidates.push(ln.split("\t")[0]);
      }
      return { candidates, directive };
    }

    async function complete() {
      const rawWord = (line.match(/(\S*)$/) || ["", ""])[1];
      const head = line.slice(0, line.length - rawWord.length);
      const headTokens = parseArgv(head);
      const afterKapi = headTokens[0] === "kapi" ? headTokens.slice(1) : headTokens;
      const completingCommand = afterKapi.length === 0;

      let cands: string[] = [];
      let allowFiles = false;

      if (completingCommand) {
        if (topCommands === null) topCommands = (await runComplete([""])).candidates;
        cands = [...BUILTINS, ...topCommands].filter((c) => c.startsWith(rawWord));
      } else {
        const res = await runComplete([...afterKapi, rawWord]);
        cands = res.candidates.slice();
        allowFiles = (res.directive & 4) === 0; // 4 = ShellCompDirectiveNoFileComp
      }

      if (allowFiles) {
        try {
          for (const name of cli.vol.readdir(cli.cwd())) {
            if (name.startsWith(rawWord)) cands.push(name);
          }
        } catch { /* not a dir */ }
      }

      cands = Array.from(new Set(cands)).sort();
      if (cands.length === 0) { term.write("\x07"); return; }

      if (cands.length === 1) {
        const c = cands[0];
        const isDir = !completingCommand && cli.vol.exists(join(cli.cwd(), c)) && cli.vol.isDir(join(cli.cwd(), c));
        line += c.slice(rawWord.length) + (isDir ? "/" : " ");
        cursor = line.length;
        render();
        return;
      }

      const lcp = longestCommonPrefix(cands);
      if (lcp.length > rawWord.length) {
        line += lcp.slice(rawWord.length);
        cursor = line.length;
        render();
      } else {
        term.write("\r\n" + cands.join("  ") + "\r\n" + promptStr() + line);
        cursor = line.length;
      }
    }

    async function execute(cmdLine: string) {
      const argv = parseArgv(cmdLine);
      if (argv.length === 0) return;
      const cmd = argv[0];

      switch (cmd) {
        case "clear":
          term.clear();
          return;
        case "help":
          term.write(HELP.replace(/\n/g, "\r\n"));
          return;
        case "pwd":
          term.write(cli.cwd());
          return;
        case "ls": {
          try {
            const dir = resolveDir(argv[1] || ".");
            const names = cli.vol.readdir(dir);
            term.write(names.map((n) => (cli.vol.isDir(join(dir, n)) ? `\x1b[34m${n}/\x1b[0m` : n)).join("  "));
          } catch (e: any) {
            term.write(`ls: ${e.message || e}`);
          }
          return;
        }
        case "cd": {
          try { cli.chdir(argv[1] || "/project"); }
          catch (e: any) { term.write(`cd: ${e.message || e}`); }
          return;
        }
        case "cat": {
          try { term.write(new TextDecoder().decode(cli.vol.readFile(resolveDir(argv[1])))); }
          catch (e: any) { term.write(`cat: ${e.message || e}`); }
          return;
        }
        case "rm": {
          try { cli.vol.remove(resolveDir(argv[1])); }
          catch (e: any) { term.write(`rm: ${e.message || e}`); }
          return;
        }
        default: {
          // Pass to the kapi CLI. Strip a leading literal "kapi"; a bare
          // "kapi" (empty rest) runs the root command, which prints help.
          await cli.run(cmd === "kapi" ? argv.slice(1) : argv);
        }
      }
    }

    // Redraw the current input line and place the terminal cursor at `cursor`.
    // Assumes the prompt + line fit on one row (true for typical commands).
    function render() {
      term.write("\r\x1b[K" + promptStr() + line);
      const back = line.length - cursor;
      if (back > 0) term.write(`\x1b[${back}D`);
    }

    function setLine(next: string) {
      line = next;
      cursor = next.length;
      render();
    }

    const onKey = async (data: string) => {
      if (running) return;

      // Multi-byte sequences: arrows, Home/End, Delete, Tab.
      switch (data) {
        case "\x1b[D": // left
          if (cursor > 0) { cursor--; term.write("\x1b[D"); }
          return;
        case "\x1b[C": // right
          if (cursor < line.length) { cursor++; term.write("\x1b[C"); }
          return;
        case "\x1b[A": // up — history back
          if (history.length && histIdx > 0) { histIdx--; setLine(history[histIdx]); }
          return;
        case "\x1b[B": // down — history forward
          if (histIdx < history.length - 1) { histIdx++; setLine(history[histIdx]); }
          else { histIdx = history.length; setLine(""); }
          return;
        case "\x1b[H": case "\x1bOH": case "\x1b[1~": // home
          cursor = 0; render(); return;
        case "\x1b[F": case "\x1bOF": case "\x1b[4~": // end
          cursor = line.length; render(); return;
        case "\x1b[3~": // delete (forward)
          if (cursor < line.length) { line = line.slice(0, cursor) + line.slice(cursor + 1); render(); }
          return;
        case "\t":
          running = true;
          try { await complete(); } catch { /* ignore */ }
          running = false;
          return;
      }

      // Swallow any other escape sequence so it doesn't echo as literal text.
      if (data.charCodeAt(0) === 27) return;

      for (const ch of data) {
        const code = ch.charCodeAt(0);
        if (ch === "\r" || ch === "\n") {
          const cmdLine = line.trim();
          term.write("\r\n");
          line = ""; cursor = 0;
          if (cmdLine) { history.push(cmdLine); histIdx = history.length; }
          if (cmdLine) {
            running = true;
            try { await execute(cmdLine); }
            catch (e: any) { term.write(`\x1b[31m${e?.message || e}\x1b[0m`); }
            running = false;
            onFsChange();
          }
          prompt();
        } else if (code === 127 || code === 8) { // backspace
          if (cursor > 0) {
            line = line.slice(0, cursor - 1) + line.slice(cursor);
            cursor--;
            render();
          }
        } else if (ch === "\x03") { // Ctrl-C
          term.write("^C");
          line = ""; cursor = 0;
          prompt();
        } else if (ch === "\x01") { // Ctrl-A — start of line
          cursor = 0; render();
        } else if (ch === "\x05") { // Ctrl-E — end of line
          cursor = line.length; render();
        } else if (ch === "\x15") { // Ctrl-U — clear line
          line = ""; cursor = 0; render();
        } else if (code >= 32) {
          line = line.slice(0, cursor) + ch + line.slice(cursor);
          cursor++;
          if (cursor === line.length) term.write(ch); // fast path: append at end
          else render();
        }
      }
    };

    term.writeln("\x1b[1mkapi\x1b[0m browser terminal — the kapi CLI, in WebAssembly. Type \x1b[33mhelp\x1b[0m, or \x1b[33mTab\x1b[0m to complete.");
    prompt();
    const disp = term.onData(onKey);

    // Refit whenever the container resizes — covers window resize, the files
    // panel reflow, and the maximize/fullscreen toggle.
    const ro = new ResizeObserver(() => { try { fit.fit(); } catch { /* noop */ } });
    if (ref.current) ro.observe(ref.current);

    return () => {
      ro.disconnect();
      disp.dispose();
      term.dispose();
    };
  }, [cli, onFsChange]);

  return <div ref={ref} className={styles.term} />;
}
