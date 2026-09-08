import type { CardsSpec, DemoManifest } from "../types.ts";

/** Where the outro points when the demo names no pointer: the brand's site. */
export function defaultPointer(brand: string | undefined): string {
  return brand === "bowrain" ? "bowrain.cloud" : "neokapi.github.io";
}

/** The two cards' text for a demo, with the outro's defaults filled in. */
export function cardsFor(m: Pick<DemoManifest, "title" | "subtitle" | "brand" | "outro">): CardsSpec {
  return {
    title: m.title,
    subtitle: m.subtitle ?? "",
    outroLine: m.outro?.line?.trim() || m.title,
    outroPointer: m.outro?.pointer?.trim() || defaultPointer(m.brand),
  };
}
