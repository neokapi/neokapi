const FORMAT_GROUPS = [
  {
    category: "Data",
    formats: ["JSON", "YAML", "CSV", "TSV", "XML"],
  },
  {
    category: "Content",
    formats: ["HTML", "Markdown", "Plaintext", "SRT", "VTT", "DTD"],
  },
  {
    category: "Office & publishing",
    formats: ["DOCX", "XLSX", "PPTX", "ODF", "EPUB", "PDF", "IDML", "MIF"],
  },
  {
    // Bilingual interchange — the translator handoff, not the main surface.
    category: "Interchange",
    formats: ["XLIFF 1.2", "XLIFF 2.0", "PO/POT", "TMX", "Qt TS", "Properties"],
  },
];

export function Formats() {
  return (
    <section id="formats" className="relative px-6 py-24">
      <div className="mx-auto max-w-6xl">
        <div className="mb-12 text-center">
          <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-brand-500/15 bg-brand-500/[0.06] px-3.5 py-1.5 text-xs font-medium text-brand-300">
            Still a format-aware localization toolkit
          </div>
          <h2 className="font-display text-3xl font-bold tracking-tight text-white sm:text-4xl">
            The formats your content lives in
          </h2>
          <p className="mx-auto mt-4 max-w-2xl text-lg text-neutral-400">
            Native readers and writers for localization, data, content, and office formats, detected
            by extension, MIME type, or content — with more available through the okapi-bridge.
            Round-trip, in place, not string-and-key extraction.
          </p>
        </div>

        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
          {FORMAT_GROUPS.map((group) => (
            <div
              key={group.category}
              className="rounded-2xl border border-surface-700/50 bg-surface-900/40 p-6"
            >
              <h3 className="font-display text-sm font-semibold uppercase tracking-wider text-brand-400">
                {group.category}
              </h3>
              <div className="mt-4 flex flex-wrap gap-2">
                {group.formats.map((fmt) => (
                  <span
                    key={fmt}
                    className="rounded-lg border border-surface-700/50 bg-surface-800/60 px-2.5 py-1 font-mono text-xs text-neutral-300"
                  >
                    {fmt}
                  </span>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
