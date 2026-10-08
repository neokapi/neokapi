// The lab session: one engine, one sandbox, the chapters run in order.
//
// The session owns the state a lab moves through: the engine boot, the
// sandbox seeded from the sample, which chapter is in view and how many have
// run, the transcript, and what the files pane shows. The terminal is a
// screen the session writes on; chapters type into it and the reader types
// into it, and both paths run the same lab shell.
//
// Chapters run in order, and only in order: `executed` is how many have run,
// and the chapter in view (`current`) may be anywhere. Running a chapter
// requires every earlier one to have run, so a jump ahead replays the
// chapters in between at once, without typing them out.

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

export interface LabSession {
  status: SessionStatus;
  error: string | null;
  bootProgress: BootProgress | null;
  /** The chapter in view. */
  current: number;
  /** How many chapters have run, from the first. */
  executed: number;
  /** Autoplay is on: chapters run one after another with a pause to read. */
  playing: boolean;
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

  /** Boot the engine and seed the sandbox. Idempotent. */
  start: () => void;
  play: () => void;
  pause: () => void;
  /** Run the chapter in view, typed out. Requires it to be the next unrun chapter. */
  runCurrent: () => Promise<void>;
  /** Run the chapter in view at once if it has not run, then view the next. */
  stepForward: () => Promise<void>;
  /** View the previous chapter; nothing runs. */
  stepBack: () => void;
  /** View a chapter; chapters before it that have not run are replayed at once. */
  goTo: (index: number) => Promise<void>;
  /** Run a line the reader typed. */
  runLine: (line: string) => Promise<number>;
  /** Start the lab over: a fresh sandbox, an empty transcript. */
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

/** How long autoplay waits after a chapter, for the narration to be read. */
function readingPause(ch: Chapter): number {
  const words = ch.narration.split(/\s+/).length;
  return Math.min(14000, 1800 + words * 190);
}

const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

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
  const [playing, setPlaying] = useState(false);
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
  const playingRef = useRef(false);
  const busyRef = useRef(false);
  const startedRef = useRef(false);
  const chapterRunningRef = useRef<string | undefined>(undefined);
  const showHiddenRef = useRef(false);
  const playToken = useRef(0);

  const sandbox = `/learn/${lab.id}`;

  const seedFiles = useMemo<LabFile[]>(
    () => [...SAMPLE_TREES[lab.sample], ...(lab.files ?? [])],
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

  // ── Chapters ─────────────────────────────────────────────────────────────

  const view = useCallback(
    (index: number) => {
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
      if (!ch || i !== executedRef.current) return;
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

  const pause = useCallback(() => {
    playingRef.current = false;
    playToken.current++;
    setPlaying(false);
  }, []);

  /** Autoplay from the chapter in view to the end, unless paused. */
  const play = useCallback(() => {
    if (playingRef.current) return;
    playingRef.current = true;
    setPlaying(true);
    const token = ++playToken.current;
    void (async () => {
      // Catch up to the chapter in view at once, then play from there.
      while (executedRef.current < currentRef.current && playingRef.current) {
        await runChapter(executedRef.current, false);
      }
      while (playingRef.current && token === playToken.current) {
        const i = currentRef.current;
        if (i < executedRef.current) {
          // Viewing a chapter that already ran: move on to the first unrun one.
          if (executedRef.current >= lab.chapters.length) break;
          view(executedRef.current);
          continue;
        }
        await runChapter(i, true);
        if (!playingRef.current || token !== playToken.current) break;
        if (i + 1 >= lab.chapters.length) break;
        await sleep(readingPause(lab.chapters[i]));
        if (!playingRef.current || token !== playToken.current) break;
        view(i + 1);
      }
      if (token === playToken.current) {
        playingRef.current = false;
        setPlaying(false);
      }
    })();
  }, [lab, runChapter, view]);

  const runCurrent = useCallback(async () => {
    if (busyRef.current) return;
    const i = currentRef.current;
    if (i !== executedRef.current) return;
    await runChapter(i, true);
  }, [runChapter]);

  const stepForward = useCallback(async () => {
    if (busyRef.current) return;
    pause();
    const i = currentRef.current;
    if (i === executedRef.current) await runChapter(i, false);
    if (i + 1 < lab.chapters.length) view(i + 1);
  }, [lab, pause, runChapter, view]);

  const stepBack = useCallback(() => {
    pause();
    view(currentRef.current - 1);
  }, [pause, view]);

  const goTo = useCallback(
    async (index: number) => {
      if (busyRef.current) return;
      pause();
      const i = Math.max(0, Math.min(lab.chapters.length - 1, index));
      while (executedRef.current < i) await runChapter(executedRef.current, false);
      view(i);
    },
    [lab, pause, runChapter, view],
  );

  // ── Boot and seed ────────────────────────────────────────────────────────

  const seed = useCallback(
    async (rt: KapiRuntime) => {
      try {
        await rt.reset(sandbox);
      } catch {
        /* a fresh directory has nothing to reset */
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
        const wanted = link?.chapter ? lab.chapters.findIndex((c) => c.id === link.chapter) : -1;
        if (link?.session) {
          // The shared files are the state after the chapters before this one.
          const at = Math.max(0, wanted);
          executedRef.current = at;
          setExecuted(at);
          view(at);
          term?.write(DIM("# restored from a shared link") + "\r\n");
        } else {
          // Play was pressed: catch up to the linked chapter at once, then
          // play from there, as a video resumes at the time in its link.
          if (wanted > 0) {
            while (executedRef.current < wanted) await runChapter(executedRef.current, false);
            view(wanted);
          } else {
            view(0);
          }
          play();
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
  }, [assets, lab, link, play, prepare, refreshFiles, runChapter, sandbox, seed, view]);

  const reset = useCallback(async () => {
    const rt = runtimeRef.current;
    if (!rt || busyRef.current) return;
    pause();
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
      view(0);
    } finally {
      busyRef.current = false;
      setBusy(false);
    }
  }, [pause, prepare, refreshFiles, sandbox, seed, view]);

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

  // Stop autoplay when the page goes away.
  useEffect(() => () => pause(), [pause]);

  return {
    status,
    error,
    bootProgress,
    current,
    executed,
    playing,
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
    play,
    pause,
    runCurrent,
    stepForward,
    stepBack,
    goTo,
    runLine,
    reset,
    shareLink,
    readFile,
    inspect,
    registerTerminal,
    promptLabel,
  };
}
