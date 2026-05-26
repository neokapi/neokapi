import { PlayCircle, ChevronRight, XCircle, CheckCircle2 } from 'lucide-react'

/*
  WITH/WITHOUT scorecard. The numbers below are the real output of the
  `brand-rewrite-marketing` demo cell (web/docs/demos/brand-rewrite-marketing):
  a generic marketing draft scores 70/100 against the marketing-blog brand
  voice; `kapi brand rewrite` lifts it to 95/100. Regenerate with that cell's
  run.sh. Interactive walkthroughs + the full gallery live in the docs.
*/
export function SeeItInAction() {
  return (
    <section id="see-it-in-action" className="relative px-6 py-24">
      <div className="mx-auto max-w-5xl text-center">
        <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-brand-500/15 bg-brand-500/[0.06] px-3.5 py-1.5 text-xs font-medium text-brand-300">
          <PlayCircle className="h-3.5 w-3.5" />
          See it in action
        </div>
        <h2 className="font-display text-3xl font-bold tracking-tight text-white sm:text-4xl">
          With kapi vs.{' '}
          <span className="bg-gradient-to-r from-brand-400 to-brand-300 bg-clip-text text-transparent">
            without
          </span>
        </h2>
        <p className="mx-auto mt-4 max-w-2xl text-lg text-neutral-400">
          A generic AI draft for a marketing blog drifts off-voice. kapi scores it
          against the brand profile and rewrites the violations — the same
          deterministic scorer the product ships.
        </p>

        <div className="mt-12 grid gap-5 text-left sm:grid-cols-2">
          {/* WITHOUT */}
          <div className="rounded-2xl border border-red-500/20 bg-red-500/[0.04] p-6">
            <div className="flex items-center justify-between">
              <span className="inline-flex items-center gap-1.5 text-sm font-medium text-red-300">
                <XCircle className="h-4 w-4" /> AI alone
              </span>
              <span className="font-display text-3xl font-bold text-red-400">70<span className="text-base text-neutral-500">/100</span></span>
            </div>
            <p className="mt-4 text-sm leading-relaxed text-neutral-400">
              “<span className="rounded bg-red-500/15 px-1 text-red-300">In order to</span> ship reliable software,{' '}
              <span className="rounded bg-red-500/15 px-1 text-red-300">it goes without saying</span> that teams need a strategy.{' '}
              <span className="rounded bg-red-500/15 px-1 text-red-300">At the end of the day</span>, any{' '}
              <span className="rounded bg-red-500/15 px-1 text-red-300">thought leader</span> will tell you the same.”
            </p>
            <p className="mt-4 text-xs text-neutral-500">6 brand-voice findings · wordy filler, off-voice</p>
          </div>

          {/* WITH */}
          <div className="rounded-2xl border border-emerald-500/25 bg-emerald-500/[0.05] p-6">
            <div className="flex items-center justify-between">
              <span className="inline-flex items-center gap-1.5 text-sm font-medium text-emerald-300">
                <CheckCircle2 className="h-4 w-4" /> AI + kapi
              </span>
              <span className="font-display text-3xl font-bold text-emerald-400">95<span className="text-base text-neutral-500">/100</span></span>
            </div>
            <p className="mt-4 text-sm leading-relaxed text-neutral-300">
              “<span className="rounded bg-emerald-500/15 px-1 text-emerald-300">to</span> ship reliable software, teams need a strategy.{' '}
              <span className="rounded bg-emerald-500/15 px-1 text-emerald-300">ultimately</span>, any{' '}
              <span className="rounded bg-emerald-500/15 px-1 text-emerald-300">expert</span> will tell you the same.”
            </p>
            <p className="mt-4 text-xs text-neutral-500">1 finding · on the marketing-blog voice</p>
          </div>
        </div>

        <div className="mt-6 inline-flex items-center gap-2 rounded-full border border-emerald-500/20 bg-emerald-500/[0.06] px-4 py-1.5 font-display text-sm font-semibold text-emerald-300">
          +25 brand compliance · one command: <code className="font-mono text-emerald-200">kapi brand rewrite</code>
        </div>

        <div className="mt-10">
          <a
            href="https://neokapi.github.io/web/neokapi/docs/"
            target="_blank"
            rel="noopener"
            className="inline-flex items-center gap-2 rounded-xl border border-brand-500/20 bg-brand-500/[0.06] px-5 py-2.5 font-display text-sm font-semibold text-brand-300 transition hover:bg-brand-500/[0.12] hover:text-brand-200"
          >
            Explore the docs
            <ChevronRight className="h-4 w-4" />
          </a>
        </div>
      </div>
    </section>
  )
}
