// Content memory entries for the stories and tests, behind an in-memory
// adapter shaped like the host's: a paged search, the store's languages,
// edits, deletes and entity marking all act on the copy held here.

import type {
  MemoryAdapter,
  MemoryEntryDTO,
  MemoryFacets,
  MemorySearchFilter,
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
    entities: [
      {
        placeholder_id: "e1",
        type: "entity:person",
        values: {
          "en-US": { text: "Bob", start: 0, end: 3 },
          "nb-NO": { text: "Bob", start: 0, end: 3 },
        },
      },
    ],
    origins: [
      {
        source: "import",
        key: "legacy/store-nb.json",
        added_at: at(400),
        added_by: "kapi memory import",
        session_id: "imp-1",
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

/** Whether an entry carries inline codes in any of its languages. */
function hasCodes(e: MemoryEntryDTO): boolean {
  return Object.values(e.variants).some((v) => v.runs.some((r) => !("text" in r)));
}

/** An adapter over an in-memory copy of `entries`, paged and faceted as the host does. */
export function makeMemoryAdapter(entries: MemoryEntryDTO[] = SAMPLE_ENTRIES): MemoryAdapter {
  let data = structuredClone(entries);
  let seq = 0;

  const find = (
    query: string,
    anyLocale: string,
    requireLocale: string,
    filter: MemorySearchFilter = {},
  ) =>
    data.filter((e) => {
      const variants = Object.values(e.variants).filter(
        (v) => !anyLocale || v.locale === anyLocale,
      );
      if (query && !variants.some((v) => v.text.toLowerCase().includes(query.toLowerCase())))
        return false;
      if (anyLocale && variants.length === 0) return false;
      if (requireLocale && !e.variants[requireLocale]) return false;
      if (filter.project_id !== undefined && e.project_id !== filter.project_id) return false;
      if (filter.entity_types?.length) {
        const types = new Set((e.entities ?? []).map((m) => m.type));
        if (!filter.entity_types.every((type) => types.has(type))) return false;
      }
      if (filter.session_ids?.length) {
        const sessions = new Set((e.origins ?? []).map((o) => o.session_id));
        if (!filter.session_ids.some((id) => sessions.has(id))) return false;
      }
      if (filter.has_codes === true && !hasCodes(e)) return false;
      if (filter.has_codes === false && hasCodes(e)) return false;
      return true;
    });

  const facetsOf = (found: MemoryEntryDTO[]): MemoryFacets => {
    const count = <K>(keys: K[]) => {
      const m = new Map<K, number>();
      for (const k of keys) m.set(k, (m.get(k) ?? 0) + 1);
      return [...m];
    };
    return {
      locales: count(found.flatMap((e) => Object.keys(e.variants))).map(([locale, n]) => ({
        locale,
        count: n,
      })),
      projects: count(found.map((e) => e.project_id)).map(([project_id, n]) => ({
        project_id,
        count: n,
      })),
      entity_types: count(found.flatMap((e) => (e.entities ?? []).map((m) => m.type))).map(
        ([type, n]) => ({ type, count: n }),
      ),
      import_sessions: count(
        found.flatMap((e) =>
          (e.origins ?? []).filter((o) => o.session_id).map((o) => o.session_id!),
        ),
      ).map(([session_id, n]) => ({
        session_id,
        file_key: "legacy/store-nb.json",
        imported_at: at(400),
        count: n,
      })),
      has_codes: found.filter(hasCodes).length,
      no_codes: found.filter((e) => !hasCodes(e)).length,
    };
  };

  return {
    async search(query, anyLocale, requireLocale, offset, limit) {
      const found = find(query, anyLocale, requireLocale);
      return { entries: found.slice(offset, offset + limit), total_count: found.length };
    },
    async searchFiltered(query, anyLocale, requireLocale, filter, offset, limit) {
      const found = find(query, anyLocale, requireLocale, filter);
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
    async lookup(req) {
      // Word overlap with the source wording stands in for the store's fuzzy match.
      const words = new Set(req.text.toLowerCase().split(/\W+/).filter(Boolean));
      return data
        .map((entry) => {
          const source = entry.variants[entry.hint_src_lang] ?? Object.values(entry.variants)[0];
          const theirs = source.text.toLowerCase().split(/\W+/).filter(Boolean);
          const shared = theirs.filter((w) => words.has(w)).length;
          return { entry, score: theirs.length ? shared / Math.max(theirs.length, words.size) : 0 };
        })
        .filter((m) => m.score >= req.min_score)
        .sort((a, b) => b.score - a.score)
        .slice(0, req.max_results)
        .map((m) => ({
          ...m,
          match_type: m.score === 1 ? "exact" : "fuzzy",
          entity_adaptations: req.entities.length
            ? [
                {
                  placeholder_id: "e1",
                  type: req.entities[0].type,
                  stored_value: "Bob",
                  current_value: req.entities[0].text,
                },
              ]
            : [],
        }));
    },
    async getFacets() {
      return facetsOf(data);
    },
    async getFacetsFiltered(query, anyLocale, requireLocale, filter) {
      return facetsOf(find(query, anyLocale, requireLocale, filter));
    },
    async resolvePoint(entry) {
      return entry.unit ? { profile: "kapimart-store", collection: "store" } : null;
    },
    openUnit() {},
  };
}
