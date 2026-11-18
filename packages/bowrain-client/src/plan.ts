// The review plan: what a project asks of its reviewers, ordered by the reach
// of each decision (GET /:ws/:id/review/plan). Coordinates first, then the key
// language, then exceptions, then interfaces read in context, and last what
// the involvement in force leaves to the checks. The server counts; this file
// maps its answer.

/** How much a project's reviewers can be involved. */
export type ReviewInvolvement = "context" | "interfaces" | "every";

/** A level of the plan, in the order a reviewer works it. */
export type PlanLevel = "coordinates" | "key_language" | "exceptions" | "interfaces" | "policy";

/** One place in the project's context space: a collection's coordinates and its voice. */
export interface PlanPoint {
  collection: string;
  coordinates: Record<string, string>;
  voice?: string;
  sourceOnly: boolean;
  priority?: string;
}

/** The plan's part for one target language the caller may review. */
export interface PlanLanguage {
  locale: string;
  waiting: number;
  /** Translations held back by a failing check or a stale approval. */
  failing: number;
  /** Pages with a translation waiting. */
  pages: number;
  pagesMore: boolean;
  /** Translations that clear every bar and the involvement leaves to the checks. */
  settleable: number;
}

export interface ReviewPlan {
  stream: string;
  involvement: ReviewInvolvement;
  /** The first level with something waiting, or null when nothing does. */
  next: PlanLevel | null;
  coordinates: {
    sourceLanguage: string;
    targetLanguages: string[];
    points: PlanPoint[];
    confirmed: boolean;
    confirmedAt?: string;
  };
  keyLanguage: {
    /** Context cards waiting for the caller, by kind. */
    waiting: number;
    byKind: Record<string, number>;
    /** Concepts with a term in the source language. */
    concepts: number;
    /** Per target language, concepts with no usable term there. */
    untranslated: Record<string, number>;
  };
  exceptions: {
    /** Findings in the source at the last project-wide check; null before one. */
    sourceFindings: number | null;
    checkedAt?: string;
  };
  languages: PlanLanguage[];
}

export interface ServerReviewPlan {
  stream: string;
  involvement: string;
  next: string;
  coordinates: {
    source_language: string;
    target_languages?: string[] | null;
    points?:
      | {
          collection: string;
          coordinates?: Record<string, string> | null;
          voice?: string;
          source_only?: boolean;
          priority?: string;
        }[]
      | null;
    confirmed: boolean;
    confirmed_at?: string;
  };
  key_language: {
    waiting: number;
    by_kind?: Record<string, number> | null;
    concepts: number;
    untranslated?: Record<string, number> | null;
  };
  exceptions: { source_findings?: number | null; checked_at?: string };
  languages?:
    | {
        locale: string;
        waiting: number;
        failing: number;
        pages: number;
        pages_more?: boolean;
        settleable: number;
      }[]
    | null;
}

const LEVELS: readonly PlanLevel[] = [
  "coordinates",
  "key_language",
  "exceptions",
  "interfaces",
  "policy",
];

function involvementOf(v: string): ReviewInvolvement {
  return v === "interfaces" || v === "every" ? v : "context";
}

export function planFromServer(p: ServerReviewPlan): ReviewPlan {
  return {
    stream: p.stream,
    involvement: involvementOf(p.involvement),
    next: (LEVELS as readonly string[]).includes(p.next) ? (p.next as PlanLevel) : null,
    coordinates: {
      sourceLanguage: p.coordinates.source_language,
      targetLanguages: p.coordinates.target_languages ?? [],
      points: (p.coordinates.points ?? []).map((pt) => ({
        collection: pt.collection,
        coordinates: pt.coordinates ?? {},
        voice: pt.voice || undefined,
        sourceOnly: pt.source_only ?? false,
        priority: pt.priority || undefined,
      })),
      confirmed: p.coordinates.confirmed,
      confirmedAt: p.coordinates.confirmed_at || undefined,
    },
    keyLanguage: {
      waiting: p.key_language.waiting,
      byKind: p.key_language.by_kind ?? {},
      concepts: p.key_language.concepts,
      untranslated: p.key_language.untranslated ?? {},
    },
    exceptions: {
      sourceFindings: p.exceptions.source_findings ?? null,
      checkedAt: p.exceptions.checked_at || undefined,
    },
    languages: (p.languages ?? []).map((l) => ({
      locale: l.locale,
      waiting: l.waiting,
      failing: l.failing,
      pages: l.pages,
      pagesMore: l.pages_more ?? false,
      settleable: l.settleable,
    })),
  };
}

/** The products a plan's points name, with their channels, in order. */
export function planProducts(points: PlanPoint[]): { product: string; channels: string[] }[] {
  const byProduct = new Map<string, Set<string>>();
  for (const pt of points) {
    const product = pt.coordinates.product;
    if (!product) continue;
    const channels = byProduct.get(product) ?? new Set<string>();
    if (pt.coordinates.channel) channels.add(pt.coordinates.channel);
    byProduct.set(product, channels);
  }
  return [...byProduct.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([product, channels]) => ({ product, channels: [...channels].sort() }));
}
