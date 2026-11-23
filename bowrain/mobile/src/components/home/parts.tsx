// The small pieces the Home screens share: the count pill, the locale badge,
// the section header, the empty state and the skeleton shown while a list
// loads.
import Ionicons from "@expo/vector-icons/Ionicons";
import { type ComponentProps, useEffect, useRef, useState } from "react";
import {
  AccessibilityInfo,
  Animated,
  Pressable,
  Text,
  useWindowDimensions,
  View,
} from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { formatCount } from "../../lib/format";

/** The text scale from which a fixed-height row stacks its parts and lets its lines wrap. */
export const LARGE_TEXT = 1.6;

/**
 * Whether the text is at a large accessibility size, and how many lines a
 * wrapping line of small text is given there. The lines grow with the
 * scale, so a line such as a project's facts fits at every large size.
 */
export function useLargeText(): { large: boolean; lines: number } {
  const { fontScale } = useWindowDimensions();
  return { large: fontScale >= LARGE_TEXT, lines: Math.min(4, Math.round(fontScale + 0.5)) };
}

/** The line height, before scaling, of each text size a fixed-lines text takes. */
const LINE_HEIGHT = { sm: 20, xs: 16 } as const;

/**
 * Text that takes exactly `lines` lines whatever it says, so a row keeps its
 * height when "Loading" gives way to what loaded: cut to that many lines, and
 * never shorter. The system scales the line height with the text, so the
 * minimum height is scaled the same way.
 */
export function FixedLinesText({
  lines,
  size,
  className,
  testID,
  children,
}: {
  lines: number;
  size: keyof typeof LINE_HEIGHT;
  className: string;
  testID?: string;
  children?: string;
}) {
  const { fontScale } = useWindowDimensions();
  const lineHeight = LINE_HEIGHT[size];
  return (
    <Text
      testID={testID}
      className={className}
      style={{ lineHeight, minHeight: lines * lineHeight * fontScale }}
      numberOfLines={lines}
    >
      {children}
    </Text>
  );
}

/** In place of the count when it could not be loaded: never a zero or a check. */
export function UnknownPill({ testID }: { testID?: string }) {
  const muted = useCSSVariable("--color-muted-foreground");
  return (
    <View
      testID={testID}
      importantForAccessibility="no-hide-descendants"
      className="h-7 w-9 items-center justify-center"
    >
      <Ionicons name="cloud-offline-outline" size={20} color={String(muted)} />
    </View>
  );
}

/**
 * Below a list whose counts could not be loaded: says so, with a retry. It
 * sits after the cards, so it moves nothing when it goes.
 */
