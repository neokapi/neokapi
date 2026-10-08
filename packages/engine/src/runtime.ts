// KapiRuntime — the singleton that owns the one booted kapi WASM instance.
//
// In a browser the engine runs in a dedicated Worker (worker.ts): Go, SQLite
// and the engine's file system run there, off the page's thread, and the
// workspace can be kept in the origin private file system (storage.ts). The
// facade sends each call to the Worker and keeps a mirror of the engine's file
// system on the page, so `vol` reads stay synchronous: the Worker sends the
// changes a call made before the call's answer. Where no Worker can run (Node,
// a test environment, or a page that asks for it) the engine runs on the
// calling thread with its workspace in memory, as the smokes run it.
//
// There is exactly one live session at a time. That is fine for the embedding
// model: only one host surface (modal, lab, terminal) is ever driving the
// runtime. `setSinks` points the live stdout/stderr at whichever surface is
// active.
//
// Boot is lazy: nothing is fetched until `bootKapiRuntime()` is first called.
// Subsequent calls reuse the warm instance.

import { CHANGE_RESULT_SCHEMA_ID } from "@neokapi/contract-types";
import type {
  ChangeResult,
  ChangeSet,
  ContentTree,
  DescribeRequest,
  FormatDescription,
  ReadPage,
  ReadRequest,
} from "@neokapi/contract-types";
import type {
  EngineABI,
  RawInspectResponse,
  RawPreviewResponse,
  RawSegmentResponse,
  RawWorkspaceExport,
  WorkspaceProjectExport,
  WorkspaceTermStore,
} from "./abi.ts";
import { createMemFS } from "./memfs.ts";
import type { MemFS, MemVolume } from "./memfs.ts";
import { installSQLiteBridge, loadSQLite } from "./sqlite.ts";
import { fetchWasmBytes, instantiate, sibling } from "./fetch.ts";
import type { BootProgress, WasmExecHost } from "./fetch.ts";
import { applyChanges, changesOf } from "./storage.ts";
import { invokeEngine, localCalls } from "./local.ts";
import type { FolderHandle } from "./folderremote.ts";
import type { StorageInfo, VolOp, WhenHeld } from "./storage.ts";
import { presentBridges } from "./protocol.ts";
import type { FromWorker, ToWorker } from "./protocol.ts";
import "./globals.ts";

export type { BootProgress } from "./fetch.ts";
export type { MemoryReason, StorageInfo, WhenHeld } from "./storage.ts";
export { describeStorage } from "./storage.ts";
export { folderRemote } from "./folderremote.ts";
export type { ContextRemote, FolderFile, FolderHandle, RemoteObject } from "./folderremote.ts";

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

/** Which read-only annotators `inspectAnnotated` runs (all default to true). */
export interface AnnotateOptions {
  term?: boolean;
  brand?: boolean;
  qa?: boolean;
  /** Run segmentation and surface sentence boundaries in the preview. */
  segment?: boolean;
  /** Segmentation engine when `segment` is set ("" = srx; "uax29" = ICU4X). */
  segmentEngine?: string;
  /** Content language (BCP-47, "" = "en"); locale-sensitive engines tailor to it. */
  segmentLocale?: string;
}

export interface InspectResult {
  ok: boolean;
  error?: string;
  format?: string;
  /**
   * Parsed ContentTree (the hierarchical content-model view), typed with the
   * generated projection shape from @neokapi/contract-types (AD-034 —
   * generated from the Go core/editor structs, so it cannot drift).
   * @neokapi/kapi-lab layers its own loose refinement on top.
   */
  tree?: ContentTree;
  bytes?: number;
}

export interface TraceRunResult {
  /** Process-style exit code from the underlying kapiRun. */
  code: number;
  /**
   * Parsed FlowTrace JSON, or null when the run produced no trace file (null is
   * already covered by unknown). Typed as unknown; @neokapi/kapi-lab casts it to
   * its FlowTrace type.
   */
  trace: unknown;
}

/**
 * A KBF spec operation routed to the canonical Go engine (core/kbf) via the
 * `kbf` wasm endpoint. The shape is op-specific; see the KBF docs Lab/Tests
 * pages for the per-op payloads (roundtrip, validateBlock, validateTarget,
 * resolveAnchor, validateAnnotation, renderHtml).
 */
export interface KbfRequest {
  op:
    | "roundtrip"
    | "validateBlock"
    | "validateTarget"
    | "resolveAnchor"
    | "validateAnnotation"
    | "renderHtml";
  [key: string]: unknown;
}

/** Generic KBF endpoint response; always carries `ok`. */
export interface KbfResponse {
  ok: boolean;
  error?: string;
  [key: string]: unknown;
}

/** One sentence produced by the segmentation engine. */
export interface SegmentPiece {
  text: string;
}

/** Result of the `labSegment` wasm endpoint. */
export interface SegmentResult {
  ok: boolean;
  error?: string;
  /** The engine that actually ran (e.g. "srx" when "" was requested). */
  engine?: string;
  segments?: SegmentPiece[];
}

// The change contract's own types, so a host typed against this package names
// a read, a change set and its result without a second dependency.
export type {
  ChangeResult,
  ChangeSet,
  DescribeRequest,
  FormatDescription,
  ReadPage,
  ReadRequest,
} from "@neokapi/contract-types";

