// A fix shown against the draft it changes: added words marked green, removed
// words struck through. Reads the server's word diff when the fix carries
// one, or a diff the client computes between two run sequences.
import { diffRuns, type DiffToken, type FixDiff } from "@neokapi/bowrain-client";
import type { Run } from "@neokapi/kapi-format";
import { Text, View } from "react-native";

import { Chip } from "./RunText";

function tokenNode(t: DiffToken, key: string, className?: string) {
  if (t.kind === "text") {
    return (
      <Text key={key} className={className}>
        {t.text}
      </Text>
    );
  }
  return <Chip key={key} label={t.label} />;
}

/** The diff between a draft and a fix, both as runs. */
export function RunDiff({ before, after, testID }: { before: readonly Run[]; after: readonly Run[]; testID?: string }) {
  const spans = diffRuns(before, after);
  return (
    <Text testID={testID} className="text-foreground text-base leading-6">
      {spans.map((s, i) => {
        if (s.op === "same") return tokenNode(s.token, `${i}`);
        if (s.op === "add") return tokenNode(s.token, `${i}`, "bg-added");
        return tokenNode(s.token, `${i}`, "bg-removed line-through");
      })}
    </Text>
  );
}

/** The server's word diff of a fix candidate: the fix, then the draft. */
export function ServerDiff({ diff, testID }: { diff: FixDiff; testID?: string }) {
  return (
    <View testID={testID} className="gap-1">
      <Text className="text-foreground text-base leading-6">
        {diff.current.map((s, i) => (
          <Text key={i} className={s.op === "added" ? "bg-added" : undefined}>
            {s.text}
          </Text>
        ))}
      </Text>
      <Text className="text-muted-foreground text-sm leading-5">
        {diff.prior.map((s, i) => (
          <Text key={i} className={s.op === "removed" ? "bg-removed line-through" : undefined}>
            {s.text}
          </Text>
        ))}
      </Text>
    </View>
  );
}
