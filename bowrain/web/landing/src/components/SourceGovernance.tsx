import { GitBranch, Sliders, TrendingUp } from 'lucide-react'

export function SourceGovernance() {
  return (
    <section className="mx-auto max-w-6xl px-6 py-24">
      <div className="mx-auto max-w-3xl text-center">
        <h2 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">
          One brand voice and terminology base,{' '}
          <span className="text-neutral-500">shared and stewarded.</span>
        </h2>
        <p className="mt-3 text-neutral-400">
          Bowrain holds the brand voice profile, terminology, and translation memory in one place, applied the same way
          for every team member, every channel, and every AI tool — and kept current as people correct and refine it.
        </p>
      </div>

      <div className="mt-12 grid gap-6 md:grid-cols-3">
        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
          <div className="mb-4 flex h-10 w-10 items-center justify-center rounded-lg bg-brand-500/10">
            <GitBranch className="h-5 w-5 text-brand-400" />
          </div>
          <h3 className="text-lg font-semibold text-white">Terminology enforcement</h3>
          <p className="mt-2 text-sm text-neutral-400">
            Preferred terms, forbidden words, competitor name handling. Applied consistently across every team member and every piece of content.
          </p>
          <div className="mt-4 space-y-2 font-mono text-xs">
            <div className="flex items-center gap-2">
              <span className="text-forbidden">✗</span>
              <span className="text-neutral-500 line-through">leverage</span>
              <span className="text-neutral-600">→</span>
              <span className="text-suggestion">use</span>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-forbidden">✗</span>
              <span className="text-neutral-500 line-through">Copilot</span>
              <span className="text-neutral-600">→</span>
              <span className="text-suggestion">AI assistant</span>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-forbidden">✗</span>
              <span className="text-neutral-500 line-through">synergy</span>
              <span className="text-neutral-600">→</span>
              <span className="text-suggestion">collaboration</span>
            </div>
          </div>
        </div>

        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
          <div className="mb-4 flex h-10 w-10 items-center justify-center rounded-lg bg-brand-500/10">
            <Sliders className="h-5 w-5 text-brand-400" />
          </div>
          <h3 className="text-lg font-semibold text-white">Channel overrides</h3>
          <p className="mt-2 text-sm text-neutral-400">
            Docs are precise. Marketing is warm. Support is empathetic. Same brand, different voice. All from one profile.
          </p>
          <div className="mt-4 space-y-2 text-xs">
            <div className="flex items-center justify-between rounded-md bg-neutral-800/50 px-3 py-2">
              <span className="text-neutral-300">Documentation</span>
              <span className="font-mono text-neutral-500">formal, precise</span>
            </div>
            <div className="flex items-center justify-between rounded-md bg-neutral-800/50 px-3 py-2">
              <span className="text-neutral-300">Marketing</span>
              <span className="font-mono text-neutral-500">casual, warm</span>
            </div>
            <div className="flex items-center justify-between rounded-md bg-neutral-800/50 px-3 py-2">
              <span className="text-neutral-300">Support</span>
              <span className="font-mono text-neutral-500">empathetic, helpful</span>
            </div>
          </div>
        </div>

        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
          <div className="mb-4 flex h-10 w-10 items-center justify-center rounded-lg bg-brand-500/10">
            <TrendingUp className="h-5 w-5 text-brand-400" />
          </div>
          <h3 className="text-lg font-semibold text-white">The feedback loop</h3>
          <p className="mt-2 text-sm text-neutral-400">
            Human corrections feed back into the shared profile. Terminology and translation memory update once and apply to everyone, so the next suggestion reflects the team's prior decisions.
          </p>
          <div className="mt-4 space-y-2 text-xs">
            <div className="flex items-center gap-2 text-neutral-400">
              <span className="text-brand-400">1.</span> AI suggests content
            </div>
            <div className="flex items-center gap-2 text-neutral-400">
              <span className="text-brand-400">2.</span> Human corrects wording
            </div>
            <div className="flex items-center gap-2 text-neutral-400">
              <span className="text-brand-400">3.</span> Correction feeds back into profile
            </div>
            <div className="flex items-center gap-2 text-neutral-400">
              <span className="text-brand-400">4.</span> Next suggestion is better
            </div>
          </div>
        </div>
      </div>

      {/* Brand Compliance Score visual */}
      <div className="mx-auto mt-12 max-w-2xl rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
        <div className="mb-4 text-center text-sm font-medium text-white">Brand Compliance Score — Five Dimensions</div>
        <div className="space-y-3">
          {[
            { label: 'Tone', score: 92, color: 'bg-brand-400' },
            { label: 'Style', score: 88, color: 'bg-purple-400' },
            { label: 'Vocabulary', score: 95, color: 'bg-suggestion' },
            { label: 'Clarity', score: 78, color: 'bg-yellow-400' },
            { label: 'Brand Compliance', score: 100, color: 'bg-brand-500' },
          ].map(dim => (
            <div key={dim.label} className="flex items-center gap-4">
              <div className="w-32 text-right text-xs text-neutral-400">{dim.label}</div>
              <div className="h-2 flex-1 overflow-hidden rounded-full bg-neutral-800">
                <div className={`h-full rounded-full ${dim.color} animate-score-fill`} style={{ width: `${dim.score}%` }} />
              </div>
              <div className="w-8 text-right font-mono text-xs text-neutral-300">{dim.score}</div>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
