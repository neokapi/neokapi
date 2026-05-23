import { useState, useEffect } from 'react'
import { Terminal, Sparkles, ChevronRight } from 'lucide-react'

const PSEUDO_MAP: Record<string, string> = {
  a: 'ä', e: 'ë', i: 'ï', o: 'ö', u: 'ü',
  A: 'Ä', E: 'Ë', I: 'Ï', O: 'Ö', U: 'Ü',
  c: 'ç', n: 'ñ', s: 'š', z: 'ž', y: 'ÿ',
  d: 'đ', D: 'Đ', r: 'ŕ', R: 'Ŕ', b: 'ƀ',
}

function pseudoChar(c: string) { return PSEUDO_MAP[c] || c }

const PSEUDO_LINES = [
  { original: 'Welcome to our platform', pseudo: '' },
  { original: 'Get started in seconds', pseudo: '' },
  { original: 'Manage your workspace', pseudo: '' },
]
PSEUDO_LINES.forEach(l => { l.pseudo = l.original.split('').map(pseudoChar).join('') })

const BRAND_INPUT = 'We leverage cutting-edge synergy to utilize our game-changer platform!!'
const BRAND_VIOLATIONS = [
  { word: 'leverage', suggestion: 'use', dim: 'Vocabulary' },
  { word: 'synergy', suggestion: 'collaboration', dim: 'Brand' },
  { word: 'utilize', suggestion: 'use', dim: 'Vocabulary' },
  { word: 'game-changer', suggestion: 'significant improvement', dim: 'Style' },
  { word: '!!', suggestion: '.', dim: 'Tone' },
]

