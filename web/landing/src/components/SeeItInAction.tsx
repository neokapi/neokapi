import { PlayCircle, ChevronRight } from 'lucide-react'

/*
  Placeholder section for the WITH/WITHOUT demo gallery.
  Another workstream owns the interactive gallery component and the
  recorded demo assets (web/docs/demos, web/docs/scenes, *.tape).
  This section only frames the heading and links out to the docs.
*/
export function SeeItInAction() {
  return (
    <section id="see-it-in-action" className="relative px-6 py-24">
      <div className="mx-auto max-w-4xl text-center">
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
          Watch an AI assistant draft copy, get scored against a brand profile,
          fix the violations, and ship it in five languages — side by side with
          what happens when it runs unguided.
        </p>

        {/* Demo gallery slot — filled by the walkthroughs/demos workstream */}
        <div className="mt-10 rounded-2xl border border-dashed border-surface-600 bg-surface-900/30 px-6 py-16">
          <p className="text-sm text-neutral-500">
            Interactive walkthroughs and recorded demos live in the docs.
          </p>
          <a
            href="https://neokapi.github.io/web/neokapi/docs/walkthroughs"
            target="_blank"
            rel="noopener"
            className="mt-5 inline-flex items-center gap-2 rounded-xl border border-brand-500/20 bg-brand-500/[0.06] px-5 py-2.5 font-display text-sm font-semibold text-brand-300 transition hover:bg-brand-500/[0.12] hover:text-brand-200"
          >
            Browse the walkthroughs
            <ChevronRight className="h-4 w-4" />
          </a>
        </div>
      </div>
    </section>
  )
}
