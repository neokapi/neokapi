// The one way the app draws an inline code. It takes a CodeDisplay, which
// only codeDisplays (@neokapi/kapi-format) makes, so a code's native data or
// an extractor's internal identifier cannot reach the screen through it: it
// shows the code's human name, or words its kind.
import type { CodeDisplay, CodeKind } from "@neokapi/kapi-format";
import { Text } from "react-native";
import { useTranslation } from "react-i18next";

/** The words for a code with no human name, as the catalog holds them. */
export function useCodeWord(): (d: CodeDisplay) => string {
  const { t } = useTranslation();
  return (d) => d.name ?? t(`code.kind.${d.kind satisfies CodeKind}`);
}

/**
 * A code as it sits in a string: its name or word in a monospace face and a
 * muted ink, so it reads as part of the sentence and stands apart from the
 * words being translated. It is not tappable: a tap in a string opens a term
 * or contextual help, and a code is neither.
 */
export function CodeChip({ display, testID }: { display: CodeDisplay; testID?: string }) {
  const word = useCodeWord()(display);
  return (
    // A nested Text takes the size of the text around it. Uniwind resolves
    // an em size by recursing until the stack overflows, so none is set.
    <Text
      testID={testID}
      accessibilityLabel={word}
      className={`text-muted-foreground font-mono${display.role === "close" ? " opacity-70" : ""}`}
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
