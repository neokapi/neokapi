// The lab session: one engine, one sandbox, the chapters run in order.
//
// The session owns the state a lab moves through: the engine boot, the
// sandbox seeded from the sample, which chapter is in view and how many have
// run, the transcript, and what the files pane shows. The terminal is a
// screen the session writes on; chapters type into it and the reader types
// into it, and both paths run the same lab shell.
//
// Chapters run in order, and only in order: `executed` is how many have run,
// and the chapter in view (`current`) is one of them. Play runs the first
// chapter; Next runs the one after the last that ran, or moves the view
// forward when an earlier chapter is in view; Previous moves the view back
// and runs nothing. A jump ahead from the list replays the chapters in
// between at once, without typing them out. Nothing moves on a timer.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { configurePlugins, bootEngine } from "@neokapi/kapi-playground/plugins";
import { onBootProgress } from "@neokapi/kapi-playground/runtime";
import type { BootProgress, InspectResult } from "@neokapi/kapi-playground/runtime";
import type { Chapter, ChapterLook, Lab, LabFile } from "../curriculum/types.ts";
import { SAMPLE_TREES } from "../samples.gen.ts";
import { runLine as shellRun, type ShellHost, type ShellSinks } from "../shell/index.ts";
import { diffAgainstSeed, formatLabLink, type LabLink } from "../deeplink.ts";
import { markChapter, markPosition } from "../progress.ts";
import { changedPaths, fingerprintTree, listFiles } from "./fileTree.ts";
import type {
  FileEntry,
  KapiRuntime,
  LabAssets,
  SessionStatus,
  TerminalHandle,
  TranscriptEntry,
} from "./types.ts";

export interface UseLabSessionOptions {
  lab: Lab;
  assets: LabAssets;
  /** The deep link the page was opened with. */
  link?: LabLink;
  /** Called when the chapter in view changes, so the host can keep the URL current. */
  onChapterChange?: (chapter: Chapter, index: number) => void;
}

/** What a quiet run returns: the exit code and everything it printed. */
export interface CaptureResult {
  code: number;
  out: string;
}

export interface LabSession {
  status: SessionStatus;
  error: string | null;
  bootProgress: BootProgress | null;
  /** The chapter in view. */
  current: number;
  /** How many chapters have run, from the first. */
  executed: number;
  /** A chapter or a typed command is running. */
  busy: boolean;
  /** The exit code of the last command, or null. */
  lastExit: number | null;
  transcript: TranscriptEntry[];
  files: FileEntry[];
  /** Files the last command wrote or changed. */
  lastChanged: ReadonlySet<string>;
  /** Bumps whenever the sandbox was re-read, so a pane re-reads what it shows. */
  version: number;
  /** Write files the reader picked into the sandbox root. */
  addFiles: (files: { name: string; bytes: Uint8Array }[]) => void;
  showHidden: boolean;
  setShowHidden: (v: boolean) => void;
  selectedFile: string | null;
  /** The reading the selected file opens in, when a chapter chose one. */
  selectedView?: ChapterLook["view"];
  selectFile: (path: string | null, view?: ChapterLook["view"]) => void;
  /** A session restored from a shared link. */
  restored: boolean;

  /** Boot the engine, seed the sandbox and run the first chapter. Idempotent. */
  start: () => void;
  /** Run the next chapter, or view the next one when an earlier chapter is in view. */
  next: () => Promise<void>;
  /** View the previous chapter; nothing runs. */
  previous: () => void;
  /** View a chapter; chapters before it that have not run are replayed at once. */
  goTo: (index: number) => Promise<void>;
  /** Run a line the reader typed. */
  runLine: (line: string) => Promise<number>;
  /** Type a line at the prompt and run it, as a chapter would. */
  typeLine: (line: string) => Promise<number>;
  /** Run a line with nothing on the terminal, and return what it printed. */
  runCapture: (line: string) => Promise<CaptureResult>;
  /** Start the lab over: a fresh sandbox, an empty transcript, the first chapter run again. */
  reset: () => Promise<void>;
  /** A link to this lab at the chapter in view, with the reader's files when asked. */
  shareLink: (includeFiles: boolean) => { url: string; tooLarge: boolean; bytes: number };
  readFile: (path: string) => Uint8Array | null;
  inspect: (path: string) => Promise<InspectResult>;
  registerTerminal: (handle: TerminalHandle | null) => void;
  /** The prompt's label: the sandbox-relative working directory. */
  promptLabel: () => string;
}

