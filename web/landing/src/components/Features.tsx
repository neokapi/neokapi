import {
  FileText, Brain, Workflow, BookOpen, Puzzle,
  Zap, Shield, Globe,
} from 'lucide-react'

const FEATURES = [
  {
    icon: FileText,
    title: 'Wide File Format Support',
    description: 'HTML, XLIFF, JSON, YAML, PO, Markdown, SRT, VTT, and more built-in. Plus 40+ Okapi Framework filters via plugin for Office, EPUB, PDF, IDML.',
    accent: 'text-brand-400',
    bg: 'bg-brand-500/8',
    border: 'border-brand-500/15',
  },
  {
    icon: Brain,
    title: 'AI-Native Translation',
    description: 'LLM translation, QA, and terminology extraction are composable pipeline tools. Use Anthropic Claude, OpenAI, or run local models with Ollama.',
    accent: 'text-accent-amber',
    bg: 'bg-accent-amber/8',
    border: 'border-accent-amber/15',
  },
  {
    icon: Workflow,
    title: 'Concurrent Pipeline',
    description: 'Channel-based streaming architecture. Each tool runs in its own goroutine with automatic backpressure. Processes large files and batches without configuration.',
    accent: 'text-accent-cyan',
    bg: 'bg-accent-cyan/8',
    border: 'border-accent-cyan/15',
  },
{
    icon: BookOpen,
    title: 'TM + Terminology',
    description: 'Built-in translation memory with entity-aware fuzzy matching. Concept-oriented termbase with lifecycle statuses and domain classification.',
    accent: 'text-forest-400',
    bg: 'bg-forest-400/8',
    border: 'border-forest-400/15',
  },
  {
    icon: Puzzle,
    title: 'Plugin System',
    description: 'Extend with gRPC plugins in any language. Crash-isolated processes ensure stability. The Okapi bridge alone adds 40+ production-proven Java filters.',
    accent: 'text-accent-rose',
    bg: 'bg-accent-rose/8',
    border: 'border-accent-rose/15',
  },
  {
    icon: Zap,
    title: 'Zero Configuration',
    description: 'Process files instantly with kapi. No project directory, no server, no YAML to write. Automatic format detection. Just point and go.',
    accent: 'text-brand-400',
    bg: 'bg-brand-500/8',
    border: 'border-brand-500/15',
  },
  {
    icon: Shield,
    title: 'Quality Assurance',
    description: 'Rule-based QA checks plus AI-powered review. Catch terminology violations, formatting errors, and fluency issues before they ship.',
    accent: 'text-accent-amber',
    bg: 'bg-accent-amber/8',
    border: 'border-accent-amber/15',
  },
  {
    icon: Globe,
    title: 'Progressive Complexity',
    description: 'Start with a single CLI command. Grow into tools, flows and CI pipelines. The same composable tools work at every scale.',
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
            Everything you need to{' '}
            <span className="bg-gradient-to-r from-brand-400 to-brand-300 bg-clip-text text-transparent">
              localize at scale
            </span>
          </h2>
          <p className="mx-auto mt-4 max-w-2xl text-lg text-neutral-400">
            A composable toolkit where every piece works standalone and composes into powerful workflows.
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
