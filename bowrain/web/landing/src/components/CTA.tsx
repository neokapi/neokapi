import { Terminal, MessageSquare } from 'lucide-react'
import { GithubIcon } from './GithubIcon'

export function CTA() {
  return (
    <section id="get-started" className="mx-auto max-w-6xl px-6 py-24">
      <div className="mx-auto max-w-3xl text-center">
        <h2 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">
          Get started in one command
        </h2>
        <p className="mt-3 text-neutral-400">
          Install the CLI. Initialize a project. Translate your first file.
        </p>

        <div className="mt-8 overflow-hidden rounded-lg border border-neutral-800 bg-neutral-950 px-6 py-4 font-mono text-sm">
          <span className="text-suggestion">$</span>{' '}
          <span className="text-neutral-400">brew install bowrain</span>
        </div>
      </div>

      <div className="mt-12 grid gap-6 md:grid-cols-2">
        <a
          href="#platform"
          className="group rounded-xl border border-neutral-800 bg-neutral-900/30 p-8 transition hover:border-brand-500/50 hover:bg-brand-500/5"
        >
          <Terminal className="mb-4 h-8 w-8 text-brand-400" />
          <h3 className="text-xl font-semibold text-white group-hover:text-brand-400 transition">Explore the platform</h3>
          <p className="mt-2 text-sm text-neutral-400">
            Project management, server sync, live connectors, automated workflows,
            and a visual translation editor. Powered by 41+ formats and 80+ tools.
          </p>
        </a>

        <a
          href="#open-source"
          className="group rounded-xl border border-neutral-800 bg-neutral-900/30 p-8 transition hover:border-brand-500/50 hover:bg-brand-500/5"
        >
          <GithubIcon className="mb-4 h-8 w-8 text-brand-400" />
          <h3 className="text-xl font-semibold text-white group-hover:text-brand-400 transition">See the open-source foundation</h3>
          <p className="mt-2 text-sm text-neutral-400">
            Apache 2.0 framework. Self-host under AGPL or a commercial license.
            No lock-in at any layer.
          </p>
        </a>
      </div>

      <div className="mt-8 flex flex-wrap items-center justify-center gap-4">
        <a
          href="https://github.com/neokapi"
          target="_blank"
          rel="noopener"
          className="flex items-center gap-2 rounded-lg border border-neutral-700 px-4 py-2 text-sm text-neutral-300 transition hover:border-neutral-500 hover:text-white"
        >
          <GithubIcon className="h-4 w-4" />
          View on GitHub
        </a>
        <a
          href="mailto:hello@bowrain.com"
          className="flex items-center gap-2 rounded-lg border border-neutral-700 px-4 py-2 text-sm text-neutral-300 transition hover:border-neutral-500 hover:text-white"
        >
          <MessageSquare className="h-4 w-4" />
          Contact us
        </a>
      </div>
    </section>
  )
}
