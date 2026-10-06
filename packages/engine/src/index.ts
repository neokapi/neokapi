// @neokapi/engine — the kapi content engine (compiled to WebAssembly) as a
// typed, dependency-light npm package.
//
// This package owns the boot path (the engine's dedicated Worker, its file
// system, the SQLite bridge and where the workspace is kept, the wasm_exec
// loader, instantiation, ready handshake) and the `KapiRuntime` facade over
// the engine's global function set. The wasm binaries are NOT
// bundled: the engine (~20 MB gzipped) and sqlite3.wasm are assets the host
// serves or points at (see the README for CDN vs self-hosted loading
// patterns).
//
// Deliberately free of UI and heavy ML dependencies (no xterm, monaco,
// pdfium, onnxruntime): those live in host kits such as
// @neokapi/kapi-playground, which builds on this package.

// Boot + runtime facade.
export {
  bootKapiRuntime,
  ChangeRefused,
  folderRemote,
  describeStorage,
  isBooted,
  makeRuntime,
  onBootProgress,
} from "./runtime.ts";
export type {
  AnnotateOptions,
  ApplyOptions,
  BootOptions,
  BootProgress,
  ChangeActor,
  ChangeCallOptions,
  ChangeResult,
  ChangeSet,
  ContextPullReport,
  ContextPushReport,
  ContextRemote,
  ContextSyncReport,
  ContextSyncStatus,
  DescribeRequest,
  FormatDescription,
  InspectResult,
  KapiRuntime,
  KbfRequest,
  KbfResponse,
  MemoryReason,
  PreviewBlock,
  PreviewResult,
  ReadPage,
  ReadRequest,
  SyncContextOptions,
  SegmentPiece,
  SegmentResult,
  StorageInfo,
  FolderFile,
  FolderHandle,
  RemoteObject,
  WhenHeld,
  TraceRunResult,
  WorkspaceExport,
  WorkspaceImport,
} from "./runtime.ts";

// In-memory filesystem (the Node-fs subset Go's js/wasm runtime calls).
export { createMemFS } from "./memfs.ts";
export type { MemFS, MemVolume } from "./memfs.ts";

// The SQLite bridge the engine's database driver calls (boot installs it; a
// host that starts the engine itself, such as a Node test runner, installs it
// before Go starts).
export { createSQLiteBridge, installSQLiteBridge, loadSQLite } from "./sqlite.ts";
export type {
  LoadSQLiteOptions,
  SQLiteBridge,
  SQLiteBridgeOptions,
  SQLiteFailure,
  SQLitePrepared,
} from "./sqlite.ts";

// Versioned ABI: feature detection + the raw wire shapes.
export { engineABI, hasEngineFunction, SUPPORTED_ENGINE_ABI } from "./abi.ts";
export type {
  EngineABI,
  RawInspectResponse,
  RawPreviewResponse,
  RawSegmentResponse,
  RawWorkspaceExport,
  WorkspaceProjectExport,
  WorkspaceTermStore,
} from "./abi.ts";

// Reverse-bridge capabilities the host page may provide.
export { detectCapabilities } from "./capabilities.ts";
export type {
  BrowserTranslatePayload,
  BrowserTranslateResult,
  CapabilityStatus,
  KapiBrowserTranslate,
  KapiEngineCapabilities,
  KapiLocalGenerate,
  KapiLocalNER,
  KapiPdfium,
  LocalGenerateMessage,
  LocalGeneratePayload,
  LocalGenerateResult,
  LocalNEREntity,
  LocalNERRequest,
  LocalNERResponse,
  PdfExtract,
  PdfGlyph,
  PdfPage,
  PdfRect,
} from "./capabilities.ts";

// Ambient typings for the engine globals (side-effect import so any consumer
// of the package gets typed `globalThis.kapiRun` & co.).
import "./globals.ts";