/** Who sends a change set: a person or an agent (core/change.Actor). */
export interface ChangeActor {
  kind: "person" | "agent";
  /** The agent's or the person's name, for the record. */
  name?: string;
  /** The session that groups one agent run. */
  session?: string;
}

/** What a read, an apply or a description says beside its request. */
export interface ChangeCallOptions {
  /**
   * The project the call acts on: its kapi.yaml, its root directory, or a
   * path inside it. Omitted is the project discovery finds from the working
   * directory, as for a command given no `-p`.
   */
  project?: string;
}

/** What an apply says beside its change set. */
export interface ApplyOptions extends ChangeCallOptions {
  /** Who sends the change set. Omitted is a person, as `kapi apply` records one. */
  actor?: ChangeActor;
}

/**
 * A read or a description the change service refused: a document that does
 * not exist, a stale cursor, a request that does not decode. `result` is the
 * kapi.change-result/v1 answer, whose `error` says why.
 */
export class ChangeRefused extends Error {
  readonly result: ChangeResult;

  constructor(result: ChangeResult) {
    super(result.error?.message ?? "the change service refused the request");
    this.name = "ChangeRefused";
    this.result = result;
  }
}

/** Whether an engine answer is a kapi.change-result/v1 document. */
function isChangeResult(v: unknown): v is ChangeResult {
  return (
    typeof v === "object" &&
    v !== null &&
    (v as { schema?: unknown }).schema === CHANGE_RESULT_SCHEMA_ID
  );
}

/** What an export of the workspace carries. */
export interface WorkspaceExport {
  /** The workspace package: a `.kpz` of kind `kapi-workspace`. */
  data: Uint8Array;
  /** The files it carries. */
  files: number;
  /** The projects whose context it carries. */
  projects: WorkspaceProjectExport[];
  /** The terms stores outside every project it carries, as terms bundles. */
  termStores: WorkspaceTermStore[];
  /** Projects whose context could not be read, with the reason. */
  skipped: { root: string; reason: string }[];
}

/** What reading a workspace package back did. */
export interface WorkspaceImport {
  /** The files written. */
  files: number;
  /** Each project whose context was merged, with the operations it added. */
  projects: { root: string; merged: number }[];
  /** Each terms store written, with the concepts written to it. */
  termStores: WorkspaceTermStore[];
}

/** How far apart the engine and a remote were at the last contact. */
export interface ContextSyncStatus {
  remote: { Kind: string; Location: string };
  /** The project's operations the remote is not known to hold. */
  to_push: number;
  /** The context operations among them, the ones a context log lists. */
  to_push_logged: number;
  /** The operations read from the remote and not yet merged. */
  to_pull: number;
  /** When the remote was last reached (RFC 3339). */
  contacted?: string;
  /** What the last attempt to reach it reported. */
  error?: string;
}

/** What a pull merged (the `pull` half of `kapi context sync --json`). */
export interface ContextPullReport extends ContextSyncStatus {
  segments: number;
  merged: number;
  rebuilt?: boolean;
  checkpoint?: string;
  /** Nothing has been pushed to the remote yet. */
  empty?: boolean;
  seconds: number;
}

/** What a push wrote (the `push` half of `kapi context sync --json`). */
export interface ContextPushReport extends ContextSyncStatus {
  pushed: number;
  /** The context operations among them (`to_push_logged`). */
  pushed_logged: number;
  segments: number;
  blobs: number;
  checkpoint?: string;
  seconds: number;
}

/** What {@link KapiRuntime.syncContext} did: each part is absent when it was not asked for. */
export interface ContextSyncReport {
  pull?: ContextPullReport;
  push?: ContextPushReport;
}

/** What {@link KapiRuntime.syncContext} syncs. */
export interface SyncContextOptions extends ChangeCallOptions {
  /** Pull from the folder. With neither set, both run. */
  pull?: boolean;
  /** Push to the folder. With neither set, both run. */
  push?: boolean;
}

