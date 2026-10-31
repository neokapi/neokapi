// @neokapi/project-screens: the project screens kapi-desktop and the Bowrain
// web app both draw (the Map, the Reader and Decide), with the shapes they
// render (Apache-2.0). Each app reads its own backend and passes the data in.

export type {
  ContextActor,
  ContextEvidence,
  DigestConflict,
  DigestItem,
  DigestSubject,
  DigestTermRule,
  DigestUsage,
  KeptRule,
  MapFileRow,
  MapSuggestionRow,
  MapSummary,
  ReaderBlock,
  ReaderFile,
  ReaderFinding,
  ReaderRefusal,
  ReaderRule,
  RuleAnswer,
  RuleDecideResult,
  RuleDecision,
  RuleOption,
  RulesDecideView,
} from "./types";

export type { ProjectScreensPort, ScreenNavigation, ScreenTarget } from "./port";
export { opens } from "./port";

export { ConflictLine, ConflictQuestion, RuleLine, RuleSentence, themeLabel } from "./DigestRule";
export { NoticedBy, NoticedMark, toolName } from "./NoticedBy";

export { MapView, decideRowKey, type MapViewProps } from "./MapView";
export { ReaderView, type ReaderViewProps } from "./ReaderView";
export {
  DecideView,
  answerArgs,
  decisionForRow,
  decisionKey,
  type DecidePick,
  type DecideViewProps,
} from "./DecideView";
export {
  DecideScreen,
  MapScreen,
  ReaderScreen,
  type DecideScreenProps,
  type MapScreenProps,
  type ReaderScreenProps,
} from "./screens";
