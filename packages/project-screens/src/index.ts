// @neokapi/project-screens: the project screens kapi-desktop and the Bowrain
// web app both draw (the Overview, the Reader and Decide), with the shapes they
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
  OverviewFileRow,
  MapSuggestionRow,
  OverviewStep,
  OverviewSummary,
  ReaderBlock,
  ReaderFile,
  ReaderFinding,
  ReaderFixFiled,
  ReaderProposal,
  ReaderRefusal,
  ReaderRule,
  RuleAnswer,
  RuleDecideResult,
  RuleDecision,
  RuleOption,
  RulesDecideView,
  ScreenMember,
} from "./types";

export type { ProjectScreensPort, ScreenNavigation, ScreenTarget } from "./port";
export { opens } from "./port";

export { ConflictLine, ConflictQuestion, RuleLine, RuleSentence, themeLabel } from "./DigestRule";
export { RecordedBy, RecordedMark, memberName, toolName } from "./RecordedBy";

export { OverviewView, decideRowKey, type OverviewViewProps } from "./OverviewView";
export { ReaderView, type ReaderViewProps } from "./ReaderView";
export { KeptRulesView, type KeptRulesViewProps } from "./KeptRulesView";
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
  KeptRulesScreen,
  OverviewScreen,
  ReaderScreen,
  type DecideScreenProps,
  type KeptRulesScreenProps,
  type OverviewScreenProps,
  type ReaderScreenProps,
} from "./screens";