export interface KapiRuntime {
  /**
   * The engine's file system. For an engine in a Worker this is a mirror on
   * the page: a write reaches the engine before the next call, and a call's
   * changes are here when its Promise resolves.
   */
  vol: MemVolume;
  /**
   * Where the engine keeps its databases and files. It changes when another
   * tab takes the workspace over (`reason: "taken"`) and when this tab takes
   * it over ({@link takeOver}); {@link onStorageChange} reports each change.
   */
  readonly storage: StorageInfo;
  /** Subscribe to changes of {@link storage}. Returns an unsubscribe. */
  onStorageChange(fn: (info: StorageInfo) => void): () => void;
  /**
   * Take the workspace over from the tab of this site that holds it. The
   * calls in flight finish first. This tab then starts an engine on the kept
   * workspace, whose files replace the ones this tab held in memory, and the
   * tab that held it is told (its `storage` becomes `{ kind: "memory",
   * reason: "taken" }`) and carries on in memory with its files. Resolves to
   * the new `storage`; rejects, leaving this tab as it was, when the
   * workspace cannot be opened. A tab that already holds it resolves at once.
   */
  takeOver(): Promise<StorageInfo>;
  run(argv: string[]): Promise<number>;
  preview(path: string): Promise<PreviewResult>;
  /** Inspect a file's content model, returning the parsed ContentTree. */
  inspect(path: string): Promise<InspectResult>;
  /**
   * Inspect a file like {@link inspect}, but run the engine's read-only
   * annotators (terminology, brand vocabulary, rule-based checks) first so the
   * parsed blocks carry stand-off overlays. `opts` toggles individual annotators
   * (term/brand/qa); all default to true. Wraps the `labInspectAnnotated` global.
   */
  inspectAnnotated(path: string, opts?: AnnotateOptions): Promise<InspectResult>;
  /** Run a KBF spec operation against the canonical Go engine. */
  kbf(req: KbfRequest): Promise<KbfResponse>;
  /** Segment raw text with a named engine ("" = default srx) and locale. */
  segment(text: string, engine: string, locale: string): Promise<SegmentResult>;
  /** List the segmentation engines registered in this wasm build. */
  segmentEngines(): Promise<string[]>;
  /**
   * Run a command with flow tracing enabled and return the parsed FlowTrace.
   * Appends `--trace <tmp>` to argv, runs it, and reads the trace back from the
   * engine's file system. The caller supplies the command plus its input and
   * output args, e.g. ["pseudo-translate", "/p/in.json", "-o", "/p/out.json"]
   * or ["run", "translate-qa", "-i", "/p/in.json", "-o", "/p/out.json"].
   */
  runWithTrace(argv: string[]): Promise<TraceRunResult>;
  /**
   * Start `dir` over as a fresh page would find it: forget the projects at
   * or below it, with their stores and context, and remove its databases,
   * then remove its files. Databases live outside the file system, so
   * removing the files alone would leave them. An engine without the
   * `kapiReset` entry point removes the files only.
   */
  reset(dir: string): Promise<void>;
  /**
   * Remove the database held at the absolute `path`, as `rm` removes a file.
   * Answers false when no database is held there. Rejects when one is open.
   */
  removeDatabase(path: string): Promise<boolean>;
  /**
   * Read a page of a document's blocks through the change service, as
   * `kapi inspect` reads them: each block carries the reference to copy into
   * an operation's `at` and the revision to send as its `if_match`. Pass the
   * page's `next` back as `cursor` for the following page. Throws
   * {@link ChangeRefused} when the service refuses the read.
   */
  read(req: ReadRequest, opts?: ChangeCallOptions): Promise<ReadPage>;
  /**
   * Apply a kapi.change/v1 change set through the change service, as
   * `kapi apply` does, and resolve to its result whatever the status: a
   * refused change set (a stale revision, a dropped inline code, a change set
   * that does not decode) is a result with status `refused` that says why, and
   * writes nothing. In a project the change is recorded in the workspace log.
   */
  apply(set: ChangeSet, opts?: ApplyOptions): Promise<ChangeResult>;
  /**
   * Say what a format supports of the contract: a format by name, or the
   * format a document is read in. Throws {@link ChangeRefused} when the
   * service refuses the request.
   */
  describe(req: DescribeRequest, opts?: ChangeCallOptions): Promise<FormatDescription>;
  /**
   * Pack the engine's files and the context of every project among them as a
   * workspace `.kpz`, to keep the workspace somewhere other than this browser.
   * A project's context travels as its operation log.
   */
  exportWorkspace(): Promise<WorkspaceExport>;
  /**
   * Read a workspace `.kpz` back: write its files, replacing a file at the
   * same path, and merge each project's context into the project's log.
   */
  importWorkspace(data: Uint8Array): Promise<WorkspaceImport>;
  /**
   * Share a project's context through a folder: pull what others pushed to
   * it, then push what this engine recorded, as `kapi context sync` does
   * for a `file` backend. `folder` is a directory
   * handle, from `showDirectoryPicker()` or the origin private file system.
   * The folder keeps the layout a `file` backend keeps on disk, so a machine
   * whose recipe names the same folder shares the context.
   */
  syncContext(folder: FolderHandle, opts?: SyncContextOptions): Promise<ContextSyncReport>;
  cwd(): string;
  chdir(dir: string): void;
  /** Point the live stdout/stderr sinks at a destination (the active terminal). */
  setSinks(out: (s: string) => void, err: (s: string) => void): void;
}

// Monotonic counter for unique trace paths, so a re-run never reads a stale
// trace from a prior call.
let traceSeq = 0;

// The one active session's output sinks. Only one terminal is ever live, so a
// single pair of refs suffices — no per-embed isolation needed (see #658).
let outSink: (s: string) => void = () => {};
let errSink: (s: string) => void = () => {};

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>("script[data-kapi-wasm-exec]");
    if (existing && (globalThis as WasmExecHost).Go) return resolve();
    const script = existing ?? document.createElement("script");
    script.addEventListener("load", () => resolve(), { once: true });
    script.addEventListener(
      "error",
      () => {
        script.remove();
        reject(new Error(`failed to load ${src}`));
      },
      { once: true },
    );
    if (!existing) {
      script.src = src;
      script.dataset.kapiWasmExec = "1";
      document.head.appendChild(script);
    }
  });
}

// ---------------------------------------------------------------------------
// Boot progress
// ---------------------------------------------------------------------------

const bootProgressListeners = new Set<(p: BootProgress) => void>();
let lastBootProgress: BootProgress | null = null;

function emitBootProgress(p: BootProgress) {
  lastBootProgress = p;
  for (const fn of bootProgressListeners) fn(p);
}

/**
 * Subscribe to engine-boot download progress. The last event is replayed on
 * subscribe (so a late subscriber catches up); returns an unsubscribe.
 */
