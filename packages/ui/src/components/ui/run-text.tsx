import { Fragment, type ReactNode } from "react";
import {
  codeDisplays,
  codeWord,
  otherBranch,
  projectRuns,
  type CodeDisplay,
  type RunSpec,
} from "@neokapi/kapi-format";
import type { Run } from "@neokapi/contract-types";

/**
 * The review surfaces' reading of a run sequence: prose reads as prose, and
 * everything that is not prose reads as a labelled chip.
 *
 * The neighbourhood, the prior version and the memory match all travel as run
 * sequences, and a reviewer judging a translation has to see the placeholders
 * and the plurals inside it. The projection is declared rather than looped, so
 * a kind added to the model is a compile error here instead of a variable that
 * quietly stops being drawn (packages/kapi-format/src/run-projection.ts).
 *
 * Kapi Desktop and the Bowrain platform both read through this, so a neighbour
 * carrying a variable draws the same chip on either surface.
 */

const CHIP =
  "mx-0.5 inline-flex items-center rounded border px-1 align-baseline font-mono text-[10px] leading-tight";
const CHIP_PH = "border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-400";
const CHIP_PC = "border-blue-500/40 bg-blue-500/10 text-blue-700 dark:text-blue-300";
const CHIP_GROUP = "border-violet-500/40 bg-violet-500/10 text-violet-700 dark:text-violet-300";
const CHIP_SUB = "border-teal-500/40 bg-teal-500/10 text-teal-700 dark:text-teal-300";

const ROLE_SHAPE: Record<CodeDisplay["role"], string> = {
  placeholder: "",
  choice: "",
  reference: "",
  open: "rounded-r-none",
  close: "rounded-l-none opacity-70",
};

/**
 * A chip for one code. It takes a CodeDisplay, which only codeDisplays
 * (@neokapi/kapi-format) makes, so a code's native data or an extractor's
 * internal identifier never reaches the screen through it.
 */
function chip(cls: string, display: CodeDisplay, title: string): ReactNode {
  const word = codeWord(display);
  return (
    <span className={`${CHIP} ${cls} ${ROLE_SHAPE[display.role]}`} title={title} translate="no">
      {word}
    </span>
  );
}

function reviewRuns(runs: readonly Run[]): RunSpec<Run, ReactNode> {
  const code = codeDisplays(runs as Parameters<typeof codeDisplays>[0]);
  const spec: RunSpec<Run, ReactNode> = {
    text: (r) => <span className="whitespace-pre-wrap">{r.text}</span>,
    ph: (r) => chip(CHIP_PH, code.ph(r.ph), "placeholder"),
    pcOpen: (r) => chip(CHIP_PC, code.pcOpen(r.pcOpen), "start of styled text"),
    pcClose: (r) => chip(CHIP_PC, code.pcClose(r.pcClose), "end of styled text"),
    sub: (r) => chip(CHIP_SUB, code.sub(r.sub), "sub-block reference"),
    // A plural keeps its marker and its wording: the chip names the pivot that
    // chooses the form, and one form is read after it so the reviewer sees words.
    plural: {
      expand: (r) => [
        chip(
          CHIP_GROUP,
          code.choice(r.plural.pivot),
          `plural: ${Object.keys(r.plural.forms).join(", ")}`,
        ),
        ...projectRuns(otherBranch(r.plural.forms), reviewRuns(otherBranch(r.plural.forms))),
      ],
    },
    select: {
      expand: (r) => [
        chip(
          CHIP_GROUP,
          code.choice(r.select.pivot),
          `select: ${Object.keys(r.select.cases).join(", ")}`,
        ),
        ...projectRuns(otherBranch(r.select.cases), reviewRuns(otherBranch(r.select.cases))),
      ],
    },
    fallback: (kind) => chip(CHIP_PH, code.unknown(), `a run this build cannot draw: ${kind}`),
  };
  return spec;
}

export interface RunTextProps {
  runs: Run[] | undefined;
  className?: string;
  /** Direction and language attributes for the locale the runs are written in. */
  dirAttrs?: { dir?: string; lang?: string };
}

/** One run sequence, read inline. */
export function RunText({ runs, className, dirAttrs }: RunTextProps) {
  const parts = projectRuns(runs, reviewRuns(runs ?? []));
  return (
    <span className={className} translate="no" {...dirAttrs}>
      {parts.map((part, i) => (
        <Fragment key={i}>{part}</Fragment>
      ))}
    </span>
  );
}
