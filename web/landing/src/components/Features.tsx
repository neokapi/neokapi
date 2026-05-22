import {
  Sparkles, Gauge, Languages, BookMarked,
  WifiOff, Plug, FileText, Workflow,
} from 'lucide-react'

const FEATURES = [
  {
    icon: Gauge,
    title: 'Brand voice, scored',
    description:
      'Load a voice profile and score any text 0–100 across tone, style, vocabulary, clarity, and compliance. `--min-score` turns it into a CI gate (exit code 3 on failure). Five starter packs included.',
    accent: 'text-brand-400',
    bg: 'bg-brand-500/8',
    border: 'border-brand-500/15',
  },
  {
    icon: Sparkles,
    title: 'On-brand at generation',
    description:
      'A bound profile is injected into the translation and rewrite prompts, so AI output is on-voice and terminology-correct when it is written — not just flagged afterward.',
    accent: 'text-accent-cyan',
    bg: 'bg-accent-cyan/8',
    border: 'border-accent-cyan/15',
  },
  {
    icon: BookMarked,
    title: 'Terminology + TM',
    description:
      'Import a termbase (CSV/JSON/TBX), enforce preferred and forbidden terms, and leverage a TMX translation memory with fuzzy matching — the same enforcement path as brand vocabulary.',
    accent: 'text-accent-amber',
    bg: 'bg-accent-amber/8',
    border: 'border-accent-amber/15',
  },
  {
    icon: WifiOff,
    title: 'Offline by default',
    description:
      'A single self-contained binary with SQLite-backed TM and termbase. Run local models with Ollama. Nothing leaves your machine unless you choose a cloud LLM or attach a server.',
    accent: 'text-forest-400',
    bg: 'bg-forest-400/8',
    border: 'border-forest-400/15',
  },
  {
    icon: Plug,
    title: 'Drops into your AI workflow',
    description:
      '`kapi mcp` exposes brand_guide, brand_check, brand_rewrite, term_lookup, and tm_search to any MCP client — Claude Code, Cursor, Windsurf, and more.',
    accent: 'text-accent-rose',
    bg: 'bg-accent-rose/8',
    border: 'border-accent-rose/15',
  },
  {
    icon: FileText,
    title: '30+ formats, natively',
    description:
      'HTML, Markdown, JSON, YAML, XML, PO, properties, .strings, XLIFF, TMX, subtitles and more — read and write in place, with 57+ Okapi-bridge filters for Office, EPUB, PDF, and InDesign.',
    accent: 'text-brand-400',
    bg: 'bg-brand-500/8',
    border: 'border-brand-500/15',
  },
  {
    icon: Languages,
    title: 'Translate, then QA',
    description:
      'AI translation with LLM and MT backends, rule-based and AI QA checks, terminology enforcement, and pseudo-translation — composable into flows with `kapi run`.',
    accent: 'text-accent-cyan',
    bg: 'bg-accent-cyan/8',
    border: 'border-accent-cyan/15',
  },
  {
    icon: Workflow,
    title: 'Governance you own',
    description:
      'Gate brand compliance in CI with `kapi brand check --min-score`, share profiles and termbases as version-controlled files, and connect a server when a team needs shared trends and automation — self-hosted or a managed platform. Open core, no lock-in.',
    accent: 'text-forest-400',
    bg: 'bg-forest-400/8',
    border: 'border-forest-400/15',
  },
]

export function Features() {
  return (
    <section id="features" className="relative px-6 py-24">
      <div className="mx-auto max-w-6xl">
        <div className="mb-16 text-center">
          <h2 className="font-display text-3xl font-bold tracking-tight text-white sm:text-4xl">
            One engine for{' '}
            <span className="bg-gradient-to-r from-brand-400 to-brand-300 bg-clip-text text-transparent">
              brand, terminology, and localization
            </span>
          </h2>
          <p className="mx-auto mt-4 max-w-2xl text-lg text-neutral-400">
            Brand governance for your AI output and a format-aware localization
            toolkit — built from the same composable pieces.
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
              <h3 className="font-display text-lg font-semibold text-white">
                {f.title}
              </h3>
              <p className="mt-2 text-sm leading-relaxed text-neutral-400">
                {f.description}
              </p>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