export function onBootProgress(fn: (p: BootProgress) => void): () => void {
  bootProgressListeners.add(fn);
  if (lastBootProgress) fn(lastBootProgress);
  return () => bootProgressListeners.delete(fn);
}

// ---------------------------------------------------------------------------
// The facade over an engine, wherever it runs
// ---------------------------------------------------------------------------

/** An engine the facade calls: on this thread, or in a Worker. */
interface Engine {
  /** Call an engine global, or one of the calls beside them (localCalls). */
  call(fn: string, args: unknown[]): Promise<unknown>;
  /** Whether the engine registered a global of this name. */
  has(fn: string): boolean;
  vol: MemVolume;
  storage: StorageInfo;
  cwd(): string;
  chdir(dir: string): void;
  /** Subscribe to changes of `storage`; returns an unsubscribe. */
  onStorageChange(fn: (info: StorageInfo) => void): () => void;
  /** Take the workspace over from the tab that holds it. */
  takeOver(): Promise<StorageInfo>;
}

function facade(engine: Engine): KapiRuntime {
  const dec = new TextDecoder();

  const parseInspect = (res: RawInspectResponse): InspectResult => {
    if (!res || !res.ok) return { ok: false, error: res?.error ?? "inspect failed" };
    try {
      return {
        ok: true,
        format: res.format,
        tree: res.json ? (JSON.parse(res.json) as ContentTree) : undefined,
        bytes: res.bytes,
      };
    } catch (e) {
      return { ok: false, error: `parse content tree: ${(e as Error).message}` };
    }
  };

  // The change contract's entry points are looked up per call, so a facade
  // over an engine that predates them still boots and says so when used.
  const changeCall = async (
    name: "kapiRead" | "kapiApply" | "kapiDescribe",
    request: unknown,
    opts: ChangeCallOptions | undefined,
  ): Promise<unknown> => {
    if (!engine.has(name)) {
      throw new Error(
        `engine global ${name} is not registered (this engine predates the change contract entry points)`,
      );
    }
    const args = opts ? [JSON.stringify(request), JSON.stringify(opts)] : [JSON.stringify(request)];
    return JSON.parse((await engine.call(name, args)) as string) as unknown;
  };
  // A read and a description answer their own type, or the refusal they were.
  const answered = <T>(v: unknown): T => {
    if (isChangeResult(v)) throw new ChangeRefused(v);
    return v as T;
  };

  return {
    vol: engine.vol,
    get storage() {
      return engine.storage;
    },
    onStorageChange: (fn) => engine.onStorageChange(fn),
    takeOver: () => engine.takeOver(),
    run: async (argv: string[]) => (await engine.call("kapiRun", [argv])) as number,
    preview: async (path: string): Promise<PreviewResult> =>
      (await engine.call("kapiPreview", [path])) as RawPreviewResponse,
    inspect: async (path: string): Promise<InspectResult> =>
      parseInspect((await engine.call("labInspect", [path])) as RawInspectResponse),
    inspectAnnotated: async (path: string, opts?: AnnotateOptions): Promise<InspectResult> => {
      if (!engine.has("labInspectAnnotated")) {
        return { ok: false, error: "labInspectAnnotated unavailable in this wasm build" };
      }
      // The wasm endpoint accepts an optional JSON options string; omit it to
      // let the engine default all annotators on.
      const args = opts ? [path, JSON.stringify(opts)] : [path];
      return parseInspect((await engine.call("labInspectAnnotated", args)) as RawInspectResponse);
    },
    kbf: async (req: KbfRequest): Promise<KbfResponse> => {
      if (!engine.has("kbf")) {
        return { ok: false, error: "kbf endpoint unavailable in this wasm build" };
      }
      try {
        return JSON.parse(
          (await engine.call("kbf", [JSON.stringify(req)])) as string,
        ) as KbfResponse;
      } catch (e) {
        return { ok: false, error: `kbf request failed: ${(e as Error).message}` };
      }
    },
    segment: async (text: string, engineName: string, locale: string): Promise<SegmentResult> => {
      // The asynchronous entry point may wait for a page bridge (ICU4X);
      // an engine that predates it answers through the synchronous one.
      const fn = engine.has("labSegmentAsync") ? "labSegmentAsync" : "labSegment";
      if (!engine.has(fn)) {
        return { ok: false, error: "segment endpoint unavailable in this wasm build" };
      }
      try {
        const res = (await engine.call(fn, [text, engineName, locale])) as RawSegmentResponse;
        if (!res || !res.ok) return { ok: false, error: res?.error ?? "segment failed" };
        const segs = (res.segments ?? []).map((s) => ({ text: s.text }));
        return { ok: true, engine: res.engine, segments: segs };
      } catch (e) {
        return { ok: false, error: `segment request failed: ${(e as Error).message}` };
      }
    },
    segmentEngines: async (): Promise<string[]> => {
      if (!engine.has("labSegmentEngines")) return [];
      try {
        return Array.from(((await engine.call("labSegmentEngines", [])) as string[]) ?? []);
      } catch {
        return [];
      }
    },
    runWithTrace: async (argv: string[]): Promise<TraceRunResult> => {
      const tracePath = `/.lab/trace-${++traceSeq}.json`;
      const code = (await engine.call("kapiRun", [[...argv, "--trace", tracePath]])) as number;
      try {
        // kapiRun resolves only after the command (incl. the synchronous
        // trace write) completes, so the file is present to read here.
        const data = (await engine.call("take", [tracePath])) as Uint8Array | null;
        return { code, trace: data ? JSON.parse(dec.decode(data)) : null };
      } catch {
        return { code, trace: null };
      }
    },
    reset: async (dir: string): Promise<void> => {
      await engine.call("reset", [dir]);
    },
    removeDatabase: async (path: string): Promise<boolean> =>
      (await engine.call("removeDatabase", [path])) as boolean,
    read: async (req: ReadRequest, opts?: ChangeCallOptions): Promise<ReadPage> =>
      answered<ReadPage>(await changeCall("kapiRead", req, opts)),
    apply: async (set: ChangeSet, opts?: ApplyOptions): Promise<ChangeResult> => {
      const res = await changeCall("kapiApply", set, opts);
      if (!isChangeResult(res)) {
        throw new Error("kapiApply answered something other than a kapi.change-result/v1 result");
      }
      return res;
    },
    describe: async (req: DescribeRequest, opts?: ChangeCallOptions): Promise<FormatDescription> =>
      answered<FormatDescription>(await changeCall("kapiDescribe", req, opts)),
    exportWorkspace: async (): Promise<WorkspaceExport> => {
      if (!engine.has("kapiExportWorkspace")) {
        throw new Error("this engine predates the workspace export (kapiExportWorkspace)");
      }
      const raw = (await engine.call("kapiExportWorkspace", [])) as RawWorkspaceExport;
      return {
        data: raw.data,
        files: raw.files,
        projects: Array.from(raw.projects ?? []),
        termStores: Array.from(raw.termStores ?? []),
        skipped: Array.from(raw.skipped ?? []),
      };
    },
    importWorkspace: async (data: Uint8Array): Promise<WorkspaceImport> => {
      if (!engine.has("kapiImportWorkspace")) {
        throw new Error("this engine predates the workspace import (kapiImportWorkspace)");
      }
      const res = JSON.parse(
        (await engine.call("kapiImportWorkspace", [data])) as string,
      ) as Partial<WorkspaceImport>;
      return {
        files: res.files ?? 0,
        projects: res.projects ?? [],
        termStores: res.termStores ?? [],
      };
    },
    syncContext: async (folder, opts): Promise<ContextSyncReport> => {
      if (!engine.has("kapiSyncContext")) {
        throw new Error("this engine predates context sync (kapiSyncContext)");
      }
      return JSON.parse(
        (await engine.call("syncContext", [folder, JSON.stringify(opts ?? {})])) as string,
      ) as ContextSyncReport;
    },
    cwd: () => engine.cwd(),
    chdir: (dir: string) => engine.chdir(dir),
    setSinks: (out, err) => {
      outSink = out;
      errSink = err;
    },
  };
}

