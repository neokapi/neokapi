import { Package, Pen, Layers } from 'lucide-react'

export function Problem() {
  return (
    <section className="mx-auto max-w-6xl px-6 py-24">
      <div className="mx-auto max-w-3xl text-center">
        <h2 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">
          The voice of low-friction
        </h2>
        <p className="mt-3 text-neutral-400">
          Localization should be as natural as adding linting and unit tests to your workflow.
        </p>
      </div>

      <div className="mt-12 grid gap-6 md:grid-cols-3">
        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
          <div className="mb-4 flex h-10 w-10 items-center justify-center rounded-lg bg-brand-500/10">
            <Package className="h-5 w-5 text-brand-400" />
          </div>
          <h3 className="text-lg font-semibold text-white">Multilingual from day one</h3>
          <p className="mt-2 text-sm leading-relaxed text-neutral-400">
            Pseudo-localize your strings in seconds to prove your pipeline is world-ready — before you translate a single word.
          </p>
        </div>

        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
          <div className="mb-4 flex h-10 w-10 items-center justify-center rounded-lg bg-suggestion/10">
            <Pen className="h-5 w-5 text-suggestion" />
          </div>
          <h3 className="text-lg font-semibold text-white">Source language on autopilot</h3>
          <p className="mt-2 text-sm leading-relaxed text-neutral-400">
            Terminology, brand voice, and style scored automatically — so every piece of content starts consistent before it goes multilingual.
          </p>
        </div>

        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
          <div className="mb-4 flex h-10 w-10 items-center justify-center rounded-lg bg-preserved/10">
            <Layers className="h-5 w-5 text-preserved" />
          </div>
          <h3 className="text-lg font-semibold text-white">Works where you work</h3>
          <p className="mt-2 text-sm leading-relaxed text-neutral-400">
            MCP, CLI, or REST. Wire it into your AI tools, your CI pipeline, or your editor. Configure once, ship everywhere.
          </p>
        </div>
      </div>
    </section>
  )
}
