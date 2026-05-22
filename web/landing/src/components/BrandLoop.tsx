import { BookOpen, Sparkles, Gauge, Wand2, Globe, ShieldCheck } from 'lucide-react'

const STEPS = [
  {
    n: '01',
    icon: BookOpen,
    title: 'Know the brand',
    cmd: 'kapi brand guide',
    description:
      'Load a machine-readable voice profile — tone, style, preferred and forbidden vocabulary — into context. Start from one of five built-in packs or your own.',
    accent: 'text-brand-400',
    bg: 'bg-brand-500/8',
    border: 'border-brand-500/15',
  },
  {
    n: '02',
    icon: Sparkles,
    title: 'Generate',
    cmd: 'in Claude Code · Cursor · MCP',
    description:
      'Your AI coding assistant writes UI strings, docs, and copy with the brand profile injected — so output is on-voice and on-terminology at generation time, not bolted on after.',
    accent: 'text-accent-cyan',
    bg: 'bg-accent-cyan/8',
    border: 'border-accent-cyan/15',
  },
  {
    n: '03',
    icon: Gauge,
    title: 'Check',
    cmd: 'kapi brand check --min-score 80',
    description:
      'Score any text against the profile — 0–100 across tone, style, vocabulary, clarity, and compliance. Use --min-score as a CI gate that fails the build below threshold.',
    accent: 'text-accent-amber',
    bg: 'bg-accent-amber/8',
    border: 'border-accent-amber/15',
  },
  {
    n: '04',
    icon: Wand2,
    title: 'Fix',
    cmd: 'kapi brand rewrite',
    description:
      'Rewrite content to resolve forbidden terms, competitor mentions, and off-voice phrasing — keeping meaning intact while bringing the score up.',
    accent: 'text-accent-rose',
    bg: 'bg-accent-rose/8',
    border: 'border-accent-rose/15',
  },
  {
    n: '05',
    icon: Globe,
    title: 'Publish',
    cmd: 'kapi run ai-translate-qa …',
    description:
      'Translate into every locale — brand-voice-aware, terminology-enforced, with TM leverage — and write back into the same native formats (with more through the okapi-bridge).',
    accent: 'text-forest-400',
    bg: 'bg-forest-400/8',
    border: 'border-forest-400/15',
  },
  {
    n: '06',
    icon: ShieldCheck,
    title: 'Govern',
    cmd: 'kapi brand check --min-score',
    description:
      'Gate brand compliance in CI and share profiles as version-controlled YAML. When a team needs shared trends, connectors, and automation, connect a server — self-hosted or a managed platform.',
    accent: 'text-brand-400',
    bg: 'bg-brand-500/8',
    border: 'border-brand-500/15',
  },
]

export function BrandLoop() {
  return (
    <section id="brand-loop" className="relative px-6 py-24">
      <div className="mx-auto max-w-6xl">
        <div className="mb-16 text-center">
          <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-brand-500/15 bg-brand-500/[0.06] px-3.5 py-1.5 text-xs font-medium text-brand-300">
            The brand-consistency loop
          </div>
          <h2 className="font-display text-3xl font-bold tracking-tight text-white sm:text-4xl">
            From a brand guide to{' '}
            <span className="bg-gradient-to-r from-brand-400 to-brand-300 bg-clip-text text-transparent">
              shipped, on-brand, multilingual
            </span>
          </h2>
          <p className="mx-auto mt-4 max-w-2xl text-lg text-neutral-400">
            One engine spans the whole loop. AI generates; kapi keeps it on-voice
            and consistent, then publishes it everywhere.
          </p>
        </div>

        {/* Step rail */}
        <div className="mb-10 hidden items-center justify-between gap-1 lg:flex">
          {STEPS.map((s, i) => (
            <div key={s.n} className="flex items-center gap-1">
              <span className={`font-mono text-xs font-semibold ${s.accent}`}>{s.title}</span>
              {i < STEPS.length - 1 && (
                <span className="mx-1 text-surface-600">&rarr;</span>
              )}
            </div>
          ))}
        </div>

        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {STEPS.map((s) => (
            <div
              key={s.n}
              className={`group relative rounded-2xl border ${s.border} ${s.bg} p-6 transition-all duration-300 hover:shadow-lg hover:shadow-brand-500/[0.03]`}
            >
              <div className="flex items-start justify-between">
                <div className={`inline-flex rounded-xl ${s.bg} p-2.5`}>
                  <s.icon className={`h-5 w-5 ${s.accent}`} />
                </div>
                <span className="font-mono text-xs text-neutral-600">{s.n}</span>
              </div>
              <h3 className="mt-4 font-display text-lg font-semibold text-white">
                {s.title}
              </h3>
              <code className={`mt-1 block font-mono text-[11px] ${s.accent}`}>
                {s.cmd}
              </code>
              <p className="mt-2.5 text-sm leading-relaxed text-neutral-400">
                {s.description}
              </p>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