export function CountsNotice({
  kind = "failed",
  onRetry,
}: {
  /** `failed`: the counts were never loaded; `stale`: they could not be refreshed. */
  kind?: "failed" | "stale";
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  const text = kind === "stale" ? t("home.countsStale") : t("home.countsFailed");
  return (
    <View testID={`counts-${kind}`} className="items-center gap-2 px-8 pt-4 pb-2">
      <Text className="text-muted-foreground text-center text-sm">{text}</Text>
      <Pressable testID="counts-retry" accessibilityRole="button" onPress={onRetry} hitSlop={8}>
        <Text className="text-primary text-base">{t("common.retry")}</Text>
      </Pressable>
    </View>
  );
}

/** A count of what waits, or a check when nothing does. */
export function CountPill({ count, testID }: { count: number; testID?: string }) {
  const { i18n } = useTranslation();
  const approve = useCSSVariable("--color-approve");
  if (count <= 0) {
    return (
      <View testID={testID} className="h-7 w-7 items-center justify-center">
        <Ionicons name="checkmark-circle" size={22} color={String(approve)} />
      </View>
    );
  }
  return (
    <View testID={testID} className="bg-primary min-w-9 items-center rounded-full px-2.5 py-1">
      <Text className="text-primary-foreground text-sm font-semibold tabular-nums">
        {formatCount(count, i18n.language)}
      </Text>
    </View>
  );
}

/**
 * A language as its code, in a badge: "NB", "PT-BR". Flags name countries,
 * not languages, so the badge carries the code alone.
 */
export function LocaleBadge({
  locale,
  active = true,
  unknown = false,
  size = "md",
}: {
  locale: string;
  /** False dims the badge: nothing waits in this language. */
  active?: boolean;
  /**
   * What waits is not known: an outlined badge, neither lit nor dimmed, so it
   * does not read as "nothing waits".
   */
  unknown?: boolean;
  size?: "sm" | "md";
}) {
  const box = size === "sm" ? "min-w-8 px-1.5 py-0.5 rounded-md" : "min-w-11 px-2 py-2 rounded-xl";
  const text = size === "sm" ? "text-[11px]" : "text-sm";
  const look = unknown
    ? "border-border border border-dashed bg-transparent"
    : active
      ? "bg-chip"
      : "bg-muted";
  const ink = unknown
    ? "text-foreground"
    : active
      ? "text-chip-foreground"
      : "text-muted-foreground";
  return (
    <View
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants"
      testID={unknown ? `badge-${locale}-unknown` : undefined}
      className={`${box} items-center ${look}`}
    >
      <Text className={`${text} font-bold tracking-wide ${ink}`}>
        {locale.replace(/_/g, "-").toUpperCase()}
      </Text>
    </View>
  );
}

export function SectionHeader({ title, testID }: { title: string; testID?: string }) {
  return (
    <Text
      testID={testID}
      accessibilityRole="header"
      className="text-foreground px-5 pt-6 pb-2 text-xl font-bold"
    >
      {title}
    </Text>
  );
}

type IconName = ComponentProps<typeof Ionicons>["name"];

/** A list with nothing in it: an icon, a title and a line under it. */
export function EmptyState({
  icon,
  title,
  body,
  testID,
}: {
  icon: IconName;
  title: string;
  body?: string;
  testID?: string;
}) {
  const muted = useCSSVariable("--color-muted-foreground");
  return (
    <View testID={testID} className="items-center gap-2 px-8 pt-12 pb-8">
      <View className="bg-muted mb-2 h-20 w-20 items-center justify-center rounded-full">
        <Ionicons name={icon} size={36} color={String(muted)} />
      </View>
      <Text className="text-foreground text-center text-lg font-semibold">{title}</Text>
      {body ? <Text className="text-muted-foreground text-center text-base">{body}</Text> : null}
    </View>
  );
}

/** True while the person has asked the system to reduce motion. */
function useReduceMotion(): boolean {
  const [reduce, setReduce] = useState(false);
  useEffect(() => {
    let live = true;
    void AccessibilityInfo.isReduceMotionEnabled?.()
      .then((v) => live && setReduce(v))
      .catch(() => undefined);
    const sub = AccessibilityInfo.addEventListener?.("reduceMotionChanged", setReduce);
    return () => {
      live = false;
      sub?.remove();
    };
  }, []);
  return reduce;
}

/**
 * Placeholder cards in the shape of the list that is loading. They pulse
 * slowly, and hold still when the system asks for reduced motion.
 */
export function SkeletonCards({ count = 3, testID }: { count?: number; testID?: string }) {
  const { t } = useTranslation();
  const reduce = useReduceMotion();
  const pulse = useRef(new Animated.Value(1)).current;
  useEffect(() => {
    if (reduce) return;
    const loop = Animated.loop(
      Animated.sequence([
        Animated.timing(pulse, { toValue: 0.5, duration: 700, useNativeDriver: true }),
        Animated.timing(pulse, { toValue: 1, duration: 700, useNativeDriver: true }),
      ]),
    );
    loop.start();
    return () => loop.stop();
  }, [pulse, reduce]);
  return (
    <Animated.View
      testID={testID}
      accessible
      accessibilityRole="progressbar"
      accessibilityLabel={t("home.loading")}
      style={{ opacity: pulse }}
    >
      <View className="gap-3 px-4 pt-2">
        {Array.from({ length: count }, (_, i) => (
          <View
            key={i}
            className="bg-card border-border flex-row items-center gap-3 rounded-3xl border p-4"
          >
            <View className="bg-muted h-11 w-11 rounded-xl" />
            <View className="flex-1 gap-2">
              <View className="bg-muted h-4 w-2/3 rounded-full" />
              <View className="bg-muted h-3 w-1/3 rounded-full" />
            </View>
          </View>
        ))}
      </View>
    </Animated.View>
  );
}
