// The port a host implements to draw the project screens: the three reads the
// Map, the Reader and Decide render from, and the actions a person can take on
// them. Each action is optional. A screen given no action draws no control for
// it, so a port with the reads alone is a read-only preview.

import type {
  ContextActor,
  KeptRule,
  MapSummary,
  ReaderFile,
  ReaderFixFiled,
  RuleAnswer,
  RuleDecideResult,
  RulesDecideView,
} from "./types";

/** A place a screen leads to. The host maps each to a view of its own. */
export type ScreenTarget =
  | { kind: "map" }
  /** One content file in the Reader, at one block when one is named. */
  | { kind: "reader"; path: string; block?: string }
  /** The questions Decide asks, on one Map row when one is named (`suggested:<id>`). */
  | { kind: "rules"; row?: string }
  /** The rules in force. */
  | { kind: "kept" }
  | { kind: "checks" }
  | { kind: "blocks" }
  /** Setting up a project whose folder is empty or matches a preset. */
  | { kind: "setup" }
  | { kind: "connect-assistant" };

/** How a screen leads elsewhere. A target the host cannot open hides its control. */
export interface ScreenNavigation {
  navigate: (target: ScreenTarget) => void;
  /** Whether the host opens a target. Absent, it opens every one. */
  canNavigate?: (target: ScreenTarget) => boolean;
}

/** Whether a navigation opens a target. */
export function opens(nav: ScreenNavigation, target: ScreenTarget): boolean {
  return nav.canNavigate ? nav.canNavigate(target) : true;
}

/**
 * What a host implements for the screens to read and act through. A screen
 * reads again whenever it is given another port object, so a host keeps the
 * port stable across renders (`useMemo`) rather than building it inline.
 */
export interface ProjectScreensPort {
  mapSummary(): Promise<MapSummary>;
  readerFile(path: string): Promise<ReaderFile>;
  decideView(): Promise<RulesDecideView>;
  /** The rules in force and who kept each. Absent, Kept by is not drawn. */
  keptRules?(): Promise<KeptRule[]>;
  /** What keeping a name with a typed form does. */
  decideConsequence?(id: string, form: string): Promise<string>;
  /** Record an answer. Absent, Decide draws no Keep, Not a rule or Later. */
  decide?(id: string, answer: RuleAnswer, form?: string): Promise<RuleDecideResult>;
  assign?(id: string, userId: string | null): Promise<void>;
  /**
   * Apply one finding's fix and answer with the file read again, or file it as
   * a proposal and answer with what was filed. Absent, the Reader draws no Fix.
   */
  fixFinding?(path: string, findingId: string): Promise<ReaderFile | ReaderFixFiled>;
  /** Apply every fix in a file, or file each as a proposal, and answer as fixFinding does. */
  fixFile?(path: string): Promise<ReaderFile | ReaderFixFiled>;
  /** Run the checks over the whole project. Absent, the Map draws no check action. */
  runChecks?(): Promise<void>;
  /** The name a person is shown by. Absent, or answering nothing, a person with no name is "you". */
  actorName?(actor: ContextActor): string | undefined;
  /** Call onChange when what a screen reads changes. Answers the function that stops it. */
  subscribe?(onChange: (what: "map" | "reader" | "decide") => void): () => void;
}
