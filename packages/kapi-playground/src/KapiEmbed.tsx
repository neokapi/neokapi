import React, { useCallback, useEffect, useImperativeHandle, useRef, useState } from "react";
import { Maximize2, Minimize2 } from "lucide-react";
import KapiTerminal from "./KapiTerminal";
import type { KapiTerminalHandle } from "./KapiTerminal";
import FilesPanel from "./FilesPanel";
import { bootKapiRuntime, isBooted } from "./runtime";
import type { KapiRuntime } from "./runtime";
import { getFixture } from "./fixtures";
import type { KapiFile, SessionState } from "./store";

/** Run parameters shared by the initial mount and later imperative opens. */
export interface KapiRunRequest {
  /** Fixture names to ensure exist in the session cwd before the command runs. */
  seed?: string[];
  /** Inline files (dynamic content) to write into the cwd before the command runs. */
  files?: KapiFile[];
  /** A command to type/run once booted and seeded. */
  cmd?: string;
  /** Additional commands to run in sequence after `cmd`. */
  steps?: string[];
  /** Run `cmd`/`steps` automatically (vs. leaving them ready at the prompt). */
  autoRun?: boolean;
}

export interface KapiEmbedHandle {
  /** Seed new fixtures and run a command against the warm session. */
  openWith(req: KapiRunRequest): void;
  /** Reset the session: wipe the cwd, reseed, and clear the terminal. */
  reset(seed?: string[]): void;
  /**
   * Capture the reproducible session state: the files currently in the cwd
   * (as inline files, so reader edits and command outputs are preserved) and
   * the commands run so far. Returns null if the runtime is not ready yet.
   */
  snapshot(): SessionState | null;
}

export interface KapiEmbedProps extends KapiRunRequest {
  /** URL of the Go `wasm_exec.js` glue (must be loadable as a <script>). */
  wasmExecUrl: string;
  /**
   * URL of the kapi-cli wasm. The runtime also probes `${wasmUrl}.gz` and
   * inflates it via DecompressionStream when available.
   */
  wasmUrl: string;
  /** Show the maximize toggle (the modal supplies its own chrome). */
  showToolbar?: boolean;
  /** Render to fill its container (used inside the modal). */
  fill?: boolean;
  ref?: React.Ref<KapiEmbedHandle>;
}

/**
 * Write the named fixtures into the session cwd if they are not already
 * present. Existing files (including ones the reader edited) are preserved —
 * we only fill gaps. Returns the cwd-relative names that now exist.
 */
function ensureSeed(runtime: KapiRuntime, names: string[] | undefined): void {
  if (!names || names.length === 0) return;
  const cwd = runtime.cwd();
  const enc = new TextEncoder();
  for (const name of names) {
    const fx = getFixture(name);
    if (!fx) continue;
    const path = cwd.replace(/\/$/, "") + "/" + fx.name;
    if (!runtime.vol.exists(path)) {
      runtime.vol.writeFile(path, enc.encode(fx.content));
    }
  }
}

// Max files / bytes we will pack into a shareable ?s= token. A demo session is
// a handful of small files; this cap keeps the URL from ballooning if a command
// emitted something large.
const SNAPSHOT_MAX_FILES = 24;
const SNAPSHOT_MAX_BYTES = 256 * 1024;

/**
 * Collect the files under `dir` (recursively, relative to the session cwd) as
 * inline KapiFiles for a shareable snapshot. Binary-ish files that don't decode
 * as UTF-8 are skipped — the demo fixtures and their outputs are all text.
 */
function collectFiles(runtime: KapiRuntime): KapiFile[] {
  const cwd = runtime.cwd().replace(/\/$/, "");
  const dec = new TextDecoder("utf-8", { fatal: true });
  const out: KapiFile[] = [];
  let bytes = 0;

  const walk = (rel: string) => {
    const abs = rel ? cwd + "/" + rel : cwd;
    let names: string[];
    try {
      names = runtime.vol.readdir(abs);
    } catch {
      return;
    }
    for (const name of names) {
      if (out.length >= SNAPSHOT_MAX_FILES) return;
      const childRel = rel ? rel + "/" + name : name;
      const childAbs = cwd + "/" + childRel;
      if (runtime.vol.isDir(childAbs)) {
        walk(childRel);
        continue;
      }
      try {
        const raw = runtime.vol.readFile(childAbs);
        if (bytes + raw.length > SNAPSHOT_MAX_BYTES) continue;
        const content = dec.decode(raw);
        out.push({ path: childRel, content });
        bytes += raw.length;
      } catch {
        /* skip unreadable / non-UTF-8 files */
      }
    }
  };

  walk("");
  return out;
}

/**
 * Write inline files into the session cwd, filling gaps (never clobbering an
 * existing/edited file). `path` may be relative (resolved against cwd) or
 * absolute.
 */
function ensureFiles(runtime: KapiRuntime, files: KapiFile[] | undefined): void {
  if (!files || files.length === 0) return;
  const cwd = runtime.cwd().replace(/\/$/, "");
  const enc = new TextEncoder();
  for (const f of files) {
    const path = f.path.startsWith("/") ? f.path : cwd + "/" + f.path;
    if (!runtime.vol.exists(path)) {
      // Create parent directories for nested paths (e.g. a restored "out/foo")
      // before writing — writeFile requires the parent dir to exist.
      const slash = path.lastIndexOf("/");
      if (slash > 0) runtime.vol.mkdirp(path.slice(0, slash));
      runtime.vol.writeFile(path, enc.encode(f.content));
    }
  }
}

