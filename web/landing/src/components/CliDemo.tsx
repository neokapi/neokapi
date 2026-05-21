import { useState } from 'react'
import { cn } from '@/lib/utils'

const TABS = [
  {
    label: 'AI Translate',
    cmd: 'kapi ai-translate -i src/en.json \\\n  --source-lang en \\\n  --target-lang fr \\\n  -o src/fr.json',
    description: 'Translate a file with an AI model. Compose it into a flow with terminology lookup and QA, or run it on its own.',
  },
  {
    label: 'Pseudo',
    cmd: 'kapi pseudo-translate src/messages.json \\\n  --target-lang qps \\\n  -o src/messages_qps.json',
    description: 'Generate pseudo-translations to test your UI for hardcoded strings, truncation, character encoding, and layout issues.',
  },
  {
    label: 'Word Count',
    cmd: 'kapi word-count "docs/**/*.md" --json',
    description: 'Count translatable words and segments across any file format. Get cost estimates before sending content for translation.',
  },
  {
    label: 'Terminology',
    cmd: 'kapi termbase import glossary.csv \\\n  --format csv \\\n  -s en \\\n  -t fr',
    description: 'Import terminology from CSV, TSV, or JSON. Enforce consistent terminology across translations with the term-enforce tool.',
  },
  {
    label: 'Formats',
    cmd: 'kapi formats\n\n# built-in readers and writers:\n# HTML, XML, XLIFF 1.2, XLIFF 2.0, JSON,\n# YAML, PO, Properties, Markdown, CSV,\n# SRT, VTT, TMX, Plaintext, ...\n# more via the Okapi bridge plugin',
    description: 'Explore the supported file formats. Neokapi detects formats by extension, MIME type, or content sniffing.',
  },
]

export function CliDemo() {
  const [active, setActive] = useState(0)

  return (
    <section id="cli" className="relative px-6 py-24">
      <div className="mx-auto max-w-6xl">
        <div className="mb-16 text-center">
          <h2 className="font-display text-3xl font-bold tracking-tight text-white sm:text-4xl">
            The{' '}
            <code className="rounded-lg bg-surface-800 px-2 py-1 font-mono text-brand-400">kapi</code>
            {' '}CLI
          </h2>
          <p className="mx-auto mt-4 max-w-2xl text-lg text-neutral-400">
            Process files directly. No project setup, no server, no configuration needed.
          </p>
        </div>

        <div className="grid gap-8 lg:grid-cols-[1fr_1.4fr]">
          {/* Tab list */}
          <div className="flex flex-col gap-2">
            {TABS.map((tab, i) => (
              <button
                key={tab.label}
                onClick={() => setActive(i)}
                className={cn(
                  'group rounded-xl px-5 py-4 text-left transition-all duration-200',
                  i === active
                    ? 'border border-brand-500/20 bg-brand-500/[0.06] shadow-lg shadow-brand-500/[0.03]'
                    : 'border border-transparent hover:border-surface-600 hover:bg-surface-800/50'
                )}
              >
                <div className="flex items-center gap-3">
                  <div
                    className={cn(
                      'h-2 w-2 rounded-full transition-colors',
                      i === active ? 'bg-brand-400' : 'bg-surface-600 group-hover:bg-surface-500'
                    )}
                  />
                  <span
                    className={cn(
                      'font-display text-sm font-semibold transition-colors',
                      i === active ? 'text-brand-300' : 'text-neutral-400 group-hover:text-neutral-300'
                    )}
                  >
                    {tab.label}
                  </span>
                </div>
                <p className={cn(
                  'mt-2 ml-5 text-sm leading-relaxed transition-colors',
                  i === active ? 'text-neutral-300' : 'text-neutral-500'
                )}>
                  {tab.description}
                </p>
              </button>
            ))}
          </div>

          {/* Code panel */}
          <div className="terminal-window overflow-hidden rounded-2xl glow-teal">
            <div className="flex items-center gap-2 border-b border-brand-500/8 px-5 py-3">
              <div className="h-2.5 w-2.5 rounded-full bg-accent-rose/50" />
              <div className="h-2.5 w-2.5 rounded-full bg-accent-amber/50" />
              <div className="h-2.5 w-2.5 rounded-full bg-brand-500/50" />
              <span className="ml-3 font-mono text-xs text-neutral-600">terminal</span>
            </div>
            <div className="p-6">
              <pre className="font-mono text-sm leading-relaxed">
                <span className="select-none text-brand-400">$ </span>
                <span className="text-neutral-200">{TABS[active].cmd}</span>
              </pre>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