/** Throw a clear error when a required engine global is missing. */
function requireFn(name: string): void {
  if (typeof (globalThis as Record<string, unknown>)[name] !== "function") {
    throw new Error(`engine global ${name} is not registered (wasm not booted?)`);
  }
}

/**
 * Wire the {@link KapiRuntime} facade over the engine globals on this thread
 * and the engine's memfs. Its workspace lives in memory. Split from boot so
 * the facade is unit-testable against a mocked global surface, and so a Node
 * harness that boots the engine itself can drive it; hosts should call
 * {@link bootKapiRuntime} instead.
 */
export function makeRuntime(
  mem: MemFS,
  storage: StorageInfo = { kind: "memory", reason: "main-thread" },
): KapiRuntime {
  requireFn("kapiRun");
  requireFn("kapiPreview");
  requireFn("labInspect");
  const local = localCalls(mem);
  return facade({
    call: (fn, args) => invokeEngine(local, fn, args),
    has: (fn) => fn in local || typeof (globalThis as Record<string, unknown>)[fn] === "function",
    vol: mem.vol,
    storage,
    cwd: () => mem.vol.cwd(),
    chdir: (dir) => mem.process.chdir(dir),
    onStorageChange: () => () => {},
    takeOver: () => Promise.reject(new Error("only an engine in a Worker can keep the workspace")),
  });
}

// ---------------------------------------------------------------------------
// The engine in a Worker
// ---------------------------------------------------------------------------

/** Answer a Worker's question to one of the page's capabilities. */
async function answerBridge(name: string, args: unknown[]): Promise<unknown> {
  const g = globalThis as Record<string, unknown>;
  const dot = name.indexOf(".");
  if (dot < 0) {
    const fn = g[name];
    if (typeof fn !== "function") throw new Error(`the page has no ${name}`);
    return await (fn as (...a: unknown[]) => unknown)(...args);
  }
  const obj = g[name.slice(0, dot)] as Record<string, unknown> | undefined;
  const member = name.slice(dot + 1);
  if (!obj) throw new Error(`the page has no ${name.slice(0, dot)}`);
  const v = obj[member];
  if (typeof v === "function") return await (v as (...a: unknown[]) => unknown).apply(obj, args);
  return await v;
}

/** Copy the bytes a message carries so the page keeps its own. */
function ownBytes(data: Uint8Array): Uint8Array {
  const copy = new Uint8Array(data.length);
  copy.set(data);
  return copy;
}

