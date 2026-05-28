import { FileCode, GitBranch, Sparkles, Database, Shield, Package, Server } from 'lucide-react'

const FORMATS_SAMPLE = [
  'JSON', 'YAML', 'XLIFF', 'PO', 'Properties', 'HTML', 'Markdown',
  'SRT', 'VTT', 'CSV', 'XML', 'RESX', 'Android XML', 'iOS Strings',
]

export function OpenSource() {
  return (
    <section id="open-source" className="mx-auto max-w-6xl px-6 py-24">
      <div className="mx-auto max-w-3xl text-center">
        <h2 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">
          Built on an open toolchain you can use on its own
        </h2>
        <p className="mt-3 text-neutral-400">
          Bowrain builds on kapi, the Apache-2.0 open-source toolchain. kapi reads and writes localization, document,
          and data formats, chains tools into workflows, translates with AI, and keeps a translation memory — and it
          runs anywhere, with or without Bowrain. Bowrain hosts and governs that work for a team.
        </p>
      </div>

      {/* Capabilities */}
      <div className="mt-12 grid gap-6 md:grid-cols-2">
        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
          <div className="mb-4 flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-brand-500/10">
              <FileCode className="h-5 w-5 text-brand-400" />
            </div>
            <h3 className="text-lg font-semibold text-white">Format-aware reading and writing</h3>
          </div>
          <p className="text-sm leading-relaxed text-neutral-400">
            Readers and writers that distinguish text from variables and markup, across localization, document,
            data, subtitle, and office formats — from JSON and Markdown to DOCX, XLSX, IDML, and PDF.
          </p>
          <div className="mt-4 flex flex-wrap gap-1.5">
            {FORMATS_SAMPLE.map(f => (
              <span key={f} className="rounded-md bg-neutral-800/70 px-2 py-0.5 text-xs text-neutral-400">
                {f}
              </span>
            ))}
            <span className="rounded-md bg-brand-500/10 px-2 py-0.5 text-xs text-brand-400">
              and more
            </span>
          </div>
        </div>

        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
          <div className="mb-4 flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-brand-500/10">
              <GitBranch className="h-5 w-5 text-brand-400" />
            </div>
            <h3 className="text-lg font-semibold text-white">Chainable workflows</h3>
          </div>
          <p className="text-sm leading-relaxed text-neutral-400">
            Tools chain into a workflow. Segment, reuse past translations, look up glossary terms,
            translate with AI, and check quality — all in one run.
          </p>
          <div className="mt-4 overflow-hidden rounded-lg border border-neutral-800 bg-neutral-950 p-4 font-mono text-xs text-neutral-400">
            <div className="text-neutral-600"># .kapi/flows/translate.yaml</div>
            <div><span className="text-brand-400">steps</span>:</div>
            <div>  - <span className="text-brand-400">segmenter</span></div>
            <div>  - <span className="text-brand-400">tm-leverage</span>: <span className="text-neutral-300">min-score: 75</span></div>
            <div>  - <span className="text-brand-400">term-lookup</span></div>
            <div>  - <span className="text-brand-400">ai-translate</span>: <span className="text-neutral-300">provider: claude</span></div>
            <div>  - <span className="text-brand-400">term-enforce</span></div>
            <div>  - <span className="text-brand-400">qa-check</span></div>
          </div>
        </div>

        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
          <div className="mb-4 flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-brand-500/10">
              <Sparkles className="h-5 w-5 text-brand-400" />
            </div>
            <h3 className="text-lg font-semibold text-white">AI translation with your glossary</h3>
          </div>
          <p className="text-sm leading-relaxed text-neutral-400">
            AI translation runs in the same workflow as your glossary and past translations.
            Your preferred terms are sent to the model, so it uses your vocabulary rather than its own.
            Works with a range of AI providers, including Claude, OpenAI, Azure OpenAI, and Ollama.
          </p>
          <div className="mt-4 overflow-hidden rounded-lg border border-neutral-800 bg-neutral-950 p-4 font-mono text-xs">
            <div className="text-neutral-600"># Your glossary, sent to the AI</div>
            <div className="mt-1 text-neutral-400">
              <span className="text-brand-400">workspace</span> → <span className="text-neutral-300">Arbeitsbereich</span> <span className="text-neutral-600">(de)</span>
            </div>
            <div className="text-neutral-400">
              <span className="text-brand-400">dashboard</span> → <span className="text-neutral-300">Übersicht</span> <span className="text-neutral-600">(de)</span>
            </div>
            <div className="text-neutral-400">
              <span className="text-brand-400">deploy</span> → <span className="text-neutral-300">bereitstellen</span> <span className="text-neutral-600">(de)</span>
            </div>
            <div className="mt-2 text-neutral-600">→ AI uses these terms, not its own guesses</div>
          </div>
        </div>

        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
          <div className="mb-4 flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-suggestion/10">
              <Database className="h-5 w-5 text-suggestion" />
            </div>
            <h3 className="text-lg font-semibold text-white">Translation reuse</h3>
          </div>
          <p className="text-sm leading-relaxed text-neutral-400">
            Names, companies, and dates are recognized and set aside before matching,
            so a sentence you translated once gets reused even when those values change.
          </p>
          <div className="mt-4 space-y-2 text-xs">
            <div className="rounded-lg border border-neutral-800 bg-neutral-950 p-3">
              <div className="text-neutral-500">Already translated:</div>
              <div className="text-neutral-300">"<span className="text-brand-400">John</span> works at <span className="text-brand-400">Acme</span>"</div>
            </div>
            <div className="rounded-lg border border-suggestion/30 bg-suggestion/5 p-3">
              <div className="text-neutral-500">New source:</div>
              <div className="text-neutral-300">"<span className="text-suggestion">Alice</span> works at <span className="text-suggestion">Globex</span>"</div>
              <div className="mt-1 flex items-center gap-2">
                <span className="rounded-full bg-suggestion/20 px-2 py-0.5 text-suggestion font-mono">100% match</span>
                <span className="text-neutral-500">names swapped automatically</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Licensing & deployment */}
      <div className="mt-12 grid gap-6 md:grid-cols-3">
        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6 text-center">
          <Shield className="mx-auto mb-3 h-8 w-8 text-suggestion" />
          <h3 className="text-base font-semibold text-white">Open toolchain, yours to keep</h3>
          <p className="mt-2 text-sm text-neutral-400">
            kapi and the neokapi framework it builds on are Apache-2.0. Inspect, fork, contribute, and run
            <code className="rounded bg-neutral-800 px-1 text-xs text-neutral-300"> kapi</code> on its own — without Bowrain.
          </p>
        </div>

        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6 text-center">
          <Server className="mx-auto mb-3 h-8 w-8 text-brand-400" />
          <h3 className="text-base font-semibold text-white">Self-host the platform</h3>
          <p className="mt-2 text-sm text-neutral-400">
            The Bowrain platform is available under AGPL or a commercial license.
            Run it on your own infrastructure with full control of your data.
          </p>
        </div>

        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6 text-center">
          <Package className="mx-auto mb-3 h-8 w-8 text-neutral-300" />
          <h3 className="text-base font-semibold text-white">No lock-in at any layer</h3>
          <p className="mt-2 text-sm text-neutral-400">
            kapi is a single executable that needs no runtime to install. Your projects, terminology, and
            translation memory stay in open formats you can take elsewhere.
          </p>
        </div>
      </div>
    </section>
  )
}
