// Hand-authored EvolutionInput fixtures for the concept evolution timeline
// (Apache-2.0). These feed buildEvolutionModel — in Storybook, in tests, and in
// the evolution-view components — with realistic, bowrain-free brand-vocabulary
// histories. WHY four shapes: each exercises a distinct path the model and its
// renderers must handle — a rich annotated history (rich), enough below-signal
// churn to trigger the density "clouds" (dense), many lanes so focus + "N more
// languages" kicks in (manyLanguage), and a single undated term so the core-only
// degradation path is covered (sparse). NOW is fixed so every fixture is
// deterministic regardless of the wall clock.

import type { EvolutionInput } from "../evolution-model";
import type { Concept, Market, Relation, TimelineEvent } from "../types";

/** The fixed "now" every fixture builds against — keeps models deterministic. */
export const NOW = "2026-06-14T00:00:00.000Z";

// ── Markets ──────────────────────────────────────────────────────────────────

const MARKETS: Market[] = [
  { name: "Nordics", locales: ["nb-NO", "nb"] },
  { name: "DACH", locales: ["de-DE", "de"] },
];

// ── The running example: c-folder ────────────────────────────────────────────

/**
 * The canonical "directory → folder" story. English carries a deprecated
 * "directory" (validity-bounded) that a governed change-set superseded with the
 * preferred "folder"; German, Norwegian (a Nordics-scoped sibling that opens
 * after genesis), and French fill out the lanes; a dated COMPETITOR relation to
 * Explorer lands a relation milestone. The timeline is a genuine revision log —
 * create, a status promotion, the governed rename change-set, an observation, a
 * comment, and a few routine revisions — so the model takes its rich path.
 */
export function richEvolution(): EvolutionInput {
  const concept: Concept = {
    id: "c-folder",
    domain: "ui",
    source: "brand_vocabulary",
    definition: "A named container that groups files and other folders.",
    createdAt: "2023-01-01T00:00:00.000Z",
    updatedAt: "2025-01-01T00:00:00.000Z",
    terms: [
      {
        text: "directory",
        locale: "en",
        status: "deprecated",
        note: "Legacy term — replaced by 'folder' in the 2024 voice refresh.",
        validity: {
          validFrom: "2023-01-01T00:00:00.000Z",
          validTo: "2024-06-01T00:00:00.000Z",
        },
      },
      {
        text: "folder",
        locale: "en",
        status: "preferred",
        validity: { validFrom: "2024-06-01T00:00:00.000Z" },
      },
      {
        text: "Ordner",
        locale: "de",
        status: "approved",
        validity: { validFrom: "2023-01-01T00:00:00.000Z" },
      },
      {
        text: "mappe",
        locale: "nb",
        status: "preferred",
        validity: {
          validFrom: "2024-01-01T00:00:00.000Z",
          tags: { market: "nordics" },
        },
      },
      {
        text: "dossier",
        locale: "fr",
        status: "approved",
        validity: { validFrom: "2024-03-01T00:00:00.000Z" },
      },
    ],
  };

  const relations: Relation[] = [
    {
      id: "rel-explorer",
      sourceId: "c-folder",
      targetId: "c-explorer",
      type: "COMPETITOR",
      note: "Competing 'Explorer' concept tracked for disambiguation.",
      validity: { validFrom: "2025-01-01T00:00:00.000Z" },
    },
  ];

  const timeline: TimelineEvent[] = [
    {
      id: "ev-create",
      kind: "create",
      at: "2023-01-01T00:00:00.000Z",
      actor: "Aria",
      summary: "Created concept “folder”",
      ref: "rev-1",
    },
    {
      id: "ev-rev-de",
      kind: "revision",
      at: "2023-02-15T00:00:00.000Z",
      actor: "Aria",
      summary: "Added German term “Ordner”",
      ref: "rev-2",
    },
    {
      id: "ev-rev-nb",
      kind: "revision",
      at: "2024-01-01T00:00:00.000Z",
      actor: "Nils",
      summary: "Added Norwegian term “mappe” for the Nordics market",
      ref: "rev-3",
    },
    {
      id: "ev-status-folder",
      kind: "status",
      at: "2024-06-01T00:00:00.000Z",
      actor: "Aria",
      summary: "Promoted “folder” to preferred (en)",
      ref: "rev-4",
    },
    {
      id: "ev-changeset-rename",
      kind: "changeset",
      at: "2024-06-01T00:00:00.000Z",
      actor: "Aria",
      summary: "Rename directory→folder merged",
      ref: "cs1",
      detail: "Deprecated “directory” and blessed “folder” across the term base.",
    },
    {
      id: "ev-obs-competitor",
      kind: "observation",
      at: "2025-01-01T00:00:00.000Z",
      actor: "Priya",
      summary: "Logged competitor usage of “Explorer”",
      detail: "A rival product calls the same surface “Explorer”.",
    },
    {
      id: "ev-comment",
      kind: "comment",
      at: "2025-02-10T00:00:00.000Z",
      actor: "Léa",
      summary: "Discussed keeping the deprecated term visible",
      detail: "Keep “directory” listed so writers know what replaced it.",
    },
    {
      id: "ev-rev-fr",
      kind: "revision",
      at: "2024-03-01T00:00:00.000Z",
      actor: "Léa",
      summary: "Added French term “dossier”",
      ref: "rev-5",
    },
  ];

  return {
    concept,
    relations,
    neighbourLabels: { "c-explorer": "Explorer" },
    timeline,
    markets: MARKETS,
  };
}

