// The large accessibility text sizes: the one scale from which a screen or a
// sheet stacks what it drew side by side, and lets its lines wrap.
import { useWindowDimensions } from "react-native";

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
