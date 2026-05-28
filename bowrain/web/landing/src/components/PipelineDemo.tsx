import { useState, useEffect, useCallback } from 'react'
import { Play, ChevronRight } from 'lucide-react'

interface FlowStep {
  label: string
  detail: string
  status: 'pending' | 'running' | 'done'
  extra?: string
}

const INITIAL_STEPS: FlowStep[] = [
  { label: 'Reader', detail: 'src/locales/en.json → 47 translatable strings', status: 'pending' },
  { label: 'Segmenter', detail: '47 strings → 312 sentences', status: 'pending' },
  { label: 'Reuse', detail: '12 reused · 8 adapted · 27 new', status: 'pending', extra: '→ 20 strings filled from past translations' },
  { label: 'Terminology', detail: '15 glossary terms matched', status: 'pending', extra: '→ glossary sent to guide the AI' },
  { label: 'AI Translate', detail: '27 new strings via Claude', status: 'pending', extra: '→ uses your glossary, not its own guesses' },
  { label: 'Term Check', detail: '27 strings checked · 1 term corrected', status: 'pending' },
  { label: 'QA Check', detail: '47 strings passed · 0 errors · 2 warnings', status: 'pending' },
  { label: 'Writer', detail: 'src/locales/de.json written', status: 'pending' },
]

type FlowVariant = 'translate' | 'pseudo' | 'qa'

const FLOW_COMMANDS: Record<FlowVariant, { cmd: string; steps: FlowStep[] }> = {
  translate: {
    cmd: 'kapi run ai-translate-qa --target-lang de',
    steps: INITIAL_STEPS,
  },
  pseudo: {
    cmd: 'kapi run pseudo',
    steps: [
      { label: 'Reader', detail: 'src/**/*.json → 6 files, 183 blocks', status: 'pending' },
      { label: 'Segmenter', detail: '183 strings → 891 sentences', status: 'pending' },
      { label: 'Pseudo', detail: 'accents added + text expanded 30%', status: 'pending', extra: '→ "Settings" → "Šëťťïñğš [--- ----]"' },
      { label: 'QA Check', detail: '183 strings · 4 might not fit the UI', status: 'pending' },
      { label: 'Writer', detail: '6 files written to src/locales/qps/', status: 'pending' },
    ],
  },
  qa: {
    cmd: 'kapi run qa-check --target-lang de',
    steps: [
      { label: 'Reader', detail: 'de.json + en.json → 47 string pairs', status: 'pending' },
      { label: 'Terminology', detail: '15 terms verified · 2 inconsistent', status: 'pending', extra: '→ "Arbeitsbereich" vs "Workspace" in string 23' },
      { label: 'Variables', detail: '47 strings · all variables preserved', status: 'pending' },
      { label: 'Length', detail: '47 strings · 3 translations too long', status: 'pending' },
      { label: 'Formatting', detail: '47 strings · 0 issues', status: 'pending' },
      { label: 'Report', detail: '2 errors · 3 warnings · 42 passed', status: 'pending' },
    ],
  },
}

