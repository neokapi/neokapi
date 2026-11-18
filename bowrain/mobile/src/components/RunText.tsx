// A run sequence drawn for reading. Placeholders are chips that show a
// variable's name or word the code's kind; a paired code draws its content
// as styled text, with no tags. Nothing shows a code's native data: every
// code goes through codeDisplays and CodeChip. The projection is a RunSpec
// over every run kind, so no kind can be dropped without saying so; a plural
// or select shows every form, labelled.
import { codeDisplays, projectRuns, type ModelRunSpec, type Run } from "@neokapi/kapi-format";
import { Fragment, type ReactNode } from "react";
import { Text } from "react-native";

import { CodeChip } from "./CodeChip";

export interface RunTextProps {
  runs: readonly Run[];
  /** Larger text for the card's main reading. */
  size?: "base" | "lg" | "xl";
  /** Secondary ink, for the source above a translation. */
  muted?: boolean;
  testID?: string;
}

function branchList(prefix: string, branches: Record<string, Run[] | undefined>): ReactNode {
  return Object.entries(branches).map(([form, runs]) => (
    <Text key={`${prefix}-${form}`}>
      {"\n"}
      <Text className="text-muted-foreground text-xs">{form}: </Text>
      {renderRuns(runs ?? [], `${prefix}-${form}`)}
    </Text>
  ));
}

// Text inside a paired code reads as marked: underlined and tinted, the way
// a link or a styled span sits in a sentence.
const IN_PAIR = "bg-chip/40 underline decoration-dotted";

function spec(runs: readonly Run[], prefix: string): ModelRunSpec<ReactNode> {
  const code = codeDisplays(runs);
  let i = 0;
  let depth = 0;
  const key = () => `${prefix}-${i++}`;
  return {
    text: (r) => (
      <Text key={key()} className={depth > 0 ? IN_PAIR : undefined}>
        {r.text}
      </Text>
    ),
    ph: ({ ph }) => <CodeChip key={key()} display={code.ph(ph)} testID="run-chip" explain />,
    pcOpen: () => {
      depth++;
      return null;
    },
    pcClose: () => {
      depth = Math.max(0, depth - 1);
      return null;
    },
    sub: ({ sub }) => <CodeChip key={key()} display={code.sub(sub)} testID="run-chip" explain />,
    plural: ({ plural }) => {
      const k = key();
      return (
        <Fragment key={k}>
          <CodeChip display={code.choice(plural.pivot)} testID="run-chip" explain />
          {branchList(k, plural.forms)}
        </Fragment>
      );
    },
    select: ({ select }) => {
      const k = key();
      return (
        <Fragment key={k}>
          <CodeChip display={code.choice(select.pivot)} testID="run-chip" explain />
          {branchList(k, select.cases)}
        </Fragment>
      );
    },
    fallback: () => <CodeChip key={key()} display={code.unknown()} testID="run-chip-unknown" />,
  };
}

function renderRuns(runs: readonly Run[], prefix: string): ReactNode[] {
  return projectRuns(runs, spec(runs, prefix));
}

const SIZES = {
  base: "text-base leading-6",
  lg: "text-lg leading-7",
  xl: "text-2xl leading-9 font-medium",
} as const;

export function RunText({ runs, size = "base", muted = false, testID }: RunTextProps) {
  return (
    <Text
      testID={testID}
      className={`${muted ? "text-muted-foreground" : "text-foreground"} ${SIZES[size]}`}
    >
      {renderRuns(runs, "r")}
    </Text>
  );
}
