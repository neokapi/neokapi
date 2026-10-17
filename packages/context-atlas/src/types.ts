// The atlas contract (Apache-2.0): what `kapi context atlas --json` and
// host.App.ContextAtlas return. The shapes mirror the Go structs field for
// field, snake_case on the wire, instants as RFC 3339 strings.

/** Who recorded an operation. Mirrors contextop.Actor. */
export interface AtlasActor {
  /** `person`, `agent` or `tool`. */
  kind: string;
  /** A person's name or handle, an agent's client name, a tool's name. */
  name?: string;
  /** Groups the operations one agent run recorded. */
  session?: string;
}

/** Where a subject was seen. Mirrors contextop.Evidence. */
export interface AtlasEvidence {
  /** Project-relative, slash-separated. */
  path?: string;
  /** The block or unit key inside the file. */
  unit?: string;
  /** The text the subject was seen in. */
  quote?: string;
  /** A web page the subject was seen on. */
  url?: string;
}

/** The earliest and latest instant across records and events. */
export interface AtlasSpan {
  from: string;
  to: string;
}

/** How an axis's values compare. */
export type AtlasAxisKind = "category" | "geo" | "range" | "ordered";

/** Where an axis comes from. */
export type AtlasAxisSource = "structural" | "declared" | "derived";

/** One value of an axis, in display order. On a geo axis the id is a place id. */
export interface AtlasValue {
  id: string;
  label?: string;
}

/** What kind of place a place is. */
export type AtlasPlaceType =
  | "world"
  | "region"
  | "country"
  | "subdivision"
  | "territory"
  | "metro"
  | "locality"
  | "district"
  | "neighbourhood"
  | "postal-area"
  | "area";

/** A GeoJSON polygon or multipolygon, `[lon, lat]` positions. */
export type AtlasGeometry =
  | { type: "Polygon"; coordinates: number[][][] }
  | { type: "MultiPolygon"; coordinates: number[][][][] };

/**
 * One place of the project's gazetteer: a built-in place (the world, a UN M49
 * region, an ISO 3166 country or subdivision) or one the recipe declares.
 * Mirrors host.AtlasPlace.
 */
export interface AtlasPlace {
  /** `world`, `northern-europe`, `no`, `no-03`, `oslo`. */
  id: string;
  /** English display name. */
  name: string;
  /** Unknown types arrive as strings. */
  type: AtlasPlaceType | (string & {});
  /** The places it sits within. */
  within?: string[];
  /** The places it is made of. */
  of?: string[];
  /** `[lat, lon]`. */
  centroid?: [number, number];
  /** `wikidata:Q585`, `geonames:3143244`, `nuts:NO081`, `osm:relation/406091`, `wof:101752073`. */
  refs?: string[];
  /** Display names by BCP 47 locale, for a declared place. */
  names?: Record<string, string>;
  /** A declared place's shape file, project-relative. */
  shape?: string;
  /** That file's polygon, when it could be read. */
  geometry?: AtlasGeometry;
  /** Whether the recipe declares the place. */
  declared?: boolean;
}

/** One axis of the context space. */
export interface AtlasAxis {
  /** `product`, `channel`, `brand`, `market`, `locale`, `region`, `family`, `actor`, `project`, ... */
  id: string;
  /** The recipe's label; empty means the UI names it. */
  label?: string;
  /** Unknown kinds arrive as strings and read as `category`. */
  kind: AtlasAxisKind | (string & {});
  source: AtlasAxisSource | (string & {});
  /** Display order; every value any record uses. */
  values: AtlasValue[];
}

/** A state a record held from `at` onwards. */
export interface AtlasState {
  at: string;
  state: string;
}

/** One thing the context holds: a rule, a note, a memory pair, a term or a content cell. */
export interface AtlasRecord {
  /** Op id, `term:<concept>`, `content:<collection>:<locale>:<state>`, ... */
  id: string;
  /** `rule`, `note`, `memory`, `term`, `content` or `import`. */
  family: string;
  /** The phrase a person reads. */
  label: string;
  /** 1, or the unit count of a content cell. */
  weight: number;
  /** Every static axis value, derived ones included. */
  coordinates: Record<string, string>;
  /** When it entered the record; the zero instant when unknown. */
  recorded: string;
  valid_from?: string;
  valid_to?: string;
  /** Time order. The state at t is the last entry with `at <= t`; none yet is `absent`. */
  history: AtlasState[];
  /** Op short id, for the CLI chip. */
  short?: string;
  actor?: AtlasActor;
  evidence?: AtlasEvidence[];
  /** Note prose, or the observer's words. */
  text?: string;
  /** contextop.Scope.Describe(). */
  scope?: string;
  contested_by?: string[];
}

/** Something that happened on a clock. */
export interface AtlasEvent {
  id: string;
  /** The recorded clock. */
  at: string;
  /** The effective clock, when it differs. */
  effective?: string;
  /** An op kind, `window-opens`, `window-closes` or `decision`. */
  kind: string;
  /** The record it bears on. */
  record?: string;
  actor?: AtlasActor;
  label?: string;
}

/** The whole atlas answer. */
export interface AtlasData {
  /** `kapi-context-atlas`. */
  format: string;
  /** 1. */
  version: number;
  project?: string;
  generated_at: string;
  span: AtlasSpan;
  axes: AtlasAxis[];
  /**
   * Every place a geo coordinate names, with what contains it up to the
   * world, the members of each place made of others, every declared place,
   * and the direct children of each country and subdivision. Sorted by id.
   */
  places: AtlasPlace[];
  records: AtlasRecord[];
  events: AtlasEvent[];
  /** Reach, truncation, what was not read. */
  notes: string[];
}

/** The format and version this package reads. */
export const ATLAS_FORMAT = "kapi-context-atlas";
export const ATLAS_VERSION = 1;
