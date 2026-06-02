import { ArrowRight } from "lucide-react";

const MAPPINGS = [
  { okapi: "Filter", neokapi: "DataFormat", desc: "Reader/Writer" },
  { okapi: "Event", neokapi: "Part", desc: "Processing unit" },
  { okapi: "Step", neokapi: "Tool", desc: "Task unit" },
  { okapi: "Pipeline", neokapi: "Flow", desc: "Tool orchestration" },
  { okapi: "TextUnit", neokapi: "Block", desc: "Translatable content" },
  { okapi: "TextFragment", neokapi: "[]Run", desc: "Run sequence" },
  { okapi: "Code", neokapi: "Run", desc: "Inline markup (Ph/Pc)" },
  { okapi: "Tikal", neokapi: "kapi", desc: "CLI tool" },
  { okapi: "Rainbow", neokapi: "Kapi (app)", desc: "Desktop app" },
];

export function OkapiMapping() {
  return (
    <section className="relative px-6 py-24">
      <div className="mx-auto max-w-4xl">
        <div className="mb-12 text-center">
          <h2 className="font-display text-3xl font-bold tracking-tight text-white sm:text-4xl">
            Built on{" "}
            <span className="bg-gradient-to-r from-accent-amber to-brand-400 bg-clip-text text-transparent">
              Okapi's foundations
            </span>
          </h2>
          <p className="mx-auto mt-4 max-w-2xl text-lg text-neutral-400">
            Neokapi reimagines the proven Okapi Framework in Go for native speed and near-instant
            local processing. Experimental AI-based rewrites of Java filters run natively, while a
            bridge to Java gives full access to battle-tested Okapi filters and steps.
          </p>
        </div>

        <div className="overflow-hidden rounded-2xl border border-surface-700/50 bg-surface-900/40">
          {/* Header */}
          <div className="grid grid-cols-[1fr_auto_1fr_1fr] gap-4 border-b border-surface-700/50 px-6 py-3">
            <span className="font-display text-xs font-semibold uppercase tracking-wider text-neutral-500">
              Okapi (Java)
            </span>
            <span />
            <span className="font-display text-xs font-semibold uppercase tracking-wider text-brand-400">
              Neokapi (Go)
            </span>
            <span className="font-display text-xs font-semibold uppercase tracking-wider text-neutral-600 hidden sm:block">
              Purpose
            </span>
          </div>

          {/* Rows */}
          {MAPPINGS.map((m, i) => (
            <div
              key={m.okapi}
              className={`grid grid-cols-[1fr_auto_1fr_1fr] items-center gap-4 px-6 py-3 ${
                i < MAPPINGS.length - 1 ? "border-b border-surface-800/50" : ""
              }`}
            >
              <code className="font-mono text-sm text-neutral-400">{m.okapi}</code>
              <ArrowRight className="h-3.5 w-3.5 text-surface-600" />
              <code className="font-mono text-sm font-medium text-brand-300">{m.neokapi}</code>
              <span className="text-sm text-neutral-500 hidden sm:block">{m.desc}</span>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
