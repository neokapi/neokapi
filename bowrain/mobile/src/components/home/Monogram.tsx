// A workspace's or a project's mark: its initials on a gradient picked from
// its name, so the same project keeps the same colours on every screen and
// on every device. The mark is decoration beside the name, so screen readers
// skip it.
import { initialsOf, paletteIndex } from "@neokapi/bowrain-client";
import { Text, View } from "react-native";

// Spelled out in full so Tailwind sees every class. Each pair starts on a
// colour dark enough to carry white initials.
export const MONOGRAM_GRADIENTS = [
  "from-prism-4 to-prism-5",
  "from-prism-1 to-prism-5",
  "from-prism-3 to-prism-4",
  "from-prism-5 to-prism-1",
  "from-primary to-prism-3",
  "from-prism-1 to-prism-2",
] as const;

/** The gradient a name keeps. */
export function monogramGradient(seed: string): string {
  return MONOGRAM_GRADIENTS[paletteIndex(seed, MONOGRAM_GRADIENTS.length)];
}

export function Monogram({
  name,
  seed = name,
  size = 44,
  testID,
}: {
  name: string;
  /** What picks the colours; the name when absent. A stable id keeps them through a rename. */
  seed?: string;
  size?: number;
  testID?: string;
}) {
  return (
    <View
      testID={testID}
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants"
      style={{ width: size, height: size, borderRadius: Math.round(size * 0.28) }}
      className={`${monogramGradient(seed)} items-center justify-center bg-linear-to-br`}
    >
      {/* The initials fill a fixed tile, so they keep its scale rather than
          the reader's text size. */}
      <Text
        allowFontScaling={false}
        style={{ fontSize: Math.round(size * 0.4) }}
        className="font-bold text-white"
      >
        {initialsOf(name)}
      </Text>
    </View>
  );
}