/** What booting the engine in a Worker needs. */
interface WorkerBootConfig {
  wasmExecUrl: string;
  wasmUrl: string;
  sqliteWasmUrl: string;
  persist: string | null;
  whenHeld: WhenHeld;
}

/** The message a call in flight fails with when another tab takes the workspace. */
const TAKEN = "another tab took over the workspace; this tab now keeps its files in memory";

/** A booted Worker the facade sends calls to. */
interface Session {
  worker: Worker;
  pending: Map<number, { resolve: (v: unknown) => void; reject: (e: Error) => void }>;
  abi: EngineABI | null;
}

/** A Worker that has just reported ready. */
interface Started {
  session: Session;
  info: StorageInfo;
  ops: VolOp[];
  cwd: string;
}

/** A Worker lost the workspace before it was ready. */
class LostBeforeReady extends Error {}

/**
 * Boot the engine in a dedicated Worker and wire the facade to it. The page
 * keeps a mirror of the engine's file system: the Worker sends the changes a
 * call made before the call's answer, and a write on the page reaches the
 * Worker before the next call (messages keep their order).
 *
 * The facade outlives the Worker it starts with. When another tab takes the
 * workspace over, the Worker says so (`lost`); the facade fails the calls in
 * flight, stops the Worker (which frees the pool's files for the new owner)
 * and starts another in memory, handing it the page's files, so the tab keeps
 * working. `takeOver()` starts a Worker that takes the workspace from the tab
 * holding it, and moves the facade onto it.
 */
