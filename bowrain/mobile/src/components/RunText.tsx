// A run sequence drawn as text, with each placeholder and inline code as a
// chip. The projection is a RunSpec over every run kind, so no kind can be
// dropped without saying so; a plural or select shows every form, labelled.
import { projectRuns, type ModelRunSpec, type Run } from "@neokapi/kapi-format";
import { Fragment, type ReactNode } from "react";
import { Text } from "react-native";

import { codeLabels } from "./codeLabels";

export interface RunTextProps {
  runs: readonly Run[];
  /** Larger text for the card's main reading. */
  size?: "base" | "lg";
  testID?: string;
}

export function Chip({ label, testID }: { label: string; testID?: string }) {
  return (
    // A nested Text takes the size of the text around it. Uniwind resolves
    // an em size by recursing until the stack overflows, so none is set.
    <Text
      testID={testID}
      accessibilityLabel={label}
      className="bg-chip text-chip-foreground rounded-md px-1 font-mono"
    >
      {label}
    </Text>
  );
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

function spec(runs: readonly Run[], prefix: string): ModelRunSpec<ReactNode> {
  const label = codeLabels(runs);
  let i = 0;
  const key = () => `${prefix}-${i++}`;
  return {
    text: (r) => <Text key={key()}>{r.text}</Text>,
    ph: ({ ph }) => <Chip key={key()} label={label.ph(ph)} testID="run-chip" />,
    pcOpen: ({ pcOpen }) => <Chip key={key()} label={label.pcOpen(pcOpen)} testID="run-chip" />,
    pcClose: ({ pcClose }) => <Chip key={key()} label={label.pcClose(pcClose)} testID="run-chip" />,
    sub: ({ sub }) => <Chip key={key()} label={label.sub(sub)} testID="run-chip" />,
    plural: ({ plural }) => {
      const k = key();
      return (
        <Fragment key={k}>
          <Chip label={`{${plural.pivot}}`} testID="run-chip" />
          {branchList(k, plural.forms)}
        </Fragment>
      );
    },
    select: ({ select }) => {
      const k = key();
      return (
        <Fragment key={k}>
          <Chip label={`{${select.pivot}}`} testID="run-chip" />
          {branchList(k, select.cases)}
        </Fragment>
      );
    },
    fallback: (kind) => <Chip key={key()} label={`{${kind}}`} testID="run-chip-unknown" />,
  };
}

function renderRuns(runs: readonly Run[], prefix: string): ReactNode[] {
  return projectRuns(runs, spec(runs, prefix));
}

export function RunText({ runs, size = "base", testID }: RunTextProps) {
  return (
    <Text
      testID={testID}
      className={
        size === "lg" ? "text-foreground text-xl leading-8" : "text-foreground text-base leading-6"
      }
    >
      {renderRuns(runs, "r")}
    </Text>
  );
}