/**
 * The shared two-pane embed: the xterm terminal beside the files panel, backed
 * by the singleton KapiRuntime. This is the heavy, browser-only payload — it
 * is dynamically imported by the modal and rendered directly on the full-bleed
 * playground page.
 */
export default function KapiEmbed({
  wasmExecUrl,
  wasmUrl,
  seed,
  files,
  cmd,
  steps,
  autoRun = true,
  showToolbar = true,
  fill = false,
  ref,
}: KapiEmbedProps): React.ReactElement {
  const [runtime, setRuntime] = useState<KapiRuntime | null>(null);
  const [error, setError] = useState<string>("");
  const [refreshKey, setRefreshKey] = useState(0);
  const [maximized, setMaximized] = useState(false);
  const termRef = useRef<KapiTerminalHandle>(null);
  const bump = useCallback(() => setRefreshKey((k) => k + 1), []);
  // Was the runtime already warm when this embed mounted? If so, skip the
  // "Loading (~13 MB)" copy — there is no fetch on a re-open.
  const wasWarm = useRef(isBooted());

  // Drive a run request against a ready terminal: seed fixtures, then type/run
  // the command(s). For autoRun, submit them with a small stagger so each
  // command's output settles; otherwise leave the first at the prompt.
  const drive = useCallback(
    async (rt: KapiRuntime, req: KapiRunRequest) => {
      const h = termRef.current;
      if (!h) return;
      ensureSeed(rt, req.seed);
      ensureFiles(rt, req.files);
      bump();
      const queue: string[] = [];
      if (req.cmd) queue.push(req.cmd);
      if (req.steps) queue.push(...req.steps);
      if (queue.length === 0) {
        h.focus();
        return;
      }
      if (req.autoRun === false) {
        // Leave the (single) command at the prompt for the reader to run.
        await h.runCommand(queue[0], false);
        return;
      }
      // Run each command in sequence: await so the next one only begins after
      // the previous has finished typing out and executing.
      for (const c of queue) {
        await h.runCommand(c, true);
      }
      h.focus();
    },
    [bump],
  );

  useImperativeHandle(
    ref,
    () => ({
      openWith: (req: KapiRunRequest) => {
        if (runtime) void drive(runtime, req);
      },
      reset: (resetSeed?: string[]) => {
        if (!runtime) return;
        const cwd = runtime.cwd();
        try {
          for (const name of runtime.vol.readdir(cwd)) {
            runtime.vol.remove(cwd.replace(/\/$/, "") + "/" + name);
          }
        } catch {
          /* nothing to clear */
        }
        ensureSeed(runtime, resetSeed);
        bump();
      },
      snapshot: (): SessionState | null => {
        if (!runtime) return null;
        return {
          files: collectFiles(runtime),
          steps: termRef.current?.history() ?? [],
        };
      },
    }),
    [runtime, drive, bump],
  );

  useEffect(() => {
    let cancelled = false;
    bootKapiRuntime(wasmExecUrl, wasmUrl)
      .then((rt) => {
        if (cancelled) return;
        ensureSeed(rt, seed);
        ensureFiles(rt, files);
        setRuntime(rt);
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      });
    return () => {
      cancelled = true;
    };
    // Boot once per mount; the initial seed is applied above.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [wasmExecUrl, wasmUrl]);

  // Once the terminal is mounted and the runtime is ready, drive the initial
  // command (the one supplied at mount time).
  useEffect(() => {
    if (!runtime) return;
    void drive(runtime, { cmd, steps, autoRun, seed: undefined, files: undefined });
    // Run the initial command once per (runtime) — seed already applied above.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runtime]);

  if (error) {
    return (
      <div className="kapi-pg-notice">
        <strong>Could not load the kapi CLI.</strong>
        <p>{error}</p>
        <p>
          The module is built by <code>make web-wasm-cli</code> and served from{" "}
          <code>{wasmUrl}</code>.
        </p>
      </div>
    );
  }

  if (!runtime) {
    return (
      <div className="kapi-pg-loading" role="status" aria-live="polite">
        <span className="kapi-pg-spinner" aria-hidden="true" />
        <span className="kapi-pg-loading-title">Getting things ready…</span>
        <span className="kapi-pg-loading-sub">
          {wasWarm.current
            ? "Starting the kapi CLI…"
            : "Loading the kapi CLI (WebAssembly, ~13 MB)…"}
        </span>
      </div>
    );
  }

  return (
    <div
      className={`kapi-pg-wrapper${maximized ? " kapi-pg-wrapper--maximized" : ""}${fill ? " kapi-pg-wrapper--fill" : ""}`}
    >
      {showToolbar && (
        <div className="kapi-pg-toolbar">
          <span className="kapi-pg-toolbar-title">kapi terminal</span>
          <button
            type="button"
            className="kapi-pg-icon-btn"
            onClick={() => setMaximized((m) => !m)}
            aria-label={maximized ? "Restore size" : "Maximize"}
            title={maximized ? "Restore" : "Maximize"}
          >
            {maximized ? (
              <Minimize2 size={16} aria-hidden="true" />
            ) : (
              <Maximize2 size={16} aria-hidden="true" />
            )}
          </button>
        </div>
      )}
      <div className="kapi-pg-layout">
        <div className="kapi-pg-term-pane">
          <KapiTerminal ref={termRef} runtime={runtime} onFsChange={bump} />
        </div>
        <div className="kapi-pg-files-pane">
          <FilesPanel runtime={runtime} refreshKey={refreshKey} onChange={bump} />
        </div>
      </div>
    </div>
  );
}
