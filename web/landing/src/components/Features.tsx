import {
  FileText, Brain, Workflow, BookOpen, Puzzle,
  Zap, Shield, Globe,
} from 'lucide-react'

const FEATURES = [
  {
    icon: FileText,
    title: 'Wide file-format support',
    description: 'HTML, XLIFF, JSON, YAML, PO, Markdown, SRT, VTT and more built in, with the Okapi bridge plugin adding the Java filters for Office, EPUB, PDF, and IDML.',
    accent: 'text-brand-400',
    bg: 'bg-brand-500/8',
    border: 'border-brand-500/15',
  },
  {
    icon: Brain,
    title: 'AI-native translation',
    description: 'LLM translation, QA, and terminology extraction are pipeline tools. Use Anthropic Claude, OpenAI, or run local models with Ollama.',
    accent: 'text-accent-amber',
    bg: 'bg-accent-amber/8',
    border: 'border-accent-amber/15',
  },
  {
    icon: Workflow,
    title: 'Concurrent pipeline',
    description: 'A channel-based streaming architecture: each tool runs in its own goroutine with backpressure, processing large files and batches without tuning.',
    accent: 'text-accent-cyan',
    bg: 'bg-accent-cyan/8',
    border: 'border-accent-cyan/15',
  },
{
    icon: BookOpen,
    title: 'TM + terminology',
    description: 'Built-in translation memory with entity-aware fuzzy matching, and a concept-oriented termbase with lifecycle statuses and domain classification.',
    accent: 'text-forest-400',
    bg: 'bg-forest-400/8',
    border: 'border-forest-400/15',
  },
  {
    icon: Puzzle,
    title: 'Plugin system',
    description: 'Extend with gRPC plugins in any language, each running as a crash-isolated process. The Okapi bridge adds the Java filters from the Okapi Framework.',
    accent: 'text-accent-rose',
    bg: 'bg-accent-rose/8',
    border: 'border-accent-rose/15',
  },
  {
    icon: Zap,
    title: 'No configuration required',
    description: 'kapi processes files directly — no project directory, server, or YAML to write. Formats are detected from extension, MIME type, or content.',
    accent: 'text-brand-400',
    bg: 'bg-brand-500/8',
    border: 'border-brand-500/15',
  },
  {
    icon: Shield,
    title: 'Quality assurance',
    description: 'Rule-based QA checks alongside AI review, flagging terminology violations, formatting errors, and fluency issues during the pipeline.',
    accent: 'text-accent-amber',
    bg: 'bg-accent-amber/8',
    border: 'border-accent-amber/15',
  },
  {
    icon: Globe,
    title: 'Progressive complexity',
    description: 'Start with a single CLI command and grow into tools, flows, and CI pipelines — the same building blocks work at every scale.',
    accent: 'text-accent-cyan',
    bg: 'bg-accent-cyan/8',
    border: 'border-accent-cyan/15',
  },
]

export function Features() {
  return (
    <section id="features" className="relative px-6 py-24">
      <div className="mx-auto max-w-6xl">
        <div className="mb-16 text-center">
          <h2 className="font-display text-3xl font-bold tracking-tight text-white sm:text-4xl">
            One toolkit, from a single command to a{' '}
            <span className="bg-gradient-to-r from-brand-400 to-brand-300 bg-clip-text text-transparent">
              full pipeline
            </span>
          </h2>
          <p className="mx-auto mt-4 max-w-2xl text-lg text-neutral-400">
            Each tool runs on its own and chains into a flow — the same building blocks at every scale.
          </p>
        </div>

        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
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
