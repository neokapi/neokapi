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
  MapFileRow,
  MapSuggestionRow,
  MapSummary,
  ReaderBlock,
  ReaderFile,
  ReaderFinding,
  ReaderRefusal,
  ReaderRule,
  RuleDecision,
  RuleOption,
  RulesDecideView,
} from "./types";

export { ConflictLine, ConflictQuestion, RuleLine, RuleSentence, themeLabel } from "./DigestRule";
export { NoticedBy, NoticedMark, toolName } from "./NoticedBy";
