// Boots the kapi CLI WebAssembly module with an in-memory filesystem and
// exposes a small handle the React UI drives. Booted once per page load
// (the wasm module installs a single global kapiRun); the terminal and the
// files panel share the same volume.

import { createMemFS } from "./_memfs";
import type { MemVolume } from "./_memfs";

export interface PreviewBlock {
  id: string;
  text: string;
}

export interface PreviewResult {
  ok: boolean;
  error?: string;
  format?: string;
  blocks?: PreviewBlock[];
  total?: number;
  bytes?: number;
}

export interface KapiCli {
  vol: MemVolume;
  run(argv: string[]): Promise<number>;
  preview(path: string): Promise<PreviewResult>;
  cwd(): string;
  chdir(dir: string): void;
}

// Output sinks are indirected through module-level refs so a remounted
// terminal can re-point them at its fresh xterm instance without rebooting.
let outSink: (s: string) => void = () => {};
let errSink: (s: string) => void = () => {};

export function setSinks(out: (s: string) => void, err: (s: string) => void) {
  outSink = out;
  errSink = err;
}

let booting: Promise<KapiCli> | null = null;

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const existing = document.querySelector(`script[data-kapi-wasm-exec]`);
    if (existing) return resolve();
    const s = document.createElement("script");
    s.src = src;
    s.dataset.kapiWasmExec = "1";
    s.onload = () => resolve();
    s.onerror = () => reject(new Error(`failed to load ${src}`));
    document.head.appendChild(s);
  });
}

export function bootKapiCli(wasmExecUrl: string, wasmUrl: string): Promise<KapiCli> {
  if (booting) return booting;
  booting = (async () => {
    const dec = new TextDecoder();
    const mem = createMemFS({
      onStdout: (c) => outSink(dec.decode(c)),
      onStderr: (c) => errSink(dec.decode(c)),
    });

    const g = globalThis as any;
    // Install our fs/process BEFORE wasm_exec.js runs, so it doesn't install
    // its own enosys defaults. Preserve any existing process.env.
    g.fs = mem.fs;
    const existingProc = g.process || {};
    g.process = Object.assign({}, existingProc, mem.process, { env: existingProc.env || {} });

    await loadScript(wasmExecUrl);
    const Go = g.Go;
    if (!Go) throw new Error("wasm_exec.js did not define Go");

    const go = new Go();
    // stdout isn't a TTY in the browser, so force color for JSON output
    // (--json / --jq) — the terminal renders ANSI just fine.
    go.env = { CLICOLOR_FORCE: "1" };
    const ready = new Promise<void>((res) => { g.__kapiCliReady = res; });

    let instance: WebAssembly.Instance;
    try {
      const r = await WebAssembly.instantiateStreaming(fetch(wasmUrl), go.importObject);
      instance = r.instance;
    } catch {
      const buf = await (await fetch(wasmUrl)).arrayBuffer();
      const r = await WebAssembly.instantiate(buf, go.importObject);
      instance = r.instance;
    }
    void go.run(instance); // blocks forever (select{}); not awaited
    await ready;

    return {
      vol: mem.vol,
      run: (argv: string[]) => g.kapiRun(argv) as Promise<number>,
      preview: (path: string) => g.kapiPreview(path) as Promise<PreviewResult>,
      cwd: () => mem.vol.cwd(),
      chdir: (dir: string) => mem.process.chdir(dir),
    };
  })();
  return booting;
}
