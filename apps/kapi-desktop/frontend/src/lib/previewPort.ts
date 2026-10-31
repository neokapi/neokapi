// The ports the Bowrain preview draws the project screens through. Each
// reads and never acts: it carries the four reads and no decide, assign, fix
// or check, so the screens draw no control that changes the project.
//
// The local port reads the tab's own project on this Mac. The remote port
// reads the server the recipe's `bowrain:` block connects to, through the
// sign-in on this Mac; the Share page picks it when the recipe names a
// server and a sign-in names that server.

import { useMemo } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import type { ContextActor, ProjectScreensPort, ScreenTarget } from "@neokapi/project-screens";
import { api } from "../hooks/useApi";
import type {
  BowrainPreviewAnswer,
  BowrainPreviewPart,
  PreviewRefusal,
  ShareView,
} from "../types/api";

/** A read that answers null, with no backend behind it, rejects instead. */
async function required<T>(read: Promise<T | null>): Promise<T> {
  const value = await read;
  if (value === null) throw new Error(t("This project could not be read from the desktop"));
  return value;
}

/**
 * A read-only port over the tab's own project, read through the bindings the
 * tab's Map, Reader and Decide read through.
 */
export function localPreviewPort(tabID: string): ProjectScreensPort {
  return {
    mapSummary: () => required(api.mapSummary(tabID)),
    readerFile: (path) => required(api.readerFile(tabID, path)),
    decideView: () => required(api.rulesDecideView(tabID)),
    keptRules: () => required(api.keptRules(tabID)),
  };
}

/** Where a preview reads the project: the Bowrain server, or this Mac. */
export type PreviewSource = "bowrain" | "local";

/**
 * The source a Share page previews: the server when the recipe names one and
 * a sign-in on this Mac names it, this Mac otherwise.
 */
export function previewSource(view: ShareView | null | undefined): PreviewSource {
  return view?.sharing.venue_host && view.signed_in ? "bowrain" : "local";
}

/** A read the server, or this Mac, refused, carrying the refusal as it came. */
export class PreviewRefusalError extends Error {
  readonly refusal: PreviewRefusal;
  constructor(refusal: PreviewRefusal) {
    super(refusal.message ?? "");
    this.name = "PreviewRefusalError";
    this.refusal = refusal;
  }
}

/** One part of the project from the server, or the refusal as an error. */
async function fromBowrain<K extends Exclude<keyof BowrainPreviewAnswer, "refusal">>(
  tabID: string,
  part: BowrainPreviewPart,
  key: K,
  item = "",
): Promise<NonNullable<BowrainPreviewAnswer[K]>> {
  const answer = await api.bowrainPreview(tabID, part, item);
  if (answer?.refusal) throw new PreviewRefusalError(answer.refusal);
  const value = answer?.[key];
  if (value === undefined || value === null) {
    throw new Error(t("This project could not be read from Bowrain"));
  }
  return value as NonNullable<BowrainPreviewAnswer[K]>;
}

/**
 * A read-only port over the tab's project as the Bowrain server shows it.
 * The server knows a file by its item name, which the Map's rows carry, and
 * the Reader is opened by the file's path, so the port keeps the names the
 * Map reads carried, reads the Map first when it has none for a path, and
 * opens a path the Map did not list by that path. A person
 * the server leaves unnamed is the one signed in, shown by `signedInAs`.
 */
export function remotePreviewPort(tabID: string, signedInAs?: string): ProjectScreensPort {
  const itemOf = new Map<string, string>();
  let mapped: Promise<unknown> | null = null;
  const readMap = async () => {
    const summary = await fromBowrain(tabID, "map", "map");
    for (const row of summary.file_rows ?? []) itemOf.set(row.relative, row.path);
    return summary;
  };
  return {
    mapSummary: () => {
      const read = readMap();
      mapped = read;
      return read;
    },
    readerFile: async (path) => {
      // A Reader opened before any Map read has returned, from Decide say,
      // waits for one to learn the item's name.
      if (!itemOf.has(path)) {
        mapped ??= readMap();
        await mapped.catch(() => undefined);
      }
      return fromBowrain(tabID, "reader", "reader", itemOf.get(path) ?? path);
    },
    decideView: () => fromBowrain(tabID, "decide", "decide"),
    keptRules: () => fromBowrain(tabID, "kept", "kept"),
    actorName: (actor: ContextActor) =>
      actor.name || (actor.kind === "person" && signedInAs ? signedInAs : undefined),
  };
}

/**
 * The port a Share page previews through and where it reads: the remote port
 * when `previewSource` picks the server, the local port otherwise. The same
 * object while the tab, the source and the signed-in name are.
 */
export function usePreviewPort(
  tabID: string,
  view: ShareView | null | undefined,
): { port: ProjectScreensPort; source: PreviewSource } {
  const source = previewSource(view);
  const signedInAs = view?.signed_in_as;
  const port = useMemo(
    () => (source === "bowrain" ? remotePreviewPort(tabID, signedInAs) : localPreviewPort(tabID)),
    [tabID, source, signedInAs],
  );
  return { port, source };
}

/** The targets a preview opens: the Map, the Reader and the Rules tab. */
export function previewOpens(target: ScreenTarget): boolean {
  switch (target.kind) {
    case "map":
    case "reader":
    case "rules":
    case "kept":
      return true;
    default:
      return false;
  }
}