function bootWorker(spawn: () => Worker, boot: WorkerBootConfig): Promise<KapiRuntime> {
  const mirror = createMemFS();
  let current: Session | null = null;
  // While a replacement Worker boots, calls wait for it.
  let switching: Promise<Session> | null = null;
  let abi: EngineABI | null = null;
  let storage: StorageInfo = { kind: "memory" };
  let booted = false;
  let nextId = 1;
  const storageListeners = new Set<(info: StorageInfo) => void>();
  const setStorage = (info: StorageInfo) => {
    storage = info;
    for (const fn of storageListeners) fn(info);
  };

  const send = (msg: ToWorker, transfer: Transferable[] = []) =>
    current?.worker.postMessage(msg, transfer);

  // The page's writes go to the mirror and to the Worker.
  const vol: MemVolume = {
    ...mirror.vol,
    writeFile(p, data) {
      mirror.vol.writeFile(p, data);
      const path = absolute(p);
      send({
        t: "vol",
        ops: [{ op: "file", path, data: ownBytes(data), mtime: mirror.mtime(path) ?? Date.now() }],
      });
    },
    mkdirp(p) {
      mirror.vol.mkdirp(p);
      send({ t: "vol", ops: [{ op: "dir", path: absolute(p) }] });
    },
    remove(p) {
      mirror.vol.remove(p);
      send({ t: "vol", ops: [{ op: "rm", path: absolute(p) }] });
    },
  };
  const absolute = (p: string) => {
    const joined = p.startsWith("/") ? p : `${mirror.vol.cwd().replace(/\/$/, "")}/${p}`;
    const parts: string[] = [];
    for (const seg of joined.split("/")) {
      if (seg === "" || seg === ".") continue;
      if (seg === "..") parts.pop();
      else parts.push(seg);
    }
    return `/${parts.join("/")}`;
  };
  const syncCwd = (cwd: string) => {
    if (cwd === mirror.vol.cwd()) return;
    try {
      mirror.vol.mkdirp(cwd);
      mirror.process.chdir(cwd);
    } catch {
      /* the mirror catches up with the next change */
    }
  };

  /** Start a Worker and resolve once it is ready. */
  const start = (config: WorkerBootConfig): Promise<Started> =>
    new Promise<Started>((resolveStart, rejectStart) => {
      const session: Session = { worker: spawn(), pending: new Map(), abi: null };
      let ready = false;
      const isCurrent = () => current === session;
      session.worker.onmessage = (ev: MessageEvent<FromWorker>) => {
        const msg = ev.data;
        switch (msg.t) {
          case "progress":
            // A Worker started after boot (a takeover, or a restart in
            // memory) reuses the fetched engine; only the first reports.
            if (!booted) emitBootProgress(msg.progress);
            break;
          case "out":
            outSink(msg.text);
            break;
          case "err":
            errSink(msg.text);
            break;
          case "vol":
            if (isCurrent()) applyChanges(mirror, msg.ops);
            break;
          case "ready":
            ready = true;
            session.abi = msg.abi;
            resolveStart({ session, info: msg.storage, ops: msg.ops, cwd: msg.cwd });
            break;
          case "boot-error":
            rejectStart(new Error(msg.error));
            session.worker.terminate();
            break;
          case "result": {
            if (isCurrent()) syncCwd(msg.cwd);
            const call = session.pending.get(msg.id);
            session.pending.delete(msg.id);
            if (!call) break;
            if (msg.ok) call.resolve(msg.value);
            else call.reject(new Error(msg.error));
            break;
          }
          case "bridge":
            answerBridge(msg.name, msg.args).then(
              (value) =>
                session.worker.postMessage({ t: "bridge-result", id: msg.id, ok: true, value }),
              (e: unknown) =>
                session.worker.postMessage({
                  t: "bridge-result",
                  id: msg.id,
                  ok: false,
                  error: e instanceof Error ? e.message : String(e),
                }),
            );
            break;
          case "lost":
            if (!ready) {
              session.worker.terminate();
              rejectStart(new LostBeforeReady(TAKEN));
            } else if (isCurrent()) {
              lose(session);
            } else {
              session.worker.terminate();
            }
            break;
        }
      };
      session.worker.onerror = (ev) => {
        rejectStart(new Error(ev.message || "the engine's Worker failed to start"));
      };
      session.worker.postMessage({ t: "boot", boot: config } satisfies ToWorker);
    });

  /** Move the facade onto a Worker that has just started. */
  const adopt = (s: Started, replaceFiles: boolean) => {
    current = s.session;
    abi = s.session.abi ?? abi;
    if (replaceFiles) {
      for (const name of mirror.vol.readdir("/")) mirror.vol.remove(`/${name}`);
    }
    applyChanges(mirror, s.ops);
    syncCwd(s.cwd);
    // engineABI() and hasEngineFunction() read the descriptor from the page;
    // the engine's own globals are in the Worker.
    if (abi) {
      const descriptor = abi;
      globalThis.kapiEngineABI = () => descriptor;
    }
  };

  /** Fail every call in flight on a session and stop its Worker. */
  const stop = (session: Session, why: string) => {
    for (const call of session.pending.values()) call.reject(new Error(why));
    session.pending.clear();
    session.worker.terminate();
  };

  /**
   * Another tab took the workspace over: stop this tab's Worker so the pool's
   * files are free, and carry on in memory with the page's files.
   */
  const lose = (session: Session) => {
    current = null;
    stop(session, TAKEN);
    setStorage({ kind: "memory", reason: "taken" });
    switching = start({ ...boot, persist: null })
      .then((s) => {
        current = s.session;
        abi = s.session.abi ?? abi;
        // The new engine starts from the page's files.
        s.session.worker.postMessage({
          t: "vol",
          ops: changesOf(mirror, ["/"]),
        } satisfies ToWorker);
        s.session.worker.postMessage({ t: "chdir", dir: mirror.vol.cwd() } satisfies ToWorker);
        applyChanges(mirror, s.ops);
        return s.session;
      })
      .finally(() => {
        switching = null;
      });
    switching.catch((e: unknown) => {
      errSink(`kapi: could not restart the engine in memory: ${(e as Error).message}\n`);
    });
  };

  const sessionForCall = async (): Promise<Session> => {
    if (current) return current;
    if (switching) return switching;
    throw new Error("the engine is not running");
  };

  const engine: Engine = {
    call: async (fn, args) => {
      const session = await sessionForCall();
      return new Promise((resolve, reject) => {
        const id = nextId++;
        session.pending.set(id, { resolve, reject });
        session.worker.postMessage({
          t: "call",
          id,
          fn,
          args,
          bridges: presentBridges(),
        } satisfies ToWorker);
      });
    },
    has: (fn) =>
      ["removeDatabase", "reset", "take"].includes(fn) ||
      (abi ? abi.functions.includes(fn) : false),
    vol,
    get storage() {
      return storage;
    },
    cwd: () => mirror.vol.cwd(),
    chdir: (dir) => {
      mirror.process.chdir(dir);
      send({ t: "chdir", dir: mirror.vol.cwd() });
    },
    onStorageChange: (fn) => {
      storageListeners.add(fn);
      return () => storageListeners.delete(fn);
    },
    takeOver: async () => {
      if (storage.kind === "opfs") return storage;
      if (boot.persist === null) {
        throw new Error("this page keeps its workspace in memory (persist: false)");
      }
      const old = current ?? (await switching);
      if (!old) throw new Error("the engine is not running");
      // The calls in flight finish on the engine they started on.
      await waitIdle(old);
      const next = await start({ ...boot, whenHeld: "take" });
      if (next.info.kind !== "opfs") {
        next.session.worker.terminate();
        const why = next.info.detail ?? next.info.reason ?? "unknown";
        throw new Error(`could not take over the workspace: ${why}`);
      }
      // This tab's files in memory give way to the workspace it took over.
      adopt(next, true);
      stop(old, "this tab took over the workspace");
      setStorage(next.info);
      return storage;
    },
  };

  return start(boot)
    .catch((e: unknown) => {
      // The workspace was taken while this tab was still starting: run in memory.
      if (!(e instanceof LostBeforeReady)) throw e;
      return start({ ...boot, persist: null }).then((s) => ({
        ...s,
        info: { kind: "memory", reason: "taken" } as StorageInfo,
      }));
    })
    .then((s) => {
      adopt(s, false);
      storage = s.info;
      booted = true;
      return facade(engine);
    });
}

/** Resolve once no call is in flight on a session. */
async function waitIdle(session: Session): Promise<void> {
  while (session.pending.size) {
    await new Promise((r) => setTimeout(r, 20));
  }
}

let booting: Promise<KapiRuntime> | null = null;