export function Hero() {
  const [slide, setSlide] = useState(0)

  useEffect(() => {
    const timer = setInterval(() => {
      setSlide(s => (s + 1) % 2)
    }, 6000)
    return () => clearInterval(timer)
  }, [])

  return (
    <section className="relative flex min-h-[90vh] flex-col items-center justify-center px-6 pt-14">
      <div className="pointer-events-none absolute inset-0 overflow-hidden">
        <div className="absolute left-1/2 top-1/4 h-[600px] w-[600px] -translate-x-1/2 -translate-y-1/2 rounded-full bg-brand-500/5 blur-[120px]" />
      </div>

      <div className="relative z-10 mx-auto max-w-4xl text-center">
        <div className="animate-fade-in-up mb-6 inline-flex items-center gap-2 rounded-full border border-neutral-800 bg-neutral-900/50 px-4 py-1.5 text-sm text-neutral-400">
          <span className="h-1.5 w-1.5 rounded-full bg-suggestion" />
          Localization infrastructure for the pace of AI
        </div>

        <h1 className="animate-fade-in-up-delay-1 text-4xl font-bold leading-tight tracking-tight text-white sm:text-5xl md:text-6xl lg:text-7xl">
          Low-friction multilingual{' '}
          <span className="bg-gradient-to-r from-brand-400 to-brand-300 bg-clip-text text-transparent">
            plumbing for builders.
          </span>
        </h1>

        <p className="animate-fade-in-up-delay-2 mx-auto mt-6 max-w-2xl text-lg text-neutral-400 md:text-xl">
          An open-source engine that can read and write your files and content, translates with AI, and remembers past work.
          A platform that connects to your systems and keeps your team in sync.
        </p>

        {/* Primary CTAs */}
        <div className="animate-fade-in-up-delay-3 mt-10 flex flex-col items-center gap-4 sm:flex-row sm:justify-center">
          <a
            href="#pseudo-challenge"
            className="group flex w-full items-center justify-center gap-2 rounded-xl bg-brand-500 px-6 py-3 text-base font-medium text-white transition hover:bg-brand-600 sm:w-auto"
          >
            <Terminal className="h-5 w-5" />
            Try the Ṗšëüđö Challenge
          </a>
          <a
            href="#brand-challenge"
            className="group flex w-full items-center justify-center gap-2 rounded-xl border border-neutral-700 bg-neutral-900/50 px-6 py-3 text-base font-medium text-neutral-200 transition hover:border-neutral-500 hover:text-white sm:w-auto"
          >
            <Sparkles className="h-5 w-5" />
            Try the On-Brand Challenge
          </a>
        </div>

        {/* Secondary CTAs */}
        <div className="mt-6 flex items-center justify-center gap-6 text-sm text-neutral-500">
          <a href="#get-started" className="transition hover:text-neutral-300">
            <code className="rounded bg-neutral-800/50 px-2 py-1 text-xs">brew install bowrain</code>
          </a>
          <span className="text-neutral-700">|</span>
          <a href="https://github.com/neokapi" target="_blank" rel="noopener" className="transition hover:text-neutral-300">
            View on GitHub
          </a>
        </div>

        {/* Sliding demo — seamless auto-rotation */}
        <div className="animate-fade-in-up-delay-3 mt-16 overflow-hidden rounded-xl border border-neutral-800 bg-neutral-900/50 shadow-2xl shadow-brand-500/5">
          {/* Window chrome */}
          <div className="flex items-center gap-2 border-b border-neutral-800 px-4 py-3">
            <div className="h-2.5 w-2.5 rounded-full bg-red-500/60" />
            <div className="h-2.5 w-2.5 rounded-full bg-yellow-500/60" />
            <div className="h-2.5 w-2.5 rounded-full bg-green-500/60" />
            <span className="ml-2 text-xs text-neutral-600 font-mono">
              {slide === 0 ? 'kapi run pseudo' : 'kapi run brand-check'}
            </span>
          </div>

          {/* Crossfade content */}
          <div className="relative min-h-[260px]">
            <div
              className={`absolute inset-0 p-6 text-left transition-opacity duration-700 ease-in-out ${
                slide === 0 ? 'opacity-100' : 'opacity-0 pointer-events-none'
              }`}
            >
              <PseudoPreview active={slide === 0} />
            </div>
            <div
              className={`absolute inset-0 p-6 text-left transition-opacity duration-700 ease-in-out ${
                slide === 1 ? 'opacity-100' : 'opacity-0 pointer-events-none'
              }`}
            >
              <BrandPreview active={slide === 1} />
            </div>
          </div>

          {/* Subtle progress dots */}
          <div className="flex justify-center gap-2 pb-4">
            <div className={`h-1 rounded-full transition-all duration-700 ${slide === 0 ? 'w-5 bg-brand-400/60' : 'w-1 bg-neutral-700/60'}`} />
            <div className={`h-1 rounded-full transition-all duration-700 ${slide === 1 ? 'w-5 bg-brand-400/60' : 'w-1 bg-neutral-700/60'}`} />
          </div>
        </div>
      </div>
    </section>
  )
}

/* ---------- Pseudo mini-preview ---------- */

function PseudoPreview({ active }: { active: boolean }) {
  const [step, setStep] = useState(0)

  useEffect(() => {
    if (active) setStep(0)
  }, [active])

  useEffect(() => {
    if (active && step < PSEUDO_LINES.length) {
      const timer = setTimeout(() => setStep(s => s + 1), 600)
      return () => clearTimeout(timer)
    }
  }, [active, step])

  return (
    <div className="font-mono text-sm">
      <div className="flex items-center gap-2 text-neutral-400">
        <span className="text-suggestion">$</span>
        <span>kapi run pseudo</span>
      </div>

      <div className="mt-4 space-y-3">
        {PSEUDO_LINES.slice(0, step).map((line, i) => (
          <div key={i} className="space-y-1 animate-fade-in-up">
            <div className="text-neutral-600">
              <span className="text-neutral-500">#</span> {line.original}
            </div>
            <div className="flex items-start gap-2">
              <ChevronRight className="mt-0.5 h-3 w-3 text-suggestion" />
              <span className="text-neutral-200">{line.pseudo}</span>
            </div>
          </div>
        ))}
      </div>

      {step >= PSEUDO_LINES.length && (
        <div className="mt-4 flex items-center gap-4 border-t border-neutral-800 pt-3 animate-fade-in-up">
          <span className="text-xs text-neutral-500">Completed in <span className="text-suggestion">47ms</span></span>
          <span className="text-xs text-preserved">Ready for i18n testing</span>
          <a href="#pseudo-challenge" className="ml-auto flex items-center gap-1 text-xs text-brand-400 hover:text-brand-300 transition">
            Try it yourself <ChevronRight className="h-3 w-3" />
          </a>
        </div>
      )}
    </div>
  )
}

