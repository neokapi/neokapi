// Content memory entries for the stories and tests, behind an in-memory
// adapter shaped like the host's: a paged search, the store's languages,
// edits, deletes and entity marking all act on the copy held here.

import type {
  MemoryAdapter,
  MemoryEntryDTO,
  MemoryFacets,
  VariantInputDTO,
} from "@neokapi/ui-primitives";

const HOUR = 3_600_000;
const NOW = Date.UTC(2026, 9, 14, 12);

function at(hoursAgo: number): string {
  return new Date(NOW - hoursAgo * HOUR).toISOString();
}

export const SAMPLE_ENTRIES: MemoryEntryDTO[] = [
  {
    id: "1",
    project_id: "kapimart",
    hint_src_lang: "en-US",
    variants: {
      "en-US": {
        locale: "en-US",
        text: "Proceed to checkout",
        runs: [{ text: "Proceed to checkout" }],
      },
      "fr-FR": {
        locale: "fr-FR",
        text: "Passer à la caisse",
        runs: [{ text: "Passer à la caisse" }],
      },
      "de-DE": {
        locale: "de-DE",
        text: "Zur Kasse gehen",
        runs: [{ text: "Zur Kasse gehen" }],
      },
    },
    origins: [
      {
        source: "file",
        key: "store/checkout.json#checkout.cta",
        reference: "a1b2c3d",
        added_at: at(30),
        added_by: "kapi up",
      },
    ],
    unit: "store/checkout.json:checkout.cta",
    created_at: at(30),
    updated_at: at(4),
  },
  {
    id: "2",
    project_id: "kapimart",
    hint_src_lang: "en-US",
    variants: {
      "en-US": {
        locale: "en-US",
        text: "Click here to finish your checkout",
        runs: [
          { text: "Click " },
          { pcOpen: { id: "1", type: "fmt:bold", data: "<b>", equiv: "b" } },
          { text: "here" },
          { pcClose: { id: "1", type: "fmt:bold", data: "</b>", equiv: "b" } },
          { text: " to finish your checkout" },
        ],
      },
      "fr-FR": {
        locale: "fr-FR",
        text: "Cliquez ici pour terminer votre commande",
        runs: [
          { text: "Cliquez " },
          { pcOpen: { id: "1", type: "fmt:bold", data: "<b>", equiv: "b" } },
          { text: "ici" },
          { pcClose: { id: "1", type: "fmt:bold", data: "</b>", equiv: "b" } },
          { text: " pour terminer votre commande" },
        ],
      },
    },
    origins: [
      {
        source: "user",
        key: "docs/checkout.md#p3",
        added_at: at(52),
        added_by: "asgeir",
      },
    ],
    unit: "docs/checkout.md:p3",
    point: { profile: "kapimart-store", channel: "web" },
    note: "Bold marks the link text.",
    created_at: at(52),
    updated_at: at(52),
  },
  {
    id: "3",
    project_id: "",
    hint_src_lang: "en-US",
    variants: {
      "en-US": {
        locale: "en-US",
        text: "Bob added 3 items to the cart",
        runs: [
          { ph: { id: "e1", type: "entity:person", data: "Bob", equiv: "Bob" } },
          { text: " added 3 items to the cart" },
        ],
      },
      "nb-NO": {
        locale: "nb-NO",
        text: "Bob la 3 varer i handlekurven",
        runs: [
          { ph: { id: "e1", type: "entity:person", data: "Bob", equiv: "Bob" } },
          { text: " la 3 varer i handlekurven" },
        ],
      },
    },
    origins: [
      {
        source: "import",
        key: "legacy/store-nb.json",
        added_at: at(400),
        added_by: "kapi memory import",
      },
    ],
    created_at: at(400),
    updated_at: at(400),
  },
];

/** A store of `count` entries in French and German, for the long-list stories. */
export function manyEntries(count: number): MemoryEntryDTO[] {
  return Array.from({ length: count }, (_, i) => ({
    id: `m${i}`,
    project_id: "kapimart",
    hint_src_lang: "en-US",
    variants: {
      "en-US": {
        locale: "en-US",
        text: `Approved sentence number ${i + 1} from the store's checkout pages`,
        runs: [{ text: `Approved sentence number ${i + 1} from the store's checkout pages` }],
      },
      "fr-FR": {
        locale: "fr-FR",
        text: `Phrase approuvée numéro ${i + 1}`,
        runs: [{ text: `Phrase approuvée numéro ${i + 1}` }],
      },
      ...(i % 2 === 0
        ? {
            "de-DE": {
              locale: "de-DE",
              text: `Genehmigter Satz Nummer ${i + 1}`,
              runs: [{ text: `Genehmigter Satz Nummer ${i + 1}` }],
            },
          }
        : {}),
    },
    created_at: at(i + 1),
    updated_at: at(i + 1),
  }));
}

function variantsFrom(input: Record<string, VariantInputDTO>): MemoryEntryDTO["variants"] {
  const variants: MemoryEntryDTO["variants"] = {};
  for (const [locale, v] of Object.entries(input)) {
    variants[locale] = { locale, text: v.text, runs: v.runs ?? [{ text: v.text }] };
  }
  return variants;
}

/** An adapter over an in-memory copy of `entries`, paged as the host pages it. */
export function makeMemoryAdapter(entries: MemoryEntryDTO[] = SAMPLE_ENTRIES): MemoryAdapter {
  let data = structuredClone(entries);
  let seq = 0;
  const matches = (e: MemoryEntryDTO, q: string) =>
    Object.values(e.variants).some((v) => v.text.toLowerCase().includes(q.toLowerCase()));

  return {
    async search(query, _anyLocale, requireLocale, offset, limit) {
      const found = data.filter(
        (e) => (!query || matches(e, query)) && (!requireLocale || !!e.variants[requireLocale]),
      );
      return { entries: found.slice(offset, offset + limit), total_count: found.length };
    },
    async getEntry(id) {
      return data.find((e) => e.id === id) ?? null;
    },
    async addEntry(req) {
      data.unshift({
        id: `new-${++seq}`,
        project_id: req.project_id ?? "",
        hint_src_lang: req.hint_src_lang,
        variants: variantsFrom(req.variants),
        origins: [{ source: "user", added_at: new Date(NOW).toISOString(), added_by: "you" }],
        created_at: new Date(NOW).toISOString(),
        updated_at: new Date(NOW).toISOString(),
      });
    },
    async updateEntry(req) {
      data = data.map((e) =>
        e.id === req.entry_id
          ? { ...e, variants: variantsFrom(req.variants), updated_at: new Date(NOW).toISOString() }
          : e,
      );
    },
    async deleteEntry(id) {
      data = data.filter((e) => e.id !== id);
    },
    async deleteEntries(ids) {
      const gone = new Set(ids);
      data = data.filter((e) => !gone.has(e.id));
    },
    async annotateEntities(req) {
      return { entries_updated: req.entry_ids.length, entities_added: req.patterns.length };
    },
    async getFacets(): Promise<MemoryFacets> {
      const counts = new Map<string, number>();
      for (const e of data) {
        for (const l of Object.keys(e.variants)) counts.set(l, (counts.get(l) ?? 0) + 1);
      }
      return {
        locales: [...counts].map(([locale, count]) => ({ locale, count })),
        projects: [],
        entity_types: [],
        import_sessions: [],
        has_codes: 0,
        no_codes: 0,
      };
    },
    async resolvePoint(entry) {
      return entry.unit ? { profile: "kapimart-store", collection: "store" } : null;
    },
    openUnit() {},
  };
}
