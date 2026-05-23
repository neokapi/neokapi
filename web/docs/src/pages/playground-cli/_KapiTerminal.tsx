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

const HELP = [
  "kapi browser terminal — the kapi CLI compiled to WebAssembly.",
  "",
  "  kapi <command> …   run a kapi command (e.g. kapi formats list)",
  "  <command> …        the leading 'kapi' is optional",
  "",
  "Shell builtins (run in the browser, not kapi):",
  "  ls [dir]   pwd   cd <dir>   cat <file>   rm <file>   clear   help",
  "",
  "Try:",
  "  kapi formats list",
  "  kapi word-count messages.json",
  "  kapi pseudo-translate messages.json -o out.json --target-lang fr",
  "  cat out.json",
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

    setSinks((s) => term.write(s), (s) => term.write(`\x1b[31m${s}\x1b[0m`));

    let line = "";
    const history: string[] = [];
    let histIdx = 0;
    let running = false;

    const prompt = () => term.write(`\r\n\x1b[32m${cli.cwd()}\x1b[0m $ `);

    function resolveDir(arg: string): string {
      if (!arg || arg === ".") return cli.cwd();
      if (arg.startsWith("/")) return arg;
      return cli.cwd().replace(/\/$/, "") + "/" + arg;
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
            term.write(names.map((n) => (cli.vol.isDir(dir.replace(/\/$/, "") + "/" + n) ? `\x1b[34m${n}/\x1b[0m` : n)).join("  "));
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
          try {
            const data = cli.vol.readFile(resolveDir(argv[1]));
            term.write(new TextDecoder().decode(data));
          } catch (e: any) {
            term.write(`cat: ${e.message || e}`);
          }
          return;
        }
        case "rm": {
          try { cli.vol.remove(resolveDir(argv[1])); }
          catch (e: any) { term.write(`rm: ${e.message || e}`); }
          return;
        }
        default: {
          // Pass to the kapi CLI. Strip a leading literal "kapi".
          const cmdArgv = cmd === "kapi" ? argv.slice(1) : argv;
          if (cmdArgv.length === 0) return;
          await cli.run(cmdArgv);
        }
      }
    }

    const onKey = async (data: string) => {
      if (running) return;
      // Handle the common escape sequences (arrows) first.
      if (data === "\x1b[A") { // up
        if (history.length && histIdx > 0) {
          histIdx--;
          term.write("\r\x1b[K" + `\x1b[32m${cli.cwd()}\x1b[0m $ ` + history[histIdx]);
          line = history[histIdx];
        }
        return;
      }
      if (data === "\x1b[B") { // down
        if (histIdx < history.length - 1) {
          histIdx++;
          term.write("\r\x1b[K" + `\x1b[32m${cli.cwd()}\x1b[0m $ ` + history[histIdx]);
          line = history[histIdx];
        } else {
          histIdx = history.length;
          term.write("\r\x1b[K" + `\x1b[32m${cli.cwd()}\x1b[0m $ `);
          line = "";
        }
        return;
      }
      for (const ch of data) {
        const code = ch.charCodeAt(0);
        if (ch === "\r") {
          const cmdLine = line.trim();
          term.write("\r\n");
          line = "";
          if (cmdLine) { history.push(cmdLine); histIdx = history.length; }
          if (cmdLine) {
            running = true;
            try { await execute(cmdLine); }
            catch (e: any) { term.write(`\x1b[31m${e?.message || e}\x1b[0m`); }
            running = false;
            onFsChange();
          }
          prompt();
        } else if (code === 127) { // backspace
          if (line.length > 0) { line = line.slice(0, -1); term.write("\b \b"); }
        } else if (ch === "\x03") { // Ctrl-C
          term.write("^C");
          line = "";
          prompt();
        } else if (code >= 32) {
          line += ch;
          term.write(ch);
        }
      }
    };

    term.writeln("\x1b[1mkapi\x1b[0m browser terminal — the kapi CLI, in WebAssembly. Type \x1b[33mhelp\x1b[0m.");
    prompt();
    const disp = term.onData(onKey);

    const onResize = () => { try { fit.fit(); } catch { /* noop */ } };
    window.addEventListener("resize", onResize);

    return () => {
      window.removeEventListener("resize", onResize);
      disp.dispose();
      term.dispose();
    };
  }, [cli, onFsChange]);

  return <div ref={ref} className={styles.term} />;
}
