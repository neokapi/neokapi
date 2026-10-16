// A deterministic, synthetic atlas for stories and tests: Larkspur Travel, a
// fictional travel company with four products, four channels, markets across
// Europe and beyond, several locales, and 90 days of context work. Everything
// here is made up; the shape is exactly the `kapi context atlas --json`
// contract.

import type {
  AtlasActor,
  AtlasAxis,
  AtlasData,
  AtlasEvent,
  AtlasRecord,
  AtlasState,
  AtlasValue,
} from "../types";

// ── Deterministic randomness ─────────────────────────────────────────────

function mulberry32(seed: number) {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

type Rand = ReturnType<typeof mulberry32>;

function pickOf<T>(rand: Rand, items: readonly T[]): T {
  return items[Math.floor(rand() * items.length)];
}

function weighted<T>(rand: Rand, items: readonly [T, number][]): T {
  const total = items.reduce((s, [, w]) => s + w, 0);
  let r = rand() * total;
  for (const [item, w] of items) {
    r -= w;
    if (r <= 0) return item;
  }
  return items[items.length - 1][0];
}

// ── Time ─────────────────────────────────────────────────────────────────

const ORIGIN = Date.parse("2026-07-17T00:00:00Z");
const DAYS = 90;
const HOUR = 3_600_000;

function at(day: number, hour = 10): string {
  return new Date(ORIGIN + day * 86_400_000 + hour * HOUR).toISOString();
}

// ── The company ──────────────────────────────────────────────────────────

const PRODUCTS: AtlasValue[] = [
  { id: "booking", label: "Booking" },
  { id: "loyalty", label: "Loyalty" },
  { id: "help-center", label: "Help center" },
  { id: "partner-portal", label: "Partner portal" },
];

const CHANNELS: AtlasValue[] = [
  { id: "app", label: "App" },
  { id: "web", label: "Web" },
  { id: "email", label: "Email" },
  { id: "push", label: "Push" },
];

const COUNTRIES: AtlasValue[] = [
  { id: "no", label: "Norway", parent: "northern-europe", centroid: [62, 10] },
  { id: "se", label: "Sweden", parent: "northern-europe", centroid: [62, 15] },
  { id: "dk", label: "Denmark", parent: "northern-europe", centroid: [56, 10] },
  { id: "fi", label: "Finland", parent: "northern-europe", centroid: [64, 26] },
  { id: "is", label: "Iceland", parent: "northern-europe", centroid: [65, -18] },
  { id: "gb", label: "United Kingdom", parent: "northern-europe", centroid: [54, -2] },
  { id: "ie", label: "Ireland", parent: "northern-europe", centroid: [53, -8] },
  { id: "nl", label: "Netherlands", parent: "western-europe", centroid: [52.2, 5.5] },
  { id: "de", label: "Germany", parent: "western-europe", centroid: [51, 10] },
  { id: "fr", label: "France", parent: "western-europe", centroid: [46, 2] },
  { id: "es", label: "Spain", parent: "southern-europe", centroid: [40, -4] },
  { id: "it", label: "Italy", parent: "southern-europe", centroid: [42.8, 12.8] },
  { id: "pl", label: "Poland", parent: "eastern-europe", centroid: [52, 19] },
  { id: "us", label: "United States", parent: "northern-america", centroid: [39.8, -98.6] },
  { id: "ca", label: "Canada", parent: "northern-america", centroid: [56, -106] },
  { id: "jp", label: "Japan", parent: "eastern-asia", centroid: [36, 138] },
  { id: "au", label: "Australia", parent: "australia-and-new-zealand", centroid: [-25, 134] },
];

/** The market axis also names a declared group: a rule can apply to all of it. */
const MARKET_GROUP: AtlasValue = { id: "nordics", label: "Nordics", parent: "northern-europe" };

const LOCALES = [
  "en-GB",
  "en-US",
  "nb-NO",
  "sv-SE",
  "da-DK",
  "fi-FI",
  "de-DE",
  "fr-FR",
  "es-ES",
  "it-IT",
  "nl-NL",
  "pl-PL",
  "ja-JP",
  "fr-CA",
] as const;

function regionOf(locale: string): string {
  return locale.split("-")[1]?.toLowerCase() ?? "";
}

const AUDIENCES: AtlasValue[] = [
  { id: "general", label: "General" },
  { id: "new-customers", label: "New customers" },
  { id: "members", label: "Members" },
  { id: "partners", label: "Partners" },
];

const AGES: AtlasValue[] = [{ id: "13-17" }, { id: "18-34" }, { id: "35-64" }, { id: "65-" }];

const LIFECYCLE: AtlasValue[] = [
  { id: "preview", label: "Preview" },
  { id: "current", label: "Current" },
  { id: "sunset", label: "Sunset" },
];

const PEOPLE: AtlasActor[] = [
  { kind: "person", name: "Ingrid Solberg" },
  { kind: "person", name: "Tomás Ferreira" },
  { kind: "person", name: "Mei Tanaka" },
];
const AGENTS: AtlasActor[] = [
  { kind: "agent", name: "claude-code", session: "s-41c2" },
  { kind: "agent", name: "codex", session: "s-9a07" },
  { kind: "agent", name: "cursor", session: "s-77e1" },
];
const TOOLS: AtlasActor[] = [{ kind: "tool", name: "kapi-import" }];

// ── Wording ──────────────────────────────────────────────────────────────

const RULE_TEMPLATES: [string, string][] = [
  ["Say “travel pass”, not “pass”", "Your travel pass is ready in the app."],
  ["Use “check in online” for web check-in", "Check in online up to 24 hours before departure."],
  ["Avoid “cheap” in loyalty copy", "Members save on every trip."],
  ["Write prices with the currency after the amount", "From 499 NOK per person."],
  ["Name the partner lounge “Larkspur Lounge”", "Relax in the Larkspur Lounge before boarding."],
  ["Keep push notifications under 90 characters", "Gate changed: now boarding at B12."],
  ["Address members by first name", "Welcome back, Sara."],
  ["Say “points”, not “miles”", "You earned 120 points on this trip."],
  ["Spell out the refund window", "You can cancel for a full refund within 24 hours."],
  ["Use “help center”, not “support portal”", "Find answers in the help center."],
  ["Use sentence case in buttons", "Book this trip"],
  ["Prefer “change your booking” to “modify reservation”", "Change your booking in two steps."],
];

const NOTE_TEMPLATES = [
  "Partners read the portal on desktop; long tables are fine there.",
  "Loyalty emails go out on Tuesdays; keep subject lines short.",
  "The help center answers are reviewed by legal before release.",
  "Younger readers skim; lead with the action.",
  "Refund wording differs by market because the law differs.",
  "Japanese copy uses the polite form throughout.",
];

const MEMORY_TEMPLATES = [
  "Your trip is confirmed.",
  "Add a bag",
  "Boarding starts at {time}.",
  "Manage booking",
  "Earn points on every trip",
  "Your seat",
  "Change flight",
  "Download boarding pass",
];

const TERMS = [
  "travel pass",
  "boarding pass",
  "points",
  "Larkspur Lounge",
  "check in online",
  "help center",
  "fare class",
  "carry-on bag",
  "gate",
  "layover",
];

const PATHS = [
  "content/booking/checkout.md",
  "content/booking/confirmation.md",
  "content/loyalty/welcome-email.md",
  "content/help-center/refunds/policy.md",
  "content/help-center/baggage.md",
  "apps/mobile/src/i18n/en.json",
  "content/partner-portal/onboarding/guide.md",
];

const LONG_SUFFIX =
  " when the copy appears in a transactional message that a customer receives after changing a booking through a partner";

// ── Generation ───────────────────────────────────────────────────────────

export interface FixtureOptions {
  /** PRNG seed; the same seed gives the same atlas. */
  seed?: number;
  /** Scale the record counts (1 = about 500 records). */
  scale?: number;
  /** Make labels and paths long, for layout stories. */
  longLabels?: boolean;
}

interface Built {
  records: AtlasRecord[];
  events: AtlasEvent[];
}

const STATE_EVENT: Record<string, string> = {
  suggested: "observe",
  established: "keep",
  contested: "correct",
  dropped: "drop",
  withdrawn: "withdraw",
  reset: "reset",
};

function short(rand: Rand): string {
  return Math.floor(rand() * 0xffffff)
    .toString(16)
    .padStart(6, "0");
}

/**
 * A lifecycle for a governed record: suggested on `day`, then usually kept,
 * sometimes contested, dropped or withdrawn. Days stay inside the span.
 */
function lifecycle(rand: Rand, day: number): AtlasState[] {
  const states: AtlasState[] = [{ at: at(day, 9 + Math.floor(rand() * 8)), state: "suggested" }];
  let d = day;
  const next = (lo: number, hi: number) => {
    d = d + lo + Math.floor(rand() * (hi - lo + 1));
    return d < DAYS ? d : -1;
  };
  const path = weighted<string>(rand, [
    ["keep", 55],
    ["wait", 18],
    ["contest", 12],
    ["drop", 8],
    ["keep-withdraw", 4],
    ["keep-contest", 3],
  ]);
  const push = (state: string, lo: number, hi: number) => {
    const nd = next(lo, hi);
    if (nd >= 0) states.push({ at: at(nd, 11 + Math.floor(rand() * 6)), state });
    return nd >= 0;
  };
  switch (path) {
    case "keep":
      push("established", 1, 12);
      break;
    case "contest":
      if (push("contested", 1, 8) && rand() < 0.5) push("established", 2, 10);
      break;
    case "drop":
      push("dropped", 1, 10);
      break;
    case "keep-withdraw":
      if (push("established", 1, 6)) push("withdrawn", 5, 30);
      break;
    case "keep-contest":
      if (push("established", 1, 6)) push("contested", 3, 20);
      break;
  }
  return states;
}

function eventsFor(rec: AtlasRecord, rand: Rand): AtlasEvent[] {
  const out: AtlasEvent[] = [];
  rec.history.forEach((h, i) => {
    const kind = STATE_EVENT[h.state] ?? "decision";
    out.push({
      id: `${rec.id}#${i}`,
      at: h.at,
      kind,
      record: rec.id,
      actor: rec.actor,
      label: rec.label,
    });
  });
  if (rec.valid_from) {
    const kept = rec.history.find((h) => h.state === "established")?.at ?? rec.recorded;
    out.push({
      id: `${rec.id}#opens`,
      at: kept,
      effective: rec.valid_from,
      kind: "window-opens",
      record: rec.id,
      label: rec.label,
    });
  }
  if (rec.valid_to) {
    const kept = rec.history.find((h) => h.state === "established")?.at ?? rec.recorded;
    out.push({
      id: `${rec.id}#closes`,
      at: kept,
      effective: rec.valid_to,
      kind: "window-closes",
      record: rec.id,
      label: rec.label,
    });
  }
  void rand;
  return out;
}

function governed(rand: Rand, opts: Required<FixtureOptions>): Built {
  const records: AtlasRecord[] = [];
  const events: AtlasEvent[] = [];
  const n = (k: number) => Math.max(1, Math.round(k * opts.scale));
  const long = (s: string) => (opts.longLabels ? s + LONG_SUFFIX : s);
  const path = (p: string) =>
    opts.longLabels ? p.replace("content/", "content/regions/europe/markets/shared/") : p;

  // Rules. Two bursts: a term change around day 44 and a UK and Ireland
  // contact rule around day 70; a summer campaign window; one upcoming window.
  for (let i = 0; i < n(150); i++) {
    const burst = i % 9 === 0 ? 44 : i % 13 === 0 ? 70 : -1;
    const day = burst >= 0 ? burst + Math.floor(rand() * 3) : Math.floor(rand() * (DAYS - 3));
    const [label, quote] = RULE_TEMPLATES[i % RULE_TEMPLATES.length];
    const actor = weighted<AtlasActor[]>(rand, [
      [AGENTS, 55],
      [PEOPLE, 35],
      [TOOLS, 10],
    ]);
    const who = pickOf(rand, actor);
    const market =
      burst === 70
        ? pickOf(rand, ["gb", "ie"])
        : weighted<string>(rand, [
            ["", 30],
            ["nordics", 8],
            ...COUNTRIES.map((c): [string, number] => [c.id, 4]),
          ]);
    const coordinates: Record<string, string> = {
      family: "rule",
      product: pickOf(rand, PRODUCTS).id,
      channel: weighted<string>(rand, [
        ["app", 34],
        ["web", 30],
        ["email", 22],
        ["push", 14],
      ]),
      audience: pickOf(rand, AUDIENCES).id,
      lifecycle: weighted<string>(rand, [
        ["current", 70],
        ["preview", 18],
        ["sunset", 12],
      ]),
      actor: who.kind,
    };
    if (market) coordinates.market = market;
    if (rand() < 0.3) coordinates.age = pickOf(rand, AGES).id;
    if (rand() < 0.4) {
      const locale = pickOf(rand, LOCALES);
      coordinates.locale = locale;
      coordinates.region = regionOf(locale);
    }
    const rec: AtlasRecord = {
      id: `op-${short(rand)}${i.toString(16)}`,
      family: "rule",
      label: long(label),
      weight: 1,
      coordinates,
      recorded: at(day, 9),
      history: lifecycle(rand, day),
      short: short(rand).slice(0, 6),
      actor: who,
      evidence: [{ path: path(pickOf(rand, PATHS)), quote }],
      scope: [coordinates.product, coordinates.channel, market].filter(Boolean).join(" · "),
    };
    rec.recorded = rec.history[0].at;
    // Windows: a summer campaign (days 20 to 55), an upcoming winter sale, and
    // a few rules that expired in September.
    if (i % 11 === 3) {
      rec.valid_from = at(20, 0);
      rec.valid_to = at(55, 0);
      rec.label = long("Summer campaign: lead with “Summer escapes”");
    } else if (i % 17 === 5) {
      rec.valid_from = at(DAYS + 20, 0);
      rec.label = long("Winter sale: say “Winter fares”, never “cheap flights”");
    } else if (i % 19 === 7) {
      rec.valid_to = at(60, 0);
    }
    if (rec.history.some((h) => h.state === "contested")) {
      rec.contested_by = [`op-${short(rand)}`];
    }
    records.push(rec);
    events.push(...eventsFor(rec, rand));
  }

  // Notes.
  for (let i = 0; i < n(40); i++) {
    const day = Math.floor(rand() * (DAYS - 2));
    const who = pickOf(rand, rand() < 0.6 ? AGENTS : PEOPLE);
    const text = NOTE_TEMPLATES[i % NOTE_TEMPLATES.length];
    const rec: AtlasRecord = {
      id: `op-n${short(rand)}${i}`,
      family: "note",
      label: long(text.split(";")[0].replace(/\.$/u, "")),
      weight: 1,
      coordinates: {
        family: "note",
        product: pickOf(rand, PRODUCTS).id,
        channel: pickOf(rand, CHANNELS).id,
        audience: pickOf(rand, AUDIENCES).id,
        actor: who.kind,
      },
      recorded: at(day, 10),
      history: [{ at: at(day, 10), state: "suggested" }],
      short: short(rand).slice(0, 6),
      actor: who,
      text,
      evidence: [{ path: path(pickOf(rand, PATHS)), quote: text }],
    };
    if (rand() < 0.6 && day + 3 < DAYS) {
      rec.history.push({ at: at(day + 1 + Math.floor(rand() * 3), 14), state: "established" });
    }
    records.push(rec);
    events.push(...eventsFor(rec, rand));
  }

  // Content memory pairs.
  for (let i = 0; i < n(90); i++) {
    const day = Math.floor(rand() * (DAYS - 2));
    const locale = LOCALES[(i * 5) % LOCALES.length];
    const who = pickOf(rand, rand() < 0.7 ? TOOLS : PEOPLE);
    const rec: AtlasRecord = {
      id: `op-m${short(rand)}${i}`,
      family: "memory",
      label: long(MEMORY_TEMPLATES[i % MEMORY_TEMPLATES.length]),
      weight: 1,
      coordinates: {
        family: "memory",
        product: pickOf(rand, PRODUCTS).id,
        locale,
        region: regionOf(locale),
        market: regionOf(locale),
        actor: who.kind,
      },
      recorded: at(day, 8),
      history: lifecycle(rand, day),
      short: short(rand).slice(0, 6),
      actor: who,
    };
    rec.recorded = rec.history[0].at;
    records.push(rec);
    events.push(...eventsFor(rec, rand));
  }

  // Terms.
  for (let i = 0; i < n(60); i++) {
    const day = i < 8 ? 44 : Math.floor(rand() * (DAYS - 2));
    const locale = LOCALES[(i * 3) % LOCALES.length];
    const term = TERMS[i % TERMS.length];
    const rec: AtlasRecord = {
      id: `term:${term.replace(/\s+/gu, "-")}-${locale.toLowerCase()}`,
      family: "term",
      label: long(term),
      weight: 1,
      coordinates: {
        family: "term",
        product: pickOf(rand, PRODUCTS).id,
        locale,
        region: regionOf(locale),
        market: regionOf(locale),
        actor: "person",
      },
      recorded: at(day, 12),
      history:
        rand() < 0.75
          ? [{ at: at(day, 12), state: "established" }]
          : [{ at: at(day, 12), state: "suggested" }],
      actor: pickOf(rand, PEOPLE),
    };
    records.push(rec);
    events.push(...eventsFor(rec, rand));
  }

  return { records, events };
}

/** Content cells: units of one collection and locale that share a state and a decision day. */
function content(rand: Rand, opts: Required<FixtureOptions>): Built {
  const records: AtlasRecord[] = [];
  const events: AtlasEvent[] = [];
  const locales = LOCALES.slice(0, 12);
  for (const product of PRODUCTS) {
    for (const locale of locales) {
      // One collection and locale in ten has no content yet.
      if (rand() < 0.1) continue;
      const cells = Math.max(1, Math.round((2 + Math.floor(rand() * 2)) * opts.scale));
      for (let c = 0; c < cells; c++) {
        // After the term change on day 44, booking content goes stale and is
        // decided again over the following weeks.
        const late = product.id === "booking" && c > 0;
        const day = late ? 45 + Math.floor(rand() * 30) : Math.floor(rand() * DAYS);
        const state = late
          ? weighted<string>(rand, [
              ["stale", 30],
              ["translated", 40],
              ["established", 30],
            ])
          : weighted<string>(rand, [
              ["established", 45],
              ["translated", 25],
              ["draft", 15],
              ["not-started", 10],
              ["stale", 5],
            ]);
        const dayStr = at(day, 0).slice(0, 10);
        const id = `content:${product.id}:${locale}:${state}:${dayStr}`;
        if (records.some((r) => r.id === id)) continue;
        const rec: AtlasRecord = {
          id,
          family: "content",
          label: `${product.label} · ${locale}`,
          weight: 5 + Math.floor(rand() * 110),
          coordinates: {
            family: "content",
            product: product.id,
            channel: product.id === "loyalty" ? "email" : product.id === "booking" ? "app" : "web",
            locale,
            region: regionOf(locale),
            market: regionOf(locale),
            lifecycle: "current",
            actor: "tool",
          },
          recorded: at(day, 16),
          history: [{ at: at(day, 16), state }],
        };
        records.push(rec);
        events.push({
          id: `${id}#decision`,
          at: at(day, 16),
          kind: "decision",
          record: id,
          label: rec.label,
        });
      }
    }
  }
  return { records, events };
}

function axes(records: AtlasRecord[]): AtlasAxis[] {
  const used = (axis: string) => new Set(records.map((r) => r.coordinates[axis]).filter(Boolean));
  const only = (values: AtlasValue[], axis: string) => {
    const u = used(axis);
    return values.filter((v) => u.has(v.id));
  };
  const regions = [...used("region")]
    .map((id) => COUNTRIES.find((c) => c.id === id))
    .filter((v): v is AtlasValue => !!v);
  return [
    {
      id: "family",
      kind: "category",
      source: "derived",
      values: only(
        [
          { id: "rule", label: "Rules" },
          { id: "note", label: "Notes" },
          { id: "memory", label: "Content memory" },
          { id: "term", label: "Terms" },
          { id: "content", label: "Content" },
        ],
        "family",
      ),
    },
    { id: "product", kind: "category", source: "structural", values: only(PRODUCTS, "product") },
    { id: "channel", kind: "category", source: "structural", values: only(CHANNELS, "channel") },
    {
      id: "market",
      label: "Market",
      kind: "geo",
      source: "declared",
      values: only([...COUNTRIES, MARKET_GROUP], "market"),
    },
    {
      id: "audience",
      label: "Audience",
      kind: "category",
      source: "declared",
      values: only(AUDIENCES, "audience"),
    },
    { id: "age", label: "Age", kind: "range", source: "declared", values: only(AGES, "age") },
    {
      id: "lifecycle",
      label: "Lifecycle",
      kind: "ordered",
      source: "declared",
      values: only(LIFECYCLE, "lifecycle"),
    },
    {
      id: "locale",
      kind: "category",
      source: "derived",
      values: LOCALES.filter((l) => used("locale").has(l)).map((id) => ({ id })),
    },
    { id: "region", kind: "geo", source: "derived", values: regions },
    {
      id: "actor",
      kind: "category",
      source: "derived",
      values: only(
        [
          { id: "person", label: "Person" },
          { id: "agent", label: "Agent" },
          { id: "tool", label: "Tool" },
        ],
        "actor",
      ),
    },
  ];
}

/** The Larkspur Travel atlas. */
export function makeAtlasFixture(options: FixtureOptions = {}): AtlasData {
  const opts: Required<FixtureOptions> = {
    seed: options.seed ?? 20261016,
    scale: options.scale ?? 1,
    longLabels: options.longLabels ?? false,
  };
  const rand = mulberry32(opts.seed);
  const g = governed(rand, opts);
  const c = content(rand, opts);
  const records = [...g.records, ...c.records];
  const events = [...g.events, ...c.events].sort((a, b) => a.at.localeCompare(b.at));
  return {
    format: "kapi-context-atlas",
    version: 1,
    project: "larkspur-travel",
    generated_at: at(DAYS - 1, 18),
    span: { from: at(0, 0), to: at(DAYS - 1, 18) },
    axes: axes(records),
    records,
    events,
    notes: [],
  };
}

/** An atlas with nothing in it: a project whose context is empty. */
export function makeEmptyAtlas(): AtlasData {
  return {
    format: "kapi-context-atlas",
    version: 1,
    project: "new-project",
    generated_at: at(DAYS - 1, 18),
    span: { from: at(DAYS - 1, 18), to: at(DAYS - 1, 18) },
    axes: [],
    records: [],
    events: [],
    notes: ["No context operations are recorded for this project yet."],
  };
}

/** An atlas holding one rule. */
export function makeOneRecordAtlas(): AtlasData {
  const rec: AtlasRecord = {
    id: "op-7f3a2c",
    family: "rule",
    label: "Say “travel pass”, not “pass”",
    weight: 1,
    coordinates: { family: "rule", product: "booking", channel: "app", actor: "agent" },
    recorded: at(40, 10),
    history: [
      { at: at(40, 10), state: "suggested" },
      { at: at(43, 15), state: "established" },
    ],
    short: "7f3a2c",
    actor: { kind: "agent", name: "claude-code", session: "s-41c2" },
    evidence: [
      { path: "content/booking/checkout.md", quote: "Your travel pass is ready in the app." },
    ],
    scope: "booking · app",
  };
  return {
    format: "kapi-context-atlas",
    version: 1,
    project: "larkspur-travel",
    generated_at: at(DAYS - 1, 18),
    span: { from: at(40, 10), to: at(DAYS - 1, 18) },
    axes: [
      { id: "family", kind: "category", source: "derived", values: [{ id: "rule" }] },
      { id: "product", kind: "category", source: "structural", values: [{ id: "booking" }] },
      { id: "channel", kind: "category", source: "structural", values: [{ id: "app" }] },
      { id: "actor", kind: "category", source: "derived", values: [{ id: "agent" }] },
    ],
    records: [rec],
    events: eventsFor(rec, mulberry32(1)),
    notes: [],
  };
}
