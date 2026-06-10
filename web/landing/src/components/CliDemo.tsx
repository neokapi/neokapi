import { useState } from "react";
import { cn } from "@/lib/utils";
import { t } from "@neokapi/kapi-react/runtime";

const TABS = [
  {
    label: t("Extract"),
    cmd: "kapi extract quarterly-report.docx \\\n  -o strings.json",
    description: t(
      "Read any file and pull its text into blocks — structure, styles, fields, and placeholders preserved for a faithful round-trip back.",
    ),
  },
  {
    label: t("Merge"),
    cmd: "kapi merge strings.de.json \\\n  --skeleton quarterly-report.docx \\\n  -o quarterly-report.de.docx",
    description: t(
      "Write the changed text back into the original file, with markup, inline tags, and placeholders intact.",
    ),
  },
  {
    label: t("Translate"),
    cmd: "kapi run ai-translate-qa \\\n  -i src/en.json -o src/fr.json \\\n  --source-lang en --target-lang fr",
    description: t(
      "Translate with an LLM or MT backend and run QA in one flow, placeholders and markup preserved throughout.",
    ),
  },
  {
    label: t("Terminology"),
    cmd: "kapi termbase import glossary.csv \\\n  --format csv \\\n  -s en \\\n  -t fr",
    description: t(
      "Import terminology from CSV, JSON, or TBX. Enforce preferred and forbidden terms across every translation with term-enforce.",
    ),
  },
  {
    label: t("Brand check"),
    cmd: "kapi brand check \\\n  --profile-file acme.yaml \\\n  --min-score 80 \\\n  release-notes.md",
    description: t(
      "Score text 0–100 across tone, style, vocabulary, clarity, and compliance. --min-score gates CI and exits 3 below the threshold.",
    ),
  },
  {
    label: t("Formats"),
    cmd: "kapi formats\n\n# native readers and writers for localization,\n# data, content, subtitle, and office formats,\n# detected by extension, MIME type, or content\n# — more available through the okapi-bridge",
    description: t(
      "List the supported file formats. neokapi detects formats by extension, MIME type, or content sniffing.",
    ),
  },
  {
    label: t("MCP"),
    cmd: "kapi mcp\n\n# exposes the engine to any MCP client:\n#   extract   translate   check\n#   term_lookup   tm_search\n# Claude Code · Cursor · Windsurf · ...",
    description: t(
      "Serve the engine — extract, translate, check, term and TM lookup — to your AI assistant over MCP.",
    ),
  },
];

export function CliDemo() {
  const [active, setActive] = useState(0);

  return (
    <section id="cli" className="relative px-6 py-24">
      <div className="mx-auto max-w-6xl">
        <div className="mb-16 text-center">
          <h2 className="font-display text-3xl font-bold tracking-tight text-white sm:text-4xl">
            The{" "}
            <span
              translate="no"
              className="rounded-lg bg-surface-800 px-2 py-1 font-mono text-brand-400"
            >
              kapi
            </span>{" "}
            CLI
          </h2>
          <p className="mx-auto mt-4 max-w-2xl text-lg text-neutral-400">
            Extract, translate, check, and write files back in place — directly from the terminal.
            No project setup, no server, no configuration needed.
          </p>
        </div>

        <div className="grid gap-8 lg:grid-cols-[1fr_1.4fr]">
          {/* Tab list */}
          <div className="flex flex-col gap-2">
            {TABS.map((tab, i) => (
              <button
                key={tab.label}
                onClick={() => setActive(i)}
                className={cn(
                  "group rounded-xl px-5 py-4 text-left transition-all duration-200",
                  i === active
                    ? "border border-brand-500/20 bg-brand-500/[0.06] shadow-lg shadow-brand-500/[0.03]"
                    : "border border-transparent hover:border-surface-600 hover:bg-surface-800/50",
                )}
              >
                <div className="flex items-center gap-3">
                  <div
                    className={cn(
                      "h-2 w-2 rounded-full transition-colors",
                      i === active ? "bg-brand-400" : "bg-surface-600 group-hover:bg-surface-500",
                    )}
                  />
                  <span
                    className={cn(
                      "font-display text-sm font-semibold transition-colors",
                      i === active
                        ? "text-brand-300"
                        : "text-neutral-400 group-hover:text-neutral-300",
                    )}
                  >
                    {tab.label}
                  </span>
                </div>
                <p
                  className={cn(
                    "mt-2 ml-5 text-sm leading-relaxed transition-colors",
                    i === active ? "text-neutral-300" : "text-neutral-500",
                  )}
                >
                  {tab.description}
                </p>
              </button>
            ))}
          </div>

          {/* Code panel */}
          <div className="terminal-window overflow-hidden rounded-2xl glow-teal">
            <div className="flex items-center gap-2 border-b border-brand-500/8 px-5 py-3">
              <div className="h-2.5 w-2.5 rounded-full bg-accent-rose/50" />
              <div className="h-2.5 w-2.5 rounded-full bg-accent-amber/50" />
              <div className="h-2.5 w-2.5 rounded-full bg-brand-500/50" />
              <span translate="no" className="ml-3 font-mono text-xs text-neutral-600">
                terminal
              </span>
            </div>
            <div className="p-6">
              <pre translate="no" className="font-mono text-sm leading-relaxed">
                <span className="select-none text-brand-400">$ </span>
                <span className="text-neutral-200">{TABS[active].cmd}</span>
              </pre>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
