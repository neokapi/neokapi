// A port over the project screens' Fernwell fixtures, for the route
// components' stories and tests. It answers the way the server's routes do:
// a fix files a source proposal and leaves the file as it is.

import type { ProjectScreensPort, ScreenMember } from "@neokapi/project-screens";
import {
  DECIDE_VIEW,
  KEPT_RULES,
  MAP_PENDING,
  READER_WITH_FIXES,
} from "@neokapi/project-screens/fixtures";
import { proposalsByFinding } from "./project-screens-port";

/** The Fernwell team: the reader, and a teammate. */
export const FERNWELL_MEMBERS: ScreenMember[] = [
  { id: "u-ada", name: "Ada Lind", me: true },
  { id: "u-mira", name: "Mira Holt" },
];

/** The Fernwell fixtures through a port with every action. */
export function fixtureScreensPort(over: Partial<ProjectScreensPort> = {}): ProjectScreensPort {
  let filed = 0;
  const nextProposal = () => `sp-fernwell-${String(++filed).padStart(4, "0")}`;
  return {
    mapSummary: async () => MAP_PENDING,
    readerFile: async () => READER_WITH_FIXES,
    decideView: async () => DECIDE_VIEW,
    keptRules: async () => KEPT_RULES,
    decideConsequence: async (_id, form) => `${form} becomes the name in 3 blocks.`,
    decide: async (_id, answer) => ({
      answer,
      kept: answer === "keep" ? 1 : 0,
      dropped: answer === "drop" ? 1 : 0,
      later: answer === "later" ? 1 : 0,
    }),
    assign: async () => {},
    members: async () => FERNWELL_MEMBERS,
    fixFinding: async (_path, findingId) => ({
      proposed: [{ finding_id: findingId, proposal: nextProposal() }],
      refused: [],
    }),
    fixFile: async () => {
      const proposals = READER_WITH_FIXES.blocks
        .filter((b) => b.findings.some((f) => f.fixable))
        .map(() => nextProposal());
      return {
        proposed: proposalsByFinding(READER_WITH_FIXES, { proposals, refused: [] }),
        refused: [],
      };
    },
    runChecks: async () => {},
    ...over,
  };
}
