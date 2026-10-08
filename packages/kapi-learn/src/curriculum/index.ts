// The curriculum: three series, eleven labs, one sample project each.
//
// Pure data. The player (../player) runs it in the browser, the verifier
// (scripts/learn-verify) runs it in Node, and the docs site reads it at build
// time to make one page per lab. Keep it free of React and engine imports.

import type { Lab, SampleId, SampleInfo, Series, SeriesId } from "./types.ts";
import { NORTHSEA_LABS } from "./labs/northsea.ts";
import { COMPASS_LABS } from "./labs/compass.ts";
import { MART_LABS } from "./labs/mart.ts";

export type {
  Chapter,
  ChapterLook,
  Lab,
  LabFile,
  SampleId,
  SampleInfo,
  Series,
  SeriesId,
} from "./types.ts";

export const SERIES: readonly Series[] = [
  {
    id: "start-here",
    title: "Start here: one language",
    tagline: "Govern the content you already have.",
    description:
      "A company repository in one language: operator documentation, interface strings and a marketing page. Read the project's context in, ask what applies to a file, run the checks like tests, record a decision, and edit a block through the engine. No second language, no server, no credential.",
    sample: "northsea",
  },
  {
    id: "add-languages",
    title: "Add languages",
    tagline: "The loop produces, review promotes, the gate releases.",
    description:
      "The same content with three languages added. Watch kapi up recycle what the project established and draft the rest, read where every language stands, approve a batch of translations as a change set, and see the manifest a deployed page's language picker follows.",
    sample: "compass",
  },
  {
    id: "engine",
    title: "The content engine",
    tagline: "Formats, tools, flows and stores, one command at a time.",
    description:
      "A storefront's catalog and prose, through the engine's own verbs: inspect the model behind any format, run tools and flows with every prompt in the open, then set up a project that remembers its memory and terms and holds a draft to them.",
    sample: "mart",
  },
];

export const SAMPLES: Record<SampleId, SampleInfo> = {
  northsea: {
    id: "northsea",
    name: "Northsea",
    blurb:
      "Northsea Maritime Systems sells software to port and fleet operators: Compass, the berth plan, and Tidewatch, the alerting service. One repository, one language, three surfaces.",
    source: "samples/northsea",
  },
  compass: {
    id: "compass",
    name: "Compass",
    blurb:
      "The Compass interface, shipped in English at four North Sea terminals, with operators who read Norwegian, German and Dutch, and a page whose language picker reads what the loop ships.",
    source: "samples/compass",
  },
  mart: {
    id: "mart",
    name: "KapiMart",
    blurb:
      "A storefront and order platform for small retailers: an interface catalog, an About page, three locales at different stages, its terms and a content memory.",
    source: "samples/mart",
  },
};

export const LABS: readonly Lab[] = [...NORTHSEA_LABS, ...COMPASS_LABS, ...MART_LABS];

const BY_ID = new Map(LABS.map((lab) => [lab.id, lab]));

export function labById(id: string): Lab | undefined {
  return BY_ID.get(id);
}

export function labsInSeries(series: SeriesId): Lab[] {
  return LABS.filter((lab) => lab.series === series).sort((a, b) => a.position - b.position);
}

export function seriesById(id: SeriesId): Series {
  const found = SERIES.find((s) => s.id === id);
  if (!found) throw new Error(`unknown series ${id}`);
  return found;
}

/** The lab after this one in the curriculum's order, crossing into the next series. */
export function nextLab(id: string): Lab | undefined {
  const i = LABS.findIndex((lab) => lab.id === id);
  return i >= 0 ? LABS[i + 1] : undefined;
}

export function previousLab(id: string): Lab | undefined {
  const i = LABS.findIndex((lab) => lab.id === id);
  return i > 0 ? LABS[i - 1] : undefined;
}

/** The number of chapters that run a command, for an index card. */
export function commandCount(lab: Lab): number {
  return lab.chapters.filter((c) => c.command).length;
}
