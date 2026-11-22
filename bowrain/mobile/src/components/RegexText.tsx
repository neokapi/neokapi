// A rule's regex with the words it looks for in bold and the syntax around
// them receding, so a reader finds the words first. Escapes and character
// sets, which stand for characters, take the primary colour.
import { Text } from "react-native";

import { type RegexTokenKind, regexTokens } from "../lib/regexTokens";

const TOKEN_CLASS: Record<RegexTokenKind, string> = {
  literal: "text-foreground font-semibold",
  escape: "text-primary",
  set: "text-primary",
  group: "text-muted-foreground",
  alternative: "text-muted-foreground",
  repeat: "text-muted-foreground",
  anchor: "text-muted-foreground",
};

export function RegexText({ source, testID }: { source: string; testID?: string }) {
  return (
    <Text testID={testID} selectable className="font-mono text-base">
      {regexTokens(source).map((tok, i) => (
        <Text key={i} className={TOKEN_CLASS[tok.kind]}>
          {tok.text}
        </Text>
      ))}
    </Text>
  );
}
