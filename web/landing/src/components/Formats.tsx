import { ChevronRight } from "lucide-react";
import { t } from "@neokapi/kapi-react/runtime";

const FORMATS_REFERENCE_URL = "https://neokapi.github.io/web/neokapi/docs/reference/formats";

const FORMAT_GROUPS = [
  {
    category: t("Data"),
    description: t("Structured key–value and tabular files — the catalogs apps ship strings in."),
  },
  {
    category: t("Content"),
    description: t("Authored prose and markup, plus subtitle and caption tracks."),
  },
  {
    category: t("Office & publishing"),
    description: t("Word-processing, spreadsheet, presentation, e-book, and layout documents."),
  },
  {
    // Bilingual interchange — the translator handoff, not the main surface.
    category: t("Interchange"),
    description: t("Bilingual handoff and translation-memory formats for the translator workflow."),
  },
];

export function Formats() {
  return (
    <section id="formats" className="relative px-6 py-24">
      <div className="mx-auto max-w-6xl">
        <div className="mb-12 text-center">
          <h2 className="font-display text-3xl font-bold tracking-tight text-white sm:text-4xl">
            The formats your content lives in
          </h2>
          <p className="mx-auto mt-4 max-w-2xl text-lg text-neutral-400">
            Native readers and writers for localization, data, content, subtitle, and office
            formats, detected by extension, MIME type, or content — with more available through the
            okapi-bridge. A round-trip, in place, not string-and-key extraction.
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
              <p className="mt-4 text-sm leading-relaxed text-neutral-400">{group.description}</p>
            </div>
          ))}
        </div>

        <div className="mt-10 text-center">
          <a
            href={FORMATS_REFERENCE_URL}
            target="_blank"
            rel="noopener"
            className="inline-flex items-center gap-2 rounded-xl border border-brand-500/20 bg-brand-500/[0.06] px-5 py-2.5 font-display text-sm font-semibold text-brand-300 transition hover:bg-brand-500/[0.12] hover:text-brand-200"
          >
            Browse the Format Reference
            <ChevronRight className="h-4 w-4" />
          </a>
        </div>
      </div>
    </section>
  );
}