// ── A dense history (triggers the density "clouds") ──────────────────────────

/** Build a run of routine `revision` events `days` apart starting at `startIso`. */
function routineRevisions(
  prefix: string,
  startIso: string,
  count: number,
  spacingDays: number,
): TimelineEvent[] {
  const startMs = Date.parse(startIso);
  const dayMs = 86_400_000;
  return Array.from({ length: count }, (_, i) => {
    const at = new Date(startMs + i * spacingDays * dayMs).toISOString();
    return {
      id: `${prefix}-${i + 1}`,
      kind: "revision",
      at,
      actor: i % 2 === 0 ? "Aria" : "Léa",
      summary: `Copy tweak #${i + 1}`,
      ref: `${prefix}-rev-${i + 1}`,
    } satisfies TimelineEvent;
  });
}

/**
 * The rich story plus two tight bursts of routine revisions — six in March 2024
 * and six in February 2025, each a day or two apart — so the below-signal events
 * fold into clusters ("N changes" clouds) while the high-signal create / status
 * / change-set milestones stay discrete.
 */
export function denseEvolution(): EvolutionInput {
  const base = richEvolution();
  const bursts: TimelineEvent[] = [
    ...routineRevisions("burst-a", "2024-03-04T00:00:00.000Z", 6, 2),
    ...routineRevisions("burst-b", "2025-02-03T00:00:00.000Z", 6, 1),
  ];
  return {
    ...base,
    timeline: [...(base.timeline ?? []), ...bursts],
  };
}

// ── Many languages (exercises focus + "N more languages") ────────────────────

interface LangSeed {
  locale: string;
  text: string;
  validFrom?: string;
  market?: string;
}

// English is the genesis lane (preferred, present from createdAt). The others
// open on staggered later dates so the non-origin lanes register as siblings,
// pushing the lane count past the focus cap.
const LANGUAGE_SEEDS: LangSeed[] = [
  { locale: "en", text: "folder" },
  { locale: "de", text: "Ordner", validFrom: "2023-06-01T00:00:00.000Z", market: "dach" },
  { locale: "fr", text: "dossier", validFrom: "2023-09-01T00:00:00.000Z" },
  { locale: "es", text: "carpeta", validFrom: "2024-01-01T00:00:00.000Z" },
  { locale: "it", text: "cartella", validFrom: "2024-04-01T00:00:00.000Z" },
  { locale: "nb", text: "mappe", validFrom: "2024-07-01T00:00:00.000Z", market: "nordics" },
  { locale: "sv", text: "mapp", validFrom: "2024-10-01T00:00:00.000Z" },
  { locale: "ja", text: "フォルダー", validFrom: "2025-01-01T00:00:00.000Z" },
];

/**
 * Eight locales, one term each, opening on staggered dates. English is preferred
 * and present from genesis; the rest are approved siblings that arrive later — so
 * the model selects a focus subset and the renderer surfaces "N more languages".
 */
export function manyLanguageEvolution(): EvolutionInput {
  const concept: Concept = {
    id: "c-folder",
    domain: "ui",
    source: "brand_vocabulary",
    definition: "A named container that groups files and other folders.",
    createdAt: "2023-01-01T00:00:00.000Z",
    updatedAt: "2025-01-01T00:00:00.000Z",
    terms: LANGUAGE_SEEDS.map((seed) => ({
      text: seed.text,
      locale: seed.locale,
      status: seed.locale === "en" ? "preferred" : "approved",
      validity: {
        validFrom: seed.validFrom ?? "2023-01-01T00:00:00.000Z",
        ...(seed.market ? { tags: { market: seed.market } } : {}),
      },
    })),
  };

  return {
    concept,
    markets: MARKETS,
  };
}

// ── Sparse (the degradation path) ────────────────────────────────────────────

/**
 * A brand-new concept: just a creation instant and a single English term with no
 * validity window and no revision log. The model has to synthesise a genesis
 * milestone and degrade gracefully — the floor case for the renderers.
 */
export function sparseEvolution(): EvolutionInput {
  const concept: Concept = {
    id: "c-folder",
    domain: "ui",
    source: "brand_vocabulary",
    definition: "A named container that groups files and other folders.",
    createdAt: "2026-05-20T00:00:00.000Z",
    terms: [{ text: "folder", locale: "en", status: "preferred" }],
  };

  return { concept };
}
