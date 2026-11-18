// The one way the app draws an inline code. It takes a CodeDisplay, which
// only codeDisplays (@neokapi/kapi-format) makes, so a code's native data or
// an extractor's internal identifier cannot reach the screen through it: a
// chip shows the code's human name, or words its kind.
import type { CodeDisplay, CodeKind } from "@neokapi/kapi-format";
import { Text } from "react-native";
import { useTranslation } from "react-i18next";

/** The words for a code with no human name, as the catalog holds them. */
export function useCodeWord(): (d: CodeDisplay) => string {
  const { t } = useTranslation();
  return (d) => d.name ?? t(`code.kind.${d.kind satisfies CodeKind}`);
}

const SHAPE: Record<CodeDisplay["role"], string> = {
  placeholder: "rounded-md",
  choice: "rounded-md",
  reference: "rounded-md",
  // The two halves of a pair face each other.
  open: "rounded-l-md rounded-r-sm",
  close: "rounded-r-md rounded-l-sm opacity-70",
};

export function CodeChip({ display, testID }: { display: CodeDisplay; testID?: string }) {
  const word = useCodeWord()(display);
  return (
    // A nested Text takes the size of the text around it. Uniwind resolves
    // an em size by recursing until the stack overflows, so none is set.
    <Text
      testID={testID}
      accessibilityLabel={word}
      className={`bg-chip text-chip-foreground px-1 ${SHAPE[display.role]}`}
    >
      {word}
    </Text>
  );
}

/** Text a person may not change (marked not for translation), locked in the editor. */
export function LockedText({ text, testID }: { text: string; testID?: string }) {
  return (
    <Text testID={testID} className="bg-muted text-foreground rounded-md px-1">
      {text}
    </Text>
  );
}
