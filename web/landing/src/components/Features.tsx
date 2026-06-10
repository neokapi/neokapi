import {
  FileText,
  Repeat,
  Languages,
  BookMarked,
  ShieldCheck,
  WifiOff,
  Plug,
  Workflow,
} from "lucide-react";
import { t } from "@neokapi/kapi-react/runtime";

const FEATURES = [
  {
    icon: FileText,
    title: t("Every format, natively"),
    description: t(
      "Read and write localization, data, content, subtitle, and office formats in place — with more through the okapi-bridge. The full list lives in the Format Reference.",
    ),
    accent: "text-brand-400",
    bg: "bg-brand-500/8",
    border: "border-brand-500/15",
  },
  {
    icon: Repeat,
    title: t("Faithful round-trip"),
    description: t(
      "Extract translatable text, change it, and write the original file back — structure, markup, inline tags, and placeholders intact. `kapi extract` then `kapi merge`, unchanged except where you intended.",
    ),
    accent: "text-accent-cyan",
    bg: "bg-accent-cyan/8",
    border: "border-accent-cyan/15",
  },
  {
    icon: Languages,
    title: t("Translate with AI or MT"),
    description: t(
      "AI translation with LLM and MT backends, terminology enforcement, and pseudo-translation — composable into flows with `kapi run`, placeholders and markup preserved throughout.",
    ),
    accent: "text-accent-amber",
    bg: "bg-accent-amber/8",
    border: "border-accent-amber/15",
  },
  {
    icon: BookMarked,
    title: t("Reuse with translation memory"),
    description: t(
      "Leverage a TMX translation memory with fuzzy matching and import a termbase (CSV/JSON/TBX) to keep preferred and forbidden terms consistent across everything you ship.",
    ),
    accent: "text-forest-400",
    bg: "bg-forest-400/8",
    border: "border-forest-400/15",
  },
  {
    icon: ShieldCheck,
    title: t("Checks that act like tests"),
    description: t(
      "Verify AI output against your rules — do-not-translate, placeholder and tag integrity, terminology, and brand voice — and gate CI with `kapi verify`, which exits non-zero on failure so a regression never ships.",
    ),
    accent: "text-accent-rose",
    bg: "bg-accent-rose/8",
    border: "border-accent-rose/15",
  },
  {
    icon: WifiOff,
    title: t("Offline by default"),
    description: t(
      "A single self-contained binary with SQLite-backed TM and termbase. Run local models with Ollama. Nothing leaves your machine unless you choose a cloud LLM.",
    ),
    accent: "text-brand-400",
    bg: "bg-brand-500/8",
    border: "border-brand-500/15",
  },
  {
    icon: Plug,
    title: t("Drops into your AI workflow"),
    description: t(
      "`kapi mcp` exposes the engine — extract, translate, check, term and TM lookup — to any MCP client: Claude Code, Cursor, Windsurf, and more.",
    ),
    accent: "text-accent-cyan",
    bg: "bg-accent-cyan/8",
    border: "border-accent-cyan/15",
  },
  {
    icon: Workflow,
    title: t("Versioned and CI-gated"),
    description: t(
      "Keep recipes, brand profiles, and termbases as version-controlled files alongside your code, and gate quality in CI. Open source, offline, no lock-in.",
    ),
    accent: "text-forest-400",
    bg: "bg-forest-400/8",
    border: "border-forest-400/15",
  },
];

export function Features() {
  return (
    <section id="features" className="relative px-6 py-24">
      <div className="mx-auto max-w-6xl">
        <div className="mb-16 text-center">
          <h2 className="font-display text-3xl font-bold tracking-tight text-white sm:text-4xl">
            One engine for{" "}
            <span className="bg-gradient-to-r from-brand-400 to-brand-300 bg-clip-text text-transparent">
              every content format
            </span>
          </h2>
          <p className="mx-auto mt-4 max-w-2xl text-lg text-neutral-400">
            Extract, translate, check, and transform — then write the original back, faithfully.
            Built from composable pieces.
          </p>
        </div>

        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {FEATURES.map((f) => (
            <div
              key={f.title}
              className={`group rounded-2xl border ${f.border} ${f.bg} p-6 transition-all duration-300 hover:border-opacity-40 hover:shadow-lg hover:shadow-brand-500/[0.03]`}
            >
              <div className={`mb-4 inline-flex rounded-xl ${f.bg} p-2.5`}>
                <f.icon className={`h-5 w-5 ${f.accent}`} />
              </div>
              <h3 className="font-display text-lg font-semibold text-white">{f.title}</h3>
              <p className="mt-2 text-sm leading-relaxed text-neutral-400">{f.description}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