/** Options for {@link bootKapiRuntime}. */
export interface BootOptions {
  /**
   * URL of `sqlite3.wasm` from `@sqlite.org/sqlite-wasm`, the release this
   * package depends on. The engine's stores run on it. Defaults to
   * `sqlite3.wasm` beside the engine binary, where `make web-wasm-cli` stages
   * it; a precompressed `.gz` sibling is preferred as for the engine.
   */
  sqliteWasmUrl?: string;
  /**
   * Run the engine in a dedicated Worker (the default where Worker exists),
   * or on the page's thread with `false`. Only an engine in a Worker can keep
   * its workspace.
   */
  worker?: boolean;
  /**
   * Keep the workspace in the origin private file system, so it outlives the
   * page and the browser: `true` (the default) under the key `"kapi"`, a
   * string to name the key, or `false` to keep it in memory. Pages that serve
   * different engines on one origin (a stable and a preview build, say) name
   * different keys, since a workspace belongs to the engine that wrote it.
   */
  persist?: boolean | string;
  /**
   * What this tab does when another tab of the site holds the workspace:
   * `"memory"` (the default) runs in memory and says so (`reason:
   * "another-tab"`), and the page can call `takeOver()` later; `"wait"` waits
   * until the owner closes or lets go; `"take"` takes the workspace over at
   * once, and the owner carries on in memory (`reason: "taken"`).
   */
  whenHeld?: WhenHeld;
}

/**
 * Boot the kapi CLI wasm once and return the shared runtime. Idempotent: the
 * first call starts the boot; later calls await the same promise.
 *
 * `wasmExecUrl` is Go's wasm_exec.js (shipped next to the engine asset);
 * `wasmUrl` is the engine binary — a precompressed sibling `<wasmUrl>.gz` is
 * preferred when the platform can inflate it.
 *
 * The engine's stores are SQL, so boot also loads SQLite (`sqlite3.wasm`,
 * see {@link BootOptions}) and installs the bridge its database driver calls
 * (sqlite.ts) before Go starts. In a Worker, both run there and the workspace
 * is kept across reloads where the browser allows it; `runtime.storage` says
 * where it is.
 */
export function bootKapiRuntime(
  wasmExecUrl: string,
  wasmUrl: string,
  opts: BootOptions = {},
): Promise<KapiRuntime> {
  if (booting) return booting;
  const sqliteWasmUrl = opts.sqliteWasmUrl ?? sibling(wasmUrl, "sqlite3.wasm");
  const inWorker = (opts.worker ?? true) && typeof Worker !== "undefined";
  booting = (inWorker ? bootInWorker() : bootOnThisThread())
    .then((rt) => {
      emitBootProgress({
        loaded: lastBootProgress?.loaded ?? 0,
        total: lastBootProgress?.total ?? null,
        done: true,
      });
      return rt;
    })
    .catch((error: unknown) => {
      booting = null;
      lastBootProgress = null;
      throw error;
    });
  return booting;

  function bootInWorker(): Promise<KapiRuntime> {
    // Absolute URLs: the Worker resolves relative ones against its own script.
    const base = typeof location !== "undefined" ? location.href : undefined;
    const abs = (u: string) => new URL(u, base).href;
    // A takeover, or a restart in memory, starts another Worker.
    const spawn = () =>
      new Worker(new URL("./worker.ts", import.meta.url), {
        type: "module",
        name: "kapi-engine",
      });
    const persist = opts.persist ?? true;
    return bootWorker(spawn, {
      wasmExecUrl: abs(wasmExecUrl),
      wasmUrl: abs(wasmUrl),
      sqliteWasmUrl: abs(sqliteWasmUrl),
      persist: persist === false ? null : persist === true ? "kapi" : persist,
      whenHeld: opts.whenHeld ?? "memory",
    });
  }

  async function bootOnThisThread(): Promise<KapiRuntime> {
    // Fetched beside the engine; awaited just before Go starts.
    const sqliteReady = loadSQLite({ wasmUrl: sqliteWasmUrl });
    sqliteReady.catch(() => {});
    const dec = new TextDecoder();
    const mem = createMemFS({
      onStdout: (c) => outSink(dec.decode(c)),
      onStderr: (c) => errSink(dec.decode(c)),
    });

    const g = globalThis as WasmExecHost;
    // Install our fs/process BEFORE wasm_exec.js runs, so it doesn't install
    // its own enosys defaults. Preserve any existing process.env.
    g.fs = mem.fs;
    const existingProc = g.process ?? {};
    g.process = Object.assign({}, existingProc, mem.process, { env: existingProc.env || {} });

    await loadScript(wasmExecUrl);
    const Go = g.Go;
    if (!Go) {
      document.querySelector("script[data-kapi-wasm-exec]")?.remove();
      throw new Error("wasm_exec.js did not define Go");
    }

    const go = new Go();
    // stdout isn't a TTY in the browser, so force color for JSON output
    // (--json / --jq) — the terminal renders ANSI just fine.
    go.env = { CLICOLOR_FORCE: "1" };
    const ready = new Promise<void>((res) => {
      globalThis.__kapiCliReady = res;
    });

    const source = await fetchWasmBytes(wasmUrl, emitBootProgress);
    installSQLiteBridge(await sqliteReady);
    const instance = await instantiate(source, go.importObject);
    // A startup failure must reject boot instead of leaving the ready wait pending.
    await Promise.race([
      ready,
      go.run(instance).then(() => {
        throw new Error("kapi engine exited before becoming ready");
      }),
    ]);
    const reason = opts.worker === false ? "main-thread" : "unsupported";
    return makeRuntime(mem, { kind: "memory", reason });
  }
}

/** True once boot has been started (used to skip a loading flash on re-open). */
export function isBooted(): boolean {
  return booting !== null;
}
