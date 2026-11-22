// What a context card says about the bulk operation that brought its item
// in: the one line on the card and the summary at the top of the origin
// sheet. The server names the operation; these choose the words for it.
import type { ContextCard, ContextOrigin } from "@neokapi/bowrain-client";
import type { TFunction } from "i18next";

import { contextCardId } from "./contextDeck";

/** The line a card shows for its origin: where the item came from. */
export function originLine(t: TFunction, origin: ContextOrigin): string {
  const key = origin.kind === "voice_edit" ? "voice_edit" : (origin.source ?? "import");
  const from = t(`context.origin.line.${key}`, { path: origin.path ?? "" });
  if (origin.kind !== "import" || !origin.count) return from;
  return `${from} · ${t("context.origin.lineCount", { count: origin.count })}`;
}

/** The sentence at the top of the origin sheet: what the operation did. */
export function originSummary(t: TFunction, origin: ContextOrigin): string {
  if (origin.kind === "voice_edit") {
    return t("context.origin.summary.voice_edit", { profile: origin.profile ?? "" });
  }
  switch (origin.source) {
    case "terms":
    case "memory":
    case "decisions":
      return t(`context.origin.summary.${origin.source}`, { count: origin.count ?? 0 });
    case "voice":
      return t("context.origin.summary.voice");
    default:
      return t("context.origin.summary.import");
  }
}

/** Whether a sibling in the origin listing is the card the sheet was opened from. */
export function isCurrentCard(sibling: ContextCard, current: ContextCard | null): boolean {
  return current !== null && contextCardId(sibling) === contextCardId(current);
}
