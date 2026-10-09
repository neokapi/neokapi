// The heavy half of the package: xterm, the engine boot path and the player.
// Hosts import it as its own chunk, after the reader has asked for a lab.

export { default as LabPlayer } from "./LabPlayer.tsx";
export type { LabPlayerProps, UpNext } from "./LabPlayer.tsx";
export { default as ExplorerPlayer } from "./ExplorerPlayer.tsx";
export type {
  ExplorerPlayerProps,
  ResolvedStage,
  StageContext,
  StageRenderer,
} from "./ExplorerPlayer.tsx";
export { useExplorerSession } from "./useExplorerSession.ts";
export type { ExplorerSession, UseExplorerSessionOptions } from "./useExplorerSession.ts";
export { useLabSession } from "./useLabSession.ts";
export type { LabSession, UseLabSessionOptions } from "./useLabSession.ts";
export type {
  FileEntry,
  LabAssets,
  SessionStatus,
  TerminalHandle,
  TranscriptEntry,
} from "./types.ts";
