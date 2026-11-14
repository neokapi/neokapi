// The pieces a feed card's preview is made of: chips, a split bar and a
// progress row. Colours come from the theme tokens; the prism ramp fills
// progress, sparingly.
import type { ReactNode } from "react";
import { Pressable, Text, View } from "react-native";

export type ChipTone = "neutral" | "added" | "removed" | "accent";

const CHIP_TONES: Record<ChipTone, string> = {
  neutral: "bg-muted",
  added: "bg-added",
  removed: "bg-removed",
  accent: "bg-chip",
};

const CHIP_TEXT: Record<ChipTone, string> = {
  neutral: "text-foreground",
  added: "text-foreground",
  removed: "text-foreground",
  accent: "text-chip-foreground",
};

/** A short fact in a rounded pill. With onPress it opens what it names. */
export function Chip({
  children,
  tone = "neutral",
  leading,
  onPress,
  testID,
}: {
  children: ReactNode;
  tone?: ChipTone;
  leading?: ReactNode;
  onPress?: () => void;
  testID?: string;
}) {
  const body = (
    <View className={`${CHIP_TONES[tone]} flex-row items-center gap-1 rounded-full px-3 py-1.5`}>
      {leading}
      <Text className={`${CHIP_TEXT[tone]} text-[13px] tabular-nums`} numberOfLines={1}>
        {children}
      </Text>
    </View>
  );
  if (!onPress) return body;
  return (
    <Pressable
      testID={testID}
      accessibilityRole="link"
      hitSlop={4}
      onPress={onPress}
      className="active:opacity-60"
    >
      {body}
    </Pressable>
  );
}

export function ChipRow({ children }: { children: ReactNode }) {
  return <View className="flex-row flex-wrap gap-2">{children}</View>;
}

/** One segment of a split bar. */
export interface Segment {
  key: string;
  value: number;
  /** A bg-* class. */
  colour: string;
  label: string;
}

/** A bar split in proportion to its segments, with a legend below it. */
export function SplitBar({ segments }: { segments: Segment[] }) {
  const shown = segments.filter((s) => s.value > 0);
  if (shown.length === 0) return null;
  return (
    <View className="gap-2">
      <View className="bg-muted h-2.5 flex-row overflow-hidden rounded-full">
        {shown.map((s) => (
          <View key={s.key} className={`${s.colour} h-full`} style={{ flex: s.value }} />
        ))}
      </View>
      <View className="flex-row flex-wrap gap-x-4 gap-y-1">
        {shown.map((s) => (
          <View key={s.key} className="flex-row items-center gap-1.5">
            <View className={`${s.colour} h-2 w-2 rounded-full`} />
            <Text className="text-muted-foreground text-[13px] tabular-nums">
              {s.label} {s.value}
            </Text>
          </View>
        ))}
      </View>
    </View>
  );
}

// Spelled out in full so Tailwind sees every class.
export const PRISM_FILLS = ["bg-prism-4", "bg-prism-3", "bg-prism-5", "bg-prism-2", "bg-prism-1"];

/** A labelled progress bar: the label, a fill, and a value at the end. */
export function ProgressRow({
  label,
  fraction,
  value,
  fill,
  onPress,
}: {
  label: string;
  fraction: number;
  value: string;
  fill: string;
  onPress?: () => void;
}) {
  const pct = Math.max(0.04, Math.min(1, fraction));
  const row = (
    <View className="gap-1.5">
      <View className="flex-row items-baseline justify-between gap-3">
        <Text className="text-foreground flex-1 text-[14px]" numberOfLines={1}>
          {label}
        </Text>
        <Text className="text-muted-foreground text-[13px] tabular-nums">{value}</Text>
      </View>
      <View className="bg-muted h-2 overflow-hidden rounded-full">
        <View className={`${fill} h-full rounded-full`} style={{ width: `${pct * 100}%` }} />
      </View>
    </View>
  );
  if (!onPress) return row;
  return (
    <Pressable accessibilityRole="link" onPress={onPress} className="active:opacity-60">
      {row}
    </Pressable>
  );
}
