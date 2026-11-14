// The header options every tab's stack shares.
import { useCSSVariable } from "uniwind";

export function useStackOptions() {
  // The transparent iOS header keeps a dark title in dark mode unless it is
  // given the foreground colour.
  const foreground = String(useCSSVariable("--color-foreground"));
  return {
    headerBackButtonDisplayMode: "minimal" as const,
    headerTitleStyle: { color: foreground },
    headerLargeTitleStyle: { color: foreground },
  };
}

/** A tab's first screen: a large title, as iOS draws one. */
export const tabRoot = { headerLargeTitle: true } as const;
