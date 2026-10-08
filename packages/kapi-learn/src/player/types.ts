// Shared types for the player.

import type { KapiRuntime } from "@neokapi/kapi-playground/runtime";

/** The engine assets the host serves (the Docusaurus adapter resolves them). */
export interface LabAssets {
  wasmExecUrl: string;
  wasmUrl: string;
  /** The key the engine keeps its workspace under in this browser. */
  storageKey?: string;
}

export type SessionStatus = "idle" | "booting" | "seeding" | "ready" | "running" | "error";

/** One command that ran in the session, for the transcript and a share. */
export interface TranscriptEntry {
  /** The chapter that ran it, or undefined for a command the reader typed. */
  chapterId?: string;
  command: string;
  /** Exit code, or null while running. */
  code: number | null;
  at: number;
}

/** A file in the sandbox, as the files pane lists it. */
export interface FileEntry {
  /** Path relative to the sandbox. */
  path: string;
  /** The file's size in bytes. */
  size: number;
  /** Written or changed since the lab started (by a chapter or by the reader). */
  changed: boolean;
}

/** What the terminal gives the session: a screen to write on and a prompt to drive. */
export interface TerminalHandle {
  /** Type a line at the prompt (animated or at once), then run it as if Enter were pressed. */
  typeAndRun(line: string, animate: boolean): Promise<number>;
  /** Write raw text (ANSI allowed). */
  write(text: string): void;
  clear(): void;
  focus(): void;
}

export type { KapiRuntime };
