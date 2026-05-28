import { WifiOff, Layers, Boxes } from 'lucide-react'

const AXES = [
  {
    icon: WifiOff,
    title: 'Local-first',
    description:
      'A single binary with embedded TM and termbase. Run entirely offline with local models, or opt into a cloud LLM — your call, not a default.',
    contrast: 'Cloud-locked MCPs require their backend for every check.',
    accent: 'text-brand-400',
    glow: 'from-brand-500/20',
  },
  {
    icon: Layers,
    title: 'Brand + terminology + l10n, unified',
    description:
      'Voice scoring, term enforcement, and translation share one model and one enforcement path — so the words your AI writes stay on-brand through every language.',
    contrast: 'Prompt-only brand skills have no engine, scoring, or l10n.',
    accent: 'text-accent-cyan',
    glow: 'from-accent-cyan/20',
  },
  {
    icon: Boxes,
    title: 'Any asset format',
    description:
      'Native readers and writers for localization, document, data, subtitle, and office formats — DOCX, XLSX, PPTX, HTML, Markdown, JSON, YAML, XML, PO, subtitles, InDesign — with more through the okapi-bridge. Round-trip, not string-and-key extraction.',
    contrast: 'String-centric tools stop at JSON keys and PO files.',
    accent: 'text-forest-400',
    glow: 'from-forest-500/20',
  },
]

export function Differentiators() {
  return (
    <section className="relative px-6 py-24">
      <div className="mx-auto max-w-6xl">
        <div className="mb-16 text-center">
          <h2 className="font-display text-3xl font-bold tracking-tight text-white sm:text-4xl">
            Three axes{' '}
            <span className="bg-gradient-to-r from-brand-400 to-forest-400 bg-clip-text text-transparent">
              no one else spans
            </span>
          </h2>
          <p className="mx-auto mt-4 max-w-2xl text-lg text-neutral-400">
            Writing tools, brand-voice prompts, and localization MCPs each own one
            slice. neokapi is built across all three.
          </p>
        </div>

        <div className="grid gap-5 lg:grid-cols-3">
          {AXES.map((a) => (
            <div
              key={a.title}
              className="group relative overflow-hidden rounded-2xl border border-surface-700/60 bg-surface-900/40 p-7"
            >
              <div
                className={`pointer-events-none absolute -right-10 -top-10 h-32 w-32 rounded-full bg-gradient-to-br ${a.glow} to-transparent opacity-60 blur-2xl`}
              />
              <div className="relative">
                <div className="inline-flex rounded-xl bg-surface-800/80 p-3">
                  <a.icon className={`h-6 w-6 ${a.accent}`} />
                </div>
                <h3 className="mt-5 font-display text-xl font-semibold text-white">
                  {a.title}
                </h3>
                <p className="mt-3 text-sm leading-relaxed text-neutral-400">
                  {a.description}
                </p>
                <p className="mt-5 border-t border-surface-700/50 pt-4 text-xs leading-relaxed text-neutral-600">
                  <span className="font-semibold text-neutral-500">vs. the field: </span>
                  {a.contrast}
                </p>
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