export function PipelineDemo() {
  const [variant, setVariant] = useState<FlowVariant>('translate')
  const [steps, setSteps] = useState<FlowStep[]>([])
  const [currentStep, setCurrentStep] = useState(-1)
  const [running, setRunning] = useState(false)
  const [elapsed, setElapsed] = useState<number | null>(null)

  const run = useCallback(() => {
    const flow = FLOW_COMMANDS[variant]
    setSteps(flow.steps.map(s => ({ ...s, status: 'pending' })))
    setCurrentStep(0)
    setRunning(true)
    setElapsed(null)
  }, [variant])

  const reset = useCallback(() => {
    setSteps([])
    setCurrentStep(-1)
    setRunning(false)
    setElapsed(null)
  }, [])

  useEffect(() => {
    if (!running || currentStep < 0) return
    const flow = FLOW_COMMANDS[variant]

    if (currentStep >= flow.steps.length) {
      setRunning(false)
      setElapsed(Math.round(800 + Math.random() * 3200))
      return
    }

    const timer = setTimeout(() => {
      setSteps(prev => prev.map((s, i) => ({
        ...s,
        status: i < currentStep ? 'done' : i === currentStep ? 'done' : 'pending',
      })))
      setCurrentStep(c => c + 1)
    }, 500 + Math.random() * 300)

    return () => clearTimeout(timer)
  }, [running, currentStep, variant])

  // Mark current step as running
  useEffect(() => {
    if (running && currentStep >= 0) {
      setSteps(prev => prev.map((s, i) => ({
        ...s,
        status: i < currentStep ? 'done' : i === currentStep ? 'running' : 'pending',
      })))
    }
  }, [currentStep, running])

  const flow = FLOW_COMMANDS[variant]

  return (
    <section id="pipeline-demo" className="mx-auto max-w-6xl px-6 py-24">
      <div className="mx-auto max-w-3xl text-center">
        <h2 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">
          See a flow run
        </h2>
        <p className="mt-3 text-neutral-400">
          Pick a flow and run it with kapi, the open toolchain Bowrain builds on. Each tool does its job in turn.
        </p>
      </div>

      {/* Flow selector */}
      <div className="mt-10 flex flex-wrap justify-center gap-2">
        {([
          { id: 'translate' as FlowVariant, label: 'AI Translate + QA' },
          { id: 'pseudo' as FlowVariant, label: 'Pseudo-localize' },
          { id: 'qa' as FlowVariant, label: 'QA Check' },
        ]).map(f => (
          <button
            key={f.id}
            onClick={() => { setVariant(f.id); reset() }}
            className={`rounded-lg border px-4 py-2 text-sm transition ${
              variant === f.id
                ? 'border-brand-500 bg-brand-500/10 text-brand-400'
                : 'border-neutral-800 text-neutral-500 hover:border-neutral-600 hover:text-neutral-300'
            }`}
          >
            {f.label}
          </button>
        ))}
      </div>

      {/* Terminal */}
      <div className="mt-8 overflow-hidden rounded-xl border border-neutral-800 bg-neutral-950">
        <div className="flex items-center gap-2 border-b border-neutral-800 px-4 py-2.5">
          <div className="h-2.5 w-2.5 rounded-full bg-red-500/60" />
          <div className="h-2.5 w-2.5 rounded-full bg-yellow-500/60" />
          <div className="h-2.5 w-2.5 rounded-full bg-green-500/60" />
          <span className="ml-2 text-xs text-neutral-600 font-mono">terminal</span>
        </div>

        <div className="p-6 font-mono text-sm">
          <div className="flex items-center gap-2 text-neutral-400">
            <span className="text-suggestion">$</span>
            <span>{flow.cmd}</span>
            {steps.length === 0 && <span className="cursor-blink text-brand-400">▋</span>}
          </div>

          {steps.length === 0 && !running && (
            <button
              onClick={run}
              className="mt-4 flex items-center gap-2 rounded-lg bg-brand-500 px-4 py-2 text-sm font-medium text-white transition hover:bg-brand-600"
            >
              <Play className="h-4 w-4" />
              Run
            </button>
          )}

          {steps.length > 0 && (
            <div className="mt-4 space-y-1.5">
              {steps.map((s, i) => (
                <div key={i}>
                  <div className={`flex items-center gap-3 ${
                    s.status === 'done' ? '' : s.status === 'running' ? 'text-brand-400' : 'text-neutral-700'
                  }`}>
                    <span className="w-4 text-center text-xs">
                      {s.status === 'done' && <span className="text-suggestion">✓</span>}
                      {s.status === 'running' && (
                        <span className="inline-block h-3 w-3 animate-spin rounded-full border border-brand-400/30 border-t-brand-400" />
                      )}
                      {s.status === 'pending' && <span className="text-neutral-700">·</span>}
                    </span>
                    <span className={`w-28 shrink-0 ${s.status === 'done' ? 'text-neutral-500' : ''}`}>{s.label}</span>
                    <span className={s.status === 'done' ? 'text-neutral-400' : s.status === 'running' ? 'text-neutral-500' : 'text-neutral-700'}>
                      {s.status !== 'pending' ? s.detail : ''}
                    </span>
                  </div>
                  {s.status === 'done' && s.extra && (
                    <div className="ml-7 flex items-center gap-1 text-xs text-neutral-600">
                      <ChevronRight className="h-3 w-3" />
                      {s.extra}
                    </div>
                  )}
                </div>
              ))}

              {elapsed !== null && (
                <div className="mt-3 flex flex-wrap items-center gap-4 border-t border-neutral-800 pt-3 animate-fade-in-up">
                  <span className="text-xs text-suggestion">Done in {(elapsed / 1000).toFixed(1)}s</span>
                  <span className="text-xs text-neutral-500">All steps ran in parallel</span>
                  <button
                    onClick={reset}
                    className="ml-auto text-xs text-brand-400 transition hover:text-brand-300"
                  >
                    Reset
                  </button>
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      <p className="mt-6 text-center text-sm text-neutral-500">
        Steps run in parallel — each one starts as soon as it has work, without waiting for others to finish.
      </p>
    </section>
  )
}