const enc = new TextEncoder();

/**
 * Every lab's sandbox sits under one directory, and Play starts the whole
 * directory over rather than the lab's own. The engine keys a project by its
 * recipe's name, so the labs that run in one sample are one project with one
 * context wherever they are seeded, and resetting only this lab's directory
 * would leave the context another lab built (its decisions, its memory) in
 * force here. The workspace also outlives the page, so the reset reaches a
 * project a previous visit left behind.
 */
const LEARN_ROOT = "/learn";

const DIM = (s: string) => `\x1b[2m${s}\x1b[0m`;
const RED = (s: string) => `\x1b[31m${s}\x1b[0m`;
// kapi reports on standard error as well as failing there, so its stderr is
// set apart from stdout without reading as an error: a warm, lighter tone.
const STDERR = (s: string) => `\x1b[38;5;223m${s}\x1b[0m`;

export function useLabSession({
  lab,
  assets,
  link,
  onChapterChange,
}: UseLabSessionOptions): LabSession {
  const [status, setStatus] = useState<SessionStatus>("idle");
  const [error, setError] = useState<string | null>(null);
  const [bootProgress, setBootProgress] = useState<BootProgress | null>(null);
  const [current, setCurrent] = useState(0);
  const [executed, setExecuted] = useState(0);
  const [busy, setBusy] = useState(false);
  const [lastExit, setLastExit] = useState<number | null>(null);
  const [transcript, setTranscript] = useState<TranscriptEntry[]>([]);
  const [files, setFiles] = useState<FileEntry[]>([]);
  const [lastChanged, setLastChanged] = useState<ReadonlySet<string>>(new Set());
  const [version, setVersion] = useState(0);
  const [showHidden, setShowHidden] = useState(false);
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [selectedView, setSelectedView] = useState<ChapterLook["view"] | undefined>(undefined);
  const [restored, setRestored] = useState(false);

  const runtimeRef = useRef<KapiRuntime | null>(null);
  const terminalRef = useRef<TerminalHandle | null>(null);
  const seedFingerprints = useRef<Map<string, string>>(new Map());
  const lastFingerprints = useRef<Map<string, string>>(new Map());
  const executedRef = useRef(0);
  const currentRef = useRef(0);
  const busyRef = useRef(false);
  const startedRef = useRef(false);
  const chapterRunningRef = useRef<string | undefined>(undefined);
  const showHiddenRef = useRef(false);
  // Set once the player is gone. The engine outlives the page's React tree,
  // so a run of chapters that was under way when the reader left the lab
  // would otherwise keep driving it, into the sandbox the next lab reseeds.
  const disposedRef = useRef(false);
  useEffect(() => {
    disposedRef.current = false;
    return () => {
      disposedRef.current = true;
    };
  }, []);

  const sandbox = `${LEARN_ROOT}/${lab.id}`;

  const seedFiles = useMemo<LabFile[]>(
    () => [
      ...((SAMPLE_TREES as Record<string, readonly LabFile[]>)[lab.sample] ?? []),
      ...(lab.files ?? []),
    ],
    [lab],
  );

  // ── Files ────────────────────────────────────────────────────────────────

  const refreshFiles = useCallback(
    (markLast: boolean) => {
      const rt = runtimeRef.current;
      if (!rt) return;
      const now = fingerprintTree(rt.vol, sandbox);
      if (markLast) setLastChanged(changedPaths(lastFingerprints.current, now));
      lastFingerprints.current = now;
      const listed = listFiles(rt.vol, sandbox, showHiddenRef.current);
      setFiles(
        listed.map((f) => ({
          path: f.path,
          size: f.size,
          changed: seedFingerprints.current.get(f.path) !== now.get(f.path),
        })),
      );
      setVersion((v) => v + 1);
    },
    [sandbox],
  );

  const addFiles = useCallback(
    (picked: { name: string; bytes: Uint8Array }[]) => {
      const rt = runtimeRef.current;
      if (!rt) return;
      for (const f of picked) {
        const name = f.name.replace(/[\\/]/g, "_");
        if (!name || name === "." || name === "..") continue;
        rt.vol.writeFile(`${sandbox}/${name}`, f.bytes);
        terminalRef.current?.write(DIM(`# added ${name}`) + "\r\n");
      }
      refreshFiles(true);
      if (picked.length === 1) setSelectedFile(picked[0].name.replace(/[\\/]/g, "_"));
    },
    [refreshFiles, sandbox],
  );

  const setShowHiddenAndRefresh = useCallback(
    (v: boolean) => {
      showHiddenRef.current = v;
      setShowHidden(v);
      refreshFiles(false);
    },
    [refreshFiles],
  );

  const writeFile = useCallback(
    (rel: string, content: string) => {
      const rt = runtimeRef.current;
      if (!rt) return;
      const abs = `${sandbox}/${rel}`;
      const slash = abs.lastIndexOf("/");
      if (slash > 0) rt.vol.mkdirp(abs.slice(0, slash));
      rt.vol.writeFile(abs, enc.encode(content));
    },
    [sandbox],
  );

  // ── The shell over the engine ────────────────────────────────────────────

  const hostFor = useCallback(
    (rt: KapiRuntime): ShellHost => ({
      runKapi: async (argv, sinks: ShellSinks) => {
        rt.setSinks(sinks.out, sinks.err);
        try {
          return await rt.run(argv);
        } finally {
          rt.setSinks(
            () => {},
            () => {},
          );
        }
      },
      readFile: (p) => rt.vol.readFile(p),
      writeFile: (p, d) => rt.vol.writeFile(p, d),
      exists: (p) => rt.vol.exists(p),
      isDir: (p) => rt.vol.isDir(p),
      readdir: (p) => rt.vol.readdir(p),
      mkdirp: (p) => rt.vol.mkdirp(p),
      remove: (p) => rt.vol.remove(p),
      cwd: () => rt.cwd(),
      chdir: (d) => rt.chdir(d),
      clear: () => terminalRef.current?.clear(),
    }),
    [],
  );

  /** Run a line with its output on the terminal. The transcript records it. */
  const runLine = useCallback(
    async (line: string): Promise<number> => {
      const rt = runtimeRef.current;
      const term = terminalRef.current;
      if (!rt) return 1;
      const sinks: ShellSinks = {
        out: (s) => term?.write(s.replace(/\r?\n/g, "\r\n")),
        err: (s) => term?.write(STDERR(s.replace(/\r?\n/g, "\r\n"))),
      };
      const entry: TranscriptEntry = {
        chapterId: chapterRunningRef.current,
        command: line,
        code: null,
        at: Date.now(),
      };
      setTranscript((t) => [...t, entry]);
      busyRef.current = true;
      setBusy(true);
      setStatus("running");
      let code = 1;
      try {
        code = await shellRun(hostFor(rt), line, sinks);
      } finally {
        busyRef.current = false;
        setBusy(false);
        setStatus("ready");
      }
      if (code !== 0) term?.write(DIM(`[exit ${code}]`) + "\r\n");
      setLastExit(code);
      setTranscript((t) => t.map((e) => (e === entry ? { ...e, code } : e)));
      refreshFiles(true);
      return code;
    },
    [hostFor, refreshFiles],
  );

  /** Run a line quietly: nothing on the terminal, nothing in the transcript. */
  const runCapture = useCallback(
    async (line: string): Promise<CaptureResult> => {
      const rt = runtimeRef.current;
      if (!rt) return { code: 1, out: "" };
      let out = "";
      const sinks: ShellSinks = {
        out: (s) => {
          out += s;
        },
        err: (s) => {
          out += s;
        },
      };
      const code = await shellRun(hostFor(rt), line, sinks);
      return { code, out };
    },
    [hostFor],
  );

  const typeLine = useCallback(async (line: string): Promise<number> => {
    const term = terminalRef.current;
    if (!term || busyRef.current) return 1;
    term.focus();
    return term.typeAndRun(line, true);
  }, []);

  // ── Chapters ─────────────────────────────────────────────────────────────

  const view = useCallback(
    (index: number) => {
      if (lab.chapters.length === 0) return;
      const i = Math.max(0, Math.min(lab.chapters.length - 1, index));
      currentRef.current = i;
      setCurrent(i);
      const ch = lab.chapters[i];
      markPosition(lab.id, ch.id);
      onChapterChange?.(ch, i);
      if (ch.look?.file) {
        setSelectedFile(ch.look.file);
        setSelectedView(ch.look.view);
      }
    },
    [lab, onChapterChange],
  );

  /** Run chapter `i`, which must be the next unrun one. */
  const runChapter = useCallback(
    async (i: number, animate: boolean) => {
      const term = terminalRef.current;
      const ch = lab.chapters[i];
      if (!ch || i !== executedRef.current || disposedRef.current) return;
      chapterRunningRef.current = ch.id;
      try {
        for (const f of ch.files ?? []) {
          writeFile(f.path, f.content);
          term?.write(DIM(`# wrote ${f.path}`) + "\r\n");
        }
        if (ch.files?.length) refreshFiles(true);
        if (ch.command && term) {
          await term.typeAndRun(ch.command, animate);
        }
      } finally {
        chapterRunningRef.current = undefined;
      }
      executedRef.current = i + 1;
      setExecuted(i + 1);
      markChapter(lab.id, ch.id);
      if (ch.look?.file) {
        setSelectedFile(ch.look.file);
        setSelectedView(ch.look.view);
      }
    },
    [lab, refreshFiles, writeFile],
  );

  const next = useCallback(async () => {
    if (busyRef.current) return;
    const i = currentRef.current;
    const n = lab.chapters.length;
    if (i + 1 < executedRef.current) {
      // An earlier chapter is in view: move on without running anything.
      view(i + 1);
      return;
    }
    if (executedRef.current < n) {
      const target = executedRef.current;
      await runChapter(target, true);
      view(target);
    }
  }, [lab, runChapter, view]);

  const previous = useCallback(() => {
    if (currentRef.current > 0) view(currentRef.current - 1);
  }, [view]);

  const goTo = useCallback(
    async (index: number) => {
      if (busyRef.current) return;
      const i = Math.max(0, Math.min(lab.chapters.length - 1, index));
      // Replay the chapters up to this one at once, and type the last one.
      while (executedRef.current <= i && executedRef.current < lab.chapters.length) {
        await runChapter(executedRef.current, executedRef.current === i);
      }
      view(i);
    },
    [lab, runChapter, view],
  );

  // ── Boot and seed ────────────────────────────────────────────────────────

  const seed = useCallback(
    async (rt: KapiRuntime) => {
      try {
        await rt.reset(LEARN_ROOT);
      } catch (e) {
        // A failed reset leaves another lab's context in force, which the
        // reader would take for this lab's. Say so where a developer looks.
        console.warn(`learn: could not start ${LEARN_ROOT} over:`, e);
      }
      rt.vol.mkdirp(sandbox);
      for (const f of seedFiles) {
        const abs = `${sandbox}/${f.path}`;
        const slash = abs.lastIndexOf("/");
        if (slash > 0) rt.vol.mkdirp(abs.slice(0, slash));
        rt.vol.writeFile(abs, enc.encode(f.content));
      }
      rt.chdir(sandbox);
      seedFingerprints.current = fingerprintTree(rt.vol, sandbox);
      lastFingerprints.current = seedFingerprints.current;
    },
    [sandbox, seedFiles],
  );

  const prepare = useCallback(
    async (rt: KapiRuntime, term: TerminalHandle | null) => {
      const host = hostFor(rt);
      const quiet: ShellSinks = { out: () => {}, err: () => {} };
      for (const line of lab.setup ?? []) {
        const code = await shellRun(host, line, quiet);
        if (code !== 0) term?.write(RED(`setup: ${line} exited ${code}`) + "\r\n");
      }
      if (lab.setup?.length) {
        term?.write(DIM(`# prepared: ${lab.setup.join(" · ")}`) + "\r\n");
      }
    },
    [hostFor, lab],
  );

  const start = useCallback(() => {
    if (startedRef.current) return;
    startedRef.current = true;
    setStatus("booting");
    setError(null);
    const off = onBootProgress((p) => setBootProgress(p.done ? null : p));
    configurePlugins(assets);
    void (async () => {
      try {
        const rt = (await bootEngine()) as KapiRuntime;
        runtimeRef.current = rt;
        setStatus("seeding");
        await seed(rt);
        const term = terminalRef.current;
        const wanted = link?.chapter ? lab.chapters.findIndex((c) => c.id === link.chapter) : -1;
        if (link?.session) {
          for (const f of link.session.files) {
            const abs = `${sandbox}/${f.path}`;
            const slash = abs.lastIndexOf("/");
            if (slash > 0) rt.vol.mkdirp(abs.slice(0, slash));
            rt.vol.writeFile(abs, enc.encode(f.content));
          }
          for (const p of link.session.removed) {
            try {
              rt.vol.remove(`${sandbox}/${p}`);
            } catch {
              /* already gone */
            }
          }
          setRestored(true);
        } else {
          await prepare(rt, term);
        }
        lastFingerprints.current = fingerprintTree(rt.vol, sandbox);
        refreshFiles(false);
        setStatus("ready");
        if (link?.session) {
          // The shared files are the state after the chapters before this
          // one; the linked chapter itself runs on top of them.
          const at = Math.max(0, wanted);
          executedRef.current = at;
          setExecuted(at);
          term?.write(DIM("# restored from a shared link") + "\r\n");
          await runChapter(at, true);
          view(at);
        } else if (lab.chapters.length > 0) {
          // Play was pressed: run up to the linked chapter at once and type
          // that one out, or run the first chapter.
          const upto = Math.max(0, wanted);
          while (
            !disposedRef.current &&
            executedRef.current <= upto &&
            executedRef.current < lab.chapters.length
          ) {
            await runChapter(executedRef.current, executedRef.current === upto);
          }
          if (!disposedRef.current) view(upto);
        }
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
        setStatus("error");
        startedRef.current = false;
      } finally {
        off();
        setBootProgress(null);
      }
    })();
  }, [assets, lab, link, prepare, refreshFiles, runChapter, sandbox, seed, view]);

  const reset = useCallback(async () => {
    const rt = runtimeRef.current;
    if (!rt || busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    try {
      await seed(rt);
      terminalRef.current?.clear();
      await prepare(rt, terminalRef.current);
      executedRef.current = 0;
      setExecuted(0);
      setTranscript([]);
      setLastExit(null);
      setRestored(false);
      lastFingerprints.current = fingerprintTree(rt.vol, sandbox);
      refreshFiles(false);
      setLastChanged(new Set());
      setSelectedFile(null);
    } finally {
      busyRef.current = false;
      setBusy(false);
    }
    if (lab.chapters.length > 0) {
      await runChapter(0, true);
      view(0);
    }
  }, [lab, prepare, refreshFiles, runChapter, sandbox, seed, view]);

  // ── Share, read, inspect ─────────────────────────────────────────────────

  const shareLink = useCallback(
    (includeFiles: boolean) => {
      const path = typeof window !== "undefined" ? window.location.pathname : `/learn/${lab.id}`;
      const origin = typeof window !== "undefined" ? window.location.origin : "";
      const ch = lab.chapters[currentRef.current];
      const rt = runtimeRef.current;
      let session;
      let bytes = 0;
      if (includeFiles && rt) {
        const current = listFiles(rt.vol, sandbox, true).map((f) => ({
          path: f.path,
          bytes: rt.vol.readFile(`${sandbox}/${f.path}`),
        }));
        session = diffAgainstSeed(seedFiles, current);
        bytes = session.files.reduce((n, f) => n + enc.encode(f.content).length, 0);
      }
      const url = formatLabLink(path, { chapter: ch?.id, session });
      const tooLarge = !!session && !url.includes("&s=") && !url.includes("?s=");
      return { url: origin + url, tooLarge, bytes };
    },
    [lab, sandbox, seedFiles],
  );

  const readFile = useCallback(
    (path: string): Uint8Array | null => {
      const rt = runtimeRef.current;
      if (!rt) return null;
      try {
        return rt.vol.readFile(`${sandbox}/${path}`);
      } catch {
        return null;
      }
    },
    [sandbox],
  );

  const inspect = useCallback(
    async (path: string): Promise<InspectResult> => {
      const rt = runtimeRef.current;
      if (!rt) return { ok: false, error: "the engine is not running" };
      return rt.inspect(`${sandbox}/${path}`);
    },
    [sandbox],
  );

  const registerTerminal = useCallback((handle: TerminalHandle | null) => {
    terminalRef.current = handle;
  }, []);

  const promptLabel = useCallback(() => {
    const rt = runtimeRef.current;
    if (!rt) return lab.sample;
    const cwd = rt.cwd();
    const rel =
      cwd === sandbox ? "" : cwd.startsWith(sandbox + "/") ? cwd.slice(sandbox.length) : cwd;
    return `${lab.sample}${rel}`;
  }, [lab.sample, sandbox]);

  return {
    status,
    error,
    bootProgress,
    current,
    executed,
    busy,
    lastExit,
    transcript,
    files,
    lastChanged,
    version,
    addFiles,
    showHidden,
    setShowHidden: setShowHiddenAndRefresh,
    selectedFile,
    selectedView,
    selectFile: (path, view) => {
      setSelectedFile(path);
      setSelectedView(view);
    },
    restored,
    start,
    next,
    previous,
    goTo,
    runLine,
    typeLine,
    runCapture,
    reset,
    shareLink,
    readFile,
    inspect,
    registerTerminal,
    promptLabel,
  };
}
