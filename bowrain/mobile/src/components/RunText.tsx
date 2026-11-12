// A run sequence drawn as text, with each placeholder and inline code as a
// chip. The projection is a RunSpec over every run kind, so no kind can be
// dropped without saying so; a plural or select shows every form, labelled.
import { projectRuns, type ModelRunSpec, type Run } from "@neokapi/kapi-format";
import { Fragment, type ReactNode } from "react";
import { Text } from "react-native";

export interface RunTextProps {
  runs: readonly Run[];
  /** Larger text for the card's main reading. */
  size?: "base" | "lg";
  testID?: string;
}

/** The label a code chip shows: what a person reads for it, never its native data. */
export function codeLabel(code: { id: string; equiv?: string; disp?: string }): string {
  return code.disp || code.equiv || code.id;
}

export function Chip({ label, testID }: { label: string; testID?: string }) {
  return (
    <Text
      testID={testID}
      accessibilityLabel={label}
      className="bg-chip text-chip-foreground rounded-md px-1 font-mono text-[0.9em]"
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

function spec(prefix: string): ModelRunSpec<ReactNode> {
  let i = 0;
  const key = () => `${prefix}-${i++}`;
  return {
    text: (r) => <Text key={key()}>{r.text}</Text>,
    ph: ({ ph }) => <Chip key={key()} label={codeLabel(ph)} testID="run-chip" />,
    pcOpen: ({ pcOpen }) => <Chip key={key()} label={`<${codeLabel(pcOpen)}>`} testID="run-chip" />,
    pcClose: ({ pcClose }) => (
      <Chip key={key()} label={`</${codeLabel({ id: pcClose.id, equiv: pcClose.equiv })}>`} testID="run-chip" />
    ),
    sub: ({ sub }) => <Chip key={key()} label={`[${codeLabel(sub)}]`} testID="run-chip" />,
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
  return projectRuns(runs, spec(prefix));
}

export function RunText({ runs, size = "base", testID }: RunTextProps) {
  return (
    <Text
      testID={testID}
      className={size === "lg" ? "text-foreground text-xl leading-8" : "text-foreground text-base leading-6"}
    >
      {renderRuns(runs, "r")}
    </Text>
  );
}