/* ---------- Brand mini-preview ---------- */

function BrandPreview({ active }: { active: boolean }) {
  const [revealed, setRevealed] = useState(0)

  useEffect(() => {
    if (active) setRevealed(0)
  }, [active])

  useEffect(() => {
    if (active && revealed < BRAND_VIOLATIONS.length) {
      const timer = setTimeout(() => setRevealed(r => r + 1), 500)
      return () => clearTimeout(timer)
    }
  }, [active, revealed])

  const score = Math.max(0, 100 - revealed * 12)
  const scoreColor = score >= 80 ? 'text-suggestion' : score >= 50 ? 'text-violation' : 'text-forbidden'
  const barColor = score >= 80 ? 'bg-suggestion' : score >= 50 ? 'bg-violation' : 'bg-forbidden'

  const DIM_COLORS: Record<string, string> = {
    Vocabulary: 'text-brand-400 bg-brand-500/10',
    Brand: 'text-forbidden bg-forbidden/10',
    Style: 'text-purple-400 bg-purple-500/10',
    Tone: 'text-violation bg-violation/10',
  }

  return (
    <div className="text-sm">
      <div className="rounded-lg bg-neutral-800/50 px-4 py-3 text-neutral-300">
        <HighlightedText text={BRAND_INPUT} violations={BRAND_VIOLATIONS.slice(0, revealed)} />
      </div>

      <div className="mt-4 flex items-start gap-6">
        <div className="text-center shrink-0">
          <div className={`text-3xl font-bold transition-all duration-500 ${scoreColor}`}>{score}</div>
          <div className="text-xs text-neutral-500">/ 100</div>
          <div className="mt-2 h-1.5 w-16 overflow-hidden rounded-full bg-neutral-800">
            <div className={`h-full rounded-full transition-all duration-500 ${barColor}`} style={{ width: `${score}%` }} />
          </div>
        </div>

        <div className="flex flex-wrap gap-2">
          {BRAND_VIOLATIONS.slice(0, revealed).map((v, i) => (
            <span key={i} className={`inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs animate-fade-in-up ${DIM_COLORS[v.dim] || 'text-neutral-400 bg-neutral-800'}`}>
              <span className="font-medium">"{v.word}"</span>
              <span className="opacity-60">→ {v.suggestion}</span>
            </span>
          ))}
        </div>
      </div>

      {revealed >= BRAND_VIOLATIONS.length && (
        <div className="mt-4 flex items-center border-t border-neutral-800 pt-3 animate-fade-in-up">
          <span className="text-xs text-neutral-500">5 violations found — fix them to hit 100</span>
          <a href="#brand-challenge" className="ml-auto flex items-center gap-1 text-xs text-brand-400 hover:text-brand-300 transition">
            Try it yourself <ChevronRight className="h-3 w-3" />
          </a>
        </div>
      )}
    </div>
  )
}

function HighlightedText({ text, violations }: { text: string; violations: typeof BRAND_VIOLATIONS }) {
  if (violations.length === 0) return <>{text}</>

  const parts: Array<{ text: string; isViolation: boolean }> = []
  let remaining = text

  const sorted = [...violations].sort((a, b) => text.indexOf(a.word) - text.indexOf(b.word))

  for (const v of sorted) {
    const idx = remaining.indexOf(v.word)
    if (idx === -1) continue
    if (idx > 0) parts.push({ text: remaining.slice(0, idx), isViolation: false })
    parts.push({ text: v.word, isViolation: true })
    remaining = remaining.slice(idx + v.word.length)
  }
  if (remaining) parts.push({ text: remaining, isViolation: false })

  return (
    <>
      {parts.map((p, i) =>
        p.isViolation
          ? <span key={i} className="rounded bg-violation/20 px-0.5 text-violation underline decoration-wavy decoration-violation/40">{p.text}</span>
          : <span key={i}>{p.text}</span>
      )}
    </>
  )
}
