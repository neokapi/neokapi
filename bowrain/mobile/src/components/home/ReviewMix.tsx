// How a language's waiting translations split over the deck's groups, as the
// server grouped them: quick to approve, worth a closer look, and for the
// desk. A bar shows the shape at a glance; the legend under it gives the
// numbers, and a screen reader reads the legend.
import { Text, View } from "react-native";
import { useTranslation } from "react-i18next";

export interface Mix {
  fast?: number;
  closer?: number;
  desk?: number;
  /** Not grouped yet; the deck groups them when it opens. */
  unclassified?: number;
}

// Spelled out in full so Tailwind sees every class.
const SEGMENTS = [
  { key: "fast", className: "bg-primary" },
  { key: "closer", className: "bg-prism-5" },
  { key: "desk", className: "bg-corrective" },
  { key: "unclassified", className: "bg-border" },
] as const;

/** The legend's text: the groups that hold anything, in deck order. */
export function mixLegend(mix: Mix, t: (key: string, o: { count: number }) => string): string {
  return SEGMENTS.filter((s) => (mix[s.key] ?? 0) > 0)
    .map((s) => t(`home.mix.${s.key}`, { count: mix[s.key] ?? 0 }))
    .join(" · ");
}

export function ReviewMix({
  mix,
  legend = true,
  testID,
}: {
  mix: Mix;
  legend?: boolean;
  testID?: string;
}) {
  const { t } = useTranslation();
  const total = SEGMENTS.reduce((n, s) => n + (mix[s.key] ?? 0), 0);
  if (total === 0) return null;
  const text = mixLegend(mix, t);
  return (
    <View testID={testID} accessible accessibilityLabel={text} className="gap-1.5">
      <View className="bg-muted h-2 flex-row gap-0.5 overflow-hidden rounded-full">
        {SEGMENTS.map((s) => {
          const n = mix[s.key] ?? 0;
          return n > 0 ? (
            <View
              key={s.key}
              testID={testID ? `${testID}-${s.key}` : undefined}
              style={{ flexGrow: n, flexBasis: 0 }}
              className={`${s.className} h-full`}
            />
          ) : null;
        })}
      </View>
      {legend ? (
        <Text className="text-muted-foreground text-xs tabular-nums" numberOfLines={2}>
          {text}
        </Text>
      ) : null}
    </View>
  );
}
