// Who did something, as a mark a reader recognises before reading the name:
// a person's initials in a colour that stays theirs, an agent's spark, and a
// tool's gear.
import { initialsOf, paletteIndex, type FeedActor } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Text, View } from "react-native";
import { useCSSVariable } from "uniwind";

// Spelled out in full so Tailwind sees every class.
const PERSON_COLOURS = [
  "bg-prism-1",
  "bg-prism-2",
  "bg-prism-3",
  "bg-prism-4",
  "bg-prism-5",
  "bg-primary",
  "bg-corrective",
  "bg-chip-foreground",
];

export function Avatar({
  actor,
  name,
  size = 40,
}: {
  actor: FeedActor;
  name: string;
  size?: number;
}) {
  const muted = useCSSVariable("--color-muted-foreground");
  const box = { width: size, height: size };
  if (actor.kind === "agent") {
    return (
      <View
        accessibilityElementsHidden
        importantForAccessibility="no"
        style={box}
        className="from-prism-4 to-prism-5 items-center justify-center rounded-2xl bg-linear-to-br"
      >
        <Ionicons name="sparkles" size={size * 0.5} color="#ffffff" />
      </View>
    );
  }
  if (actor.kind === "tool") {
    return (
      <View
        accessibilityElementsHidden
        importantForAccessibility="no"
        style={box}
        className="bg-muted border-border items-center justify-center rounded-full border"
      >
        <Ionicons name="cog" size={size * 0.55} color={String(muted)} />
      </View>
    );
  }
  const colour = PERSON_COLOURS[paletteIndex(actor.id || name, PERSON_COLOURS.length)];
  return (
    <View
      accessibilityElementsHidden
      importantForAccessibility="no"
      style={box}
      className={`${colour} items-center justify-center rounded-full`}
    >
      <Text className="text-[15px] font-semibold text-white">{initialsOf(name)}</Text>
    </View>
  );
}
