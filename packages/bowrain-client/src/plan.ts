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

/**
 * One rule and the findings it raised. Most exceptions are one rule broken
 * many times, so a reviewer decides the rule before the blocks.
 */
export interface PlanRule {
  /** What the review deck's rule filter takes. */
  key: string;
  /** The check's rule id, such as `terms.vocabulary`. */
  rule: string;
  /** The term or the pattern the rule names. */
  subject?: string;
  /** One finding's wording, which names the rule to a person. */
  message?: string;
  /** The terms-store concept a word rule comes from. */
  conceptId?: string;
  fails: boolean;
  count: number;
}

/** The plan's part for one target language the caller may review. */
export interface PlanLanguage {
  locale: string;
  waiting: number;
  /** Translations held back by a failing check or a stale approval. */
  failing: number;
  /** Pages the involvement asks a person to read: the sample under Context. */
  pages: number;
  pagesMore: boolean;
  /** Every page with a translation waiting. */
  pagesTotal: number;
  /** Pages holding a translation with a failing finding. */
  pagesWithFindings: number;
  /** Translations that clear every bar and the involvement leaves to the checks. */
  settleable: number;
}

/**
 * Whether the plan's last step may settle now: first_pass settles on the
 * checks alone once, policy once the key language is decided, held waits for
 * the key language after the first pass, none under the involvement every.
 */
export type PlanSettle = "first_pass" | "policy" | "held" | "none";

/** A project scan for candidate terms, started from the plan. */
export interface PlanScan {
  id: string;
  status: "queued" | "processing" | "completed" | "failed";
  progress: number;
  createdAt: string;
  updatedAt: string;
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
    /** The last project scan for candidate terms, if one was started. */
    scan?: PlanScan;
  };
  exceptions: {
    /** Findings in the source at the last project-wide check; null before one. */
    sourceFindings: number | null;
    checkedAt?: string;
    /** The rules behind the most findings, in the source and per target language. */
    byRule: { source: PlanRule[]; targets: Record<string, PlanRule[]> };
  };
  languages: PlanLanguage[];
  policy: {
    settle: PlanSettle;
    /** The project's open key-language decisions, for any reviewer. */
    keyLanguageOpen: number;
    firstPassAt?: string;
  };
}

interface ServerPlanScan {
  id: string;
  status: string;
  progress: number;
  created_at: string;
  updated_at: string;
}

interface ServerPlanRule {
  key: string;
  rule: string;
  subject?: string;
  message?: string;
  concept_id?: string;
  fails?: boolean;
  count: number;
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
    scan?: ServerPlanScan | null;
  };
  exceptions: {
    source_findings?: number | null;
    checked_at?: string;
    by_rule?: {
      source?: ServerPlanRule[] | null;
      targets?: Record<string, ServerPlanRule[] | null> | null;
    } | null;
  };
  languages?:
    | {
        locale: string;
        waiting: number;
        failing: number;
        pages: number;
        pages_more?: boolean;
        pages_total?: number;
        pages_with_findings?: number;
        settleable: number;
      }[]
    | null;
  policy?: {
    settle: string;
    key_language_open?: number;
    first_pass_at?: string;
  } | null;
}

const SETTLE: readonly PlanSettle[] = ["first_pass", "policy", "held", "none"];
const SCAN_STATUS: readonly PlanScan["status"][] = ["queued", "processing", "completed", "failed"];

export function planScanFromServer(s: ServerPlanScan): PlanScan {
  return {
    id: s.id,
    status: (SCAN_STATUS as readonly string[]).includes(s.status)
      ? (s.status as PlanScan["status"])
      : "failed",
    progress: s.progress,
    createdAt: s.created_at,
    updatedAt: s.updated_at,
  };
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

function ruleFromServer(r: ServerPlanRule): PlanRule {
  return {
    key: r.key,
    rule: r.rule,
    subject: r.subject || undefined,
    message: r.message || undefined,
    conceptId: r.concept_id || undefined,
    fails: r.fails ?? false,
    count: r.count,
  };
}

/** A rule's name for a person: its message, else its subject, else its id. */
export function planRuleLabel(r: PlanRule): string {
  return r.message || r.subject || r.rule;
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
      scan: p.key_language.scan ? planScanFromServer(p.key_language.scan) : undefined,
    },
    exceptions: {
      sourceFindings: p.exceptions.source_findings ?? null,
      checkedAt: p.exceptions.checked_at || undefined,
      byRule: {
        source: (p.exceptions.by_rule?.source ?? []).map(ruleFromServer),
        targets: Object.fromEntries(
          Object.entries(p.exceptions.by_rule?.targets ?? {}).map(([locale, rules]) => [
            locale,
            (rules ?? []).map(ruleFromServer),
          ]),
        ),
      },
    },
    languages: (p.languages ?? []).map((l) => ({
      locale: l.locale,
      waiting: l.waiting,
      failing: l.failing,
      pages: l.pages,
      pagesMore: l.pages_more ?? false,
      pagesTotal: l.pages_total ?? l.pages,
      pagesWithFindings: l.pages_with_findings ?? 0,
      settleable: l.settleable,
    })),
    policy: {
      settle: (SETTLE as readonly string[]).includes(p.policy?.settle ?? "")
        ? (p.policy!.settle as PlanSettle)
        : p.involvement === "every"
          ? "none"
          : "first_pass",
      keyLanguageOpen: p.policy?.key_language_open ?? 0,
      firstPassAt: p.policy?.first_pass_at || undefined,
    },
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
