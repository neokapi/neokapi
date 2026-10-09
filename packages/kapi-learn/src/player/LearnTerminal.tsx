import React, { useEffect, useRef } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import type { TerminalHandle } from "./types.ts";

// The lab's screen: an xterm with a prompt. The session runs what is typed,
// whether by a chapter (typeAndRun, letter by letter) or by the reader. The
// terminal keeps the line editing, the history and the typing animation; it
// knows nothing about kapi.

export interface LearnTerminalProps {
  /** Run a line; resolves its exit code once the output has been written. */
  onSubmit: (line: string) => Promise<number>;
  /** Receives the handle once the terminal is open, and null when it closes. */
  onReady: (handle: TerminalHandle | null) => void;
  /** The prompt's label, read before each prompt. */
  promptLabel: () => string;
  /** Called when the reader runs a command of their own. */
  onUserCommand?: () => void;
  className?: string;
}

const sleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

export default function LearnTerminal({
  onSubmit,
  onReady,
  promptLabel,
  onUserCommand,
  className,
}: LearnTerminalProps): React.ReactElement {
  const el = useRef<HTMLDivElement>(null);
  const submitRef = useRef(onSubmit);
  const labelRef = useRef(promptLabel);
  const userRef = useRef(onUserCommand);
  submitRef.current = onSubmit;
  labelRef.current = promptLabel;
  userRef.current = onUserCommand;

  useEffect(() => {
    const host = el.current;
    if (!host) return;
    const term = new Terminal({
      convertEol: true,
      cursorBlink: true,
      fontSize: 13,
      lineHeight: 1.25,
      fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
      scrollback: 4000,
      theme: {
        background: "#14151c",
        foreground: "#d8dbe6",
        cursor: "#f5d67b",
        selectionBackground: "#3b3f52",
        black: "#1f2230",
        red: "#ff7a90",
        green: "#8fd6a0",
        yellow: "#f5d67b",
        blue: "#8fb7ff",
        magenta: "#d7a6ff",
        cyan: "#8fe3e6",
        white: "#e6e8f0",
        brightBlack: "#6b7089",
      },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(host);
    const refit = () => {
      try {
        fit.fit();
      } catch {
        /* not laid out yet */
      }
    };
    refit();

    let line = "";
    let cursor = 0;
    const history: string[] = [];
    let histIdx = 0;
    let running = false;

    const promptStr = () => `\x1b[36m${labelRef.current()}\x1b[0m \x1b[2m$\x1b[0m `;
    const prompt = () => term.write(`\r\n${promptStr()}`);
    const render = () => {
      term.write("\r\x1b[K" + promptStr() + line);
      const back = line.length - cursor;
      if (back > 0) term.write(`\x1b[${back}D`);
    };
    const setLine = (next: string) => {
      line = next;
      cursor = next.length;
      render();
    };

    async function submit(cmdLine: string): Promise<number> {
      const trimmed = cmdLine.trim();
      if (!trimmed) {
        prompt();
        return 0;
      }
      history.push(trimmed);
      histIdx = history.length;
      running = true;
      let code = 0;
      try {
        code = await submitRef.current(trimmed);
      } catch (e) {
        term.write(`\x1b[31m${e instanceof Error ? e.message : String(e)}\x1b[0m`);
        code = 1;
      }
      running = false;
      prompt();
      return code;
    }

    const onData = async (data: string) => {
      if (running) return;
      switch (data) {
        case "\x1b[D":
          if (cursor > 0) {
            cursor--;
            term.write("\x1b[D");
          }
          return;
        case "\x1b[C":
          if (cursor < line.length) {
            cursor++;
            term.write("\x1b[C");
          }
          return;
        case "\x1b[A":
          if (history.length && histIdx > 0) {
            histIdx--;
            setLine(history[histIdx]);
          }
          return;
        case "\x1b[B":
          if (histIdx < history.length - 1) {
            histIdx++;
            setLine(history[histIdx]);
          } else {
            histIdx = history.length;
            setLine("");
          }
          return;
        case "\x1b[H":
        case "\x1bOH":
        case "\x1b[1~":
          cursor = 0;
          render();
          return;
        case "\x1b[F":
        case "\x1bOF":
        case "\x1b[4~":
          cursor = line.length;
          render();
          return;
        case "\x1b[3~":
          if (cursor < line.length) {
            line = line.slice(0, cursor) + line.slice(cursor + 1);
            render();
          }
          return;
        case "\t":
          return;
      }
      if (data.charCodeAt(0) === 27) return;
      for (const ch of data) {
        const code = ch.charCodeAt(0);
        if (ch === "\r" || ch === "\n") {
          const cmdLine = line;
          term.write("\r\n");
          line = "";
          cursor = 0;
          if (cmdLine.trim()) userRef.current?.();
          await submit(cmdLine);
        } else if (code === 127 || code === 8) {
          if (cursor > 0) {
            line = line.slice(0, cursor - 1) + line.slice(cursor);
            cursor--;
            render();
          }
        } else if (ch === "\x03") {
          term.write("^C");
          line = "";
          cursor = 0;
          prompt();
        } else if (ch === "\x01") {
          cursor = 0;
          render();
        } else if (ch === "\x05") {
          cursor = line.length;
          render();
        } else if (ch === "\x15") {
          line = "";
          cursor = 0;
          render();
        } else if (ch === "\x0c") {
          term.clear();
          render();
        } else if (code >= 32) {
          line = line.slice(0, cursor) + ch + line.slice(cursor);
          cursor++;
          if (cursor === line.length) term.write(ch);
          else render();
        }
      }
    };

    term.writeln(
      "\x1b[1mkapi\x1b[0m runs here, in your browser. The chapters type for you; type your own commands any time (\x1b[33mhelp\x1b[0m lists what this terminal knows).",
    );
    prompt();
    const disposer = term.onData((d) => void onData(d));

    const handle: TerminalHandle = {
      typeAndRun: async (cmd: string, animate: boolean): Promise<number> => {
        if (running) return 1;
        if (!cmd.trim()) return 0;
        if (line) {
          line = "";
          cursor = 0;
          render();
        }
        running = true;
        const reduce = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
        if (!animate || reduce) {
          term.write(cmd);
        } else {
          for (const ch of cmd) {
            term.write(ch);
            await sleep(18 + Math.random() * 34);
          }
          await sleep(220);
        }
        term.write("\r\n");
        running = false;
        return submit(cmd);
      },
      write: (text) => term.write(text),
      clear: () => term.clear(),
      focus: () => term.focus(),
    };
    onReady(handle);

    const ro = new ResizeObserver(refit);
    ro.observe(host);

    return () => {
      onReady(null);
      ro.disconnect();
      disposer.dispose();
      term.dispose();
    };
    // The terminal is created once; the callbacks are read through refs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return <div ref={el} className={className} />;
}
