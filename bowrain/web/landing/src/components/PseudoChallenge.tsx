import { useState, useEffect, useCallback } from 'react'
import { Play, ChevronRight, Shield, BarChart3, Globe } from 'lucide-react'

const PSEUDO_MAP: Record<string, string> = {
  a: 'ä', e: 'ë', i: 'ï', o: 'ö', u: 'ü',
  A: 'Ä', E: 'Ë', I: 'Ï', O: 'Ö', U: 'Ü',
  c: 'ç', n: 'ñ', s: 'š', z: 'ž', y: 'ÿ',
  C: 'Ç', N: 'Ñ', S: 'Š', Z: 'Ž', Y: 'Ÿ',
  w: 'ẃ', W: 'Ẃ', l: 'ľ', L: 'Ľ', t: 'ť', T: 'Ť',
  d: 'đ', D: 'Đ', r: 'ŕ', R: 'Ŕ', b: 'ƀ',
}

const PROTECTED_TERMS = ['Bowrain', 'Acme', 'AI assistant']

function pseudoLocalize(text: string, protectTerms: boolean): { result: string; preserved: string[] } {
  const preserved: string[] = []
  let result = text

  if (protectTerms) {
    const placeholders: { term: string; placeholder: string }[] = []
    PROTECTED_TERMS.forEach((term, i) => {
      const placeholder = `\x00${i}\x00`
      if (result.includes(term)) {
        preserved.push(term)
        result = result.replaceAll(term, placeholder)
        placeholders.push({ term, placeholder })
      }
    })

    result = result.split('').map(c => PSEUDO_MAP[c] || c).join('')

    placeholders.forEach(({ term, placeholder }) => {
      result = result.replaceAll(placeholder, term)
    })
  } else {
    result = text.split('').map(c => PSEUDO_MAP[c] || c).join('')
  }

  return { result, preserved }
}

const SAMPLE_STRINGS = [
  'Welcome to Bowrain — the best AI assistant layer',
  'Get started in seconds with our platform',
  'Manage your Acme workspace settings',
  'Contact support for help with your account',
]

const GERMAN_TRANSLATIONS: Record<string, string> = {
  'Welcome to Bowrain — the best AI assistant layer': 'Willkommen bei Bowrain — die beste KI-Assistenten-Plattform',
  'Get started in seconds with our platform': 'Starten Sie in Sekunden mit unserer Plattform',
  'Manage your Acme workspace settings': 'Verwalten Sie Ihre Acme-Arbeitsbereich-Einstellungen',
  'Contact support for help with your account': 'Kontaktieren Sie den Support für Hilfe mit Ihrem Konto',
}

type Level = 1 | 2 | 3 | 4

export function PseudoChallenge() {
  const [level, setLevel] = useState<Level>(1)
  const [running, setRunning] = useState(false)
  const [results, setResults] = useState<Array<{ original: string; pseudo: string; preserved: string[] }>>([])
  const [elapsed, setElapsed] = useState<number | null>(null)
  const [typingIndex, setTypingIndex] = useState(0)

  const runPseudo = useCallback(() => {
    setRunning(true)
    setResults([])
    setTypingIndex(0)
    const start = performance.now()

    const newResults = SAMPLE_STRINGS.map(str => {
      if (level === 4) {
        return { original: str, pseudo: GERMAN_TRANSLATIONS[str] || str, preserved: [] }
      }
      const { result, preserved } = pseudoLocalize(str, level >= 2)
      return { original: str, pseudo: result, preserved }
    })

    setTimeout(() => {
      setElapsed(Math.round(performance.now() - start + 200))
      setResults(newResults)
      setRunning(false)
    }, 400)
  }, [level])

  useEffect(() => {
    if (results.length > 0 && typingIndex < results.length) {
      const timer = setTimeout(() => setTypingIndex(i => i + 1), 150)
      return () => clearTimeout(timer)
    }
  }, [results, typingIndex])

  const totalPreserved = results.reduce((acc, r) => acc + r.preserved.length, 0)

  return (
    <section id="pseudo-challenge" className="mx-auto max-w-6xl px-6 py-24">
      <div className="mx-auto max-w-3xl text-center">
        <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-neutral-800 px-3 py-1 text-xs text-neutral-400 font-mono">
          RUNS LOCALLY WITH KAPI
        </div>
        <h2 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">
          The <span className="text-brand-400">Ṗšëüđö</span> Challenge
        </h2>
        <p className="mt-3 text-neutral-400">
          This is kapi, the open toolchain Bowrain builds on, running on your own machine. Go from zero to a
          multilingual-ready pipeline and see the result in seconds. Bowrain is what makes the glossary and voice rules
          below shared across a team.
        </p>
      </div>

      {/* Level tabs */}
      <div className="mt-10 flex flex-wrap justify-center gap-2">
        {([
          { l: 1 as Level, label: 'Go pseudo', icon: Play, desc: 'basic pseudo-localization' },
          { l: 2 as Level, label: 'Protect terms', icon: Shield, desc: 'glossary-aware' },
          { l: 3 as Level, label: 'Expand & score', icon: BarChart3, desc: 'i18n readiness' },
          { l: 4 as Level, label: 'Go real', icon: Globe, desc: 'real translation' },
        ] as const).map(({ l, label, icon: Icon, desc }) => (
          <button
            key={l}
            onClick={() => { setLevel(l); setResults([]); setElapsed(null) }}
            className={`flex items-center gap-2 rounded-lg border px-4 py-2 text-sm transition ${
              level === l
                ? 'border-brand-500 bg-brand-500/10 text-brand-400'
                : 'border-neutral-800 text-neutral-500 hover:border-neutral-600 hover:text-neutral-300'
            }`}
          >
            <Icon className="h-4 w-4" />
            <span className="font-medium">{label}</span>
            <span className="hidden text-xs opacity-60 sm:inline">— {desc}</span>
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
            <span>
              {level === 1 && 'kapi run pseudo'}
              {level === 2 && 'kapi run pseudo --protect-terms'}
              {level === 3 && 'kapi run pseudo --protect-terms --expand'}
              {level === 4 && 'kapi run ai-translate --target-lang de'}
            </span>
            {!running && results.length === 0 && <span className="cursor-blink text-brand-400">▋</span>}
          </div>

          {results.length === 0 && !running && (
            <button
              onClick={runPseudo}
              className="mt-4 flex items-center gap-2 rounded-lg bg-brand-500 px-4 py-2 text-sm font-medium text-white transition hover:bg-brand-600"
            >
              <Play className="h-4 w-4" />
              Run
            </button>
          )}

          {running && (
            <div className="mt-4 flex items-center gap-2 text-neutral-500">
              <div className="h-4 w-4 animate-spin rounded-full border-2 border-neutral-600 border-t-brand-400" />
              Processing...
            </div>
          )}

          {results.length > 0 && (
            <div className="mt-4 space-y-3">
              {results.slice(0, typingIndex).map((r, i) => (
                <div key={i} className="space-y-1">
                  <div className="text-neutral-600">
                    <span className="text-neutral-500">#</span> {r.original}
                  </div>
                  <div className="flex items-start gap-2">
                    <ChevronRight className="mt-0.5 h-3 w-3 text-suggestion" />
                    <span className="text-neutral-200">
                      {level >= 2
                        ? renderPreserved(r.pseudo, r.preserved)
                        : r.pseudo
                      }
                    </span>
                  </div>
                  {level >= 2 && r.preserved.length > 0 && (
                    <div className="ml-5 flex gap-2">
                      {r.preserved.map((term, j) => (
                        <span key={j} className="text-xs text-preserved">
                          ✓ "{term}" preserved
                        </span>
                      ))}
                    </div>
                  )}
                </div>
              ))}

              {typingIndex >= results.length && (
                <div className="mt-4 flex flex-wrap items-center gap-4 border-t border-neutral-800 pt-4">
                  <span className="text-xs text-neutral-500">
                    Completed in <span className="text-suggestion">{elapsed}ms</span>
                  </span>
                  {level >= 2 && (
                    <span className="text-xs text-preserved">
                      {totalPreserved} brand terms protected
                    </span>
                  )}
                  {level >= 3 && (
                    <span className="text-xs text-violation">
                      ~30% expansion detected — check for UI truncation
                    </span>
                  )}
                  {level < 4 && (
                    <button
                      onClick={() => { setLevel((level + 1) as Level); setResults([]); setElapsed(null) }}
                      className="ml-auto flex items-center gap-1 text-xs text-brand-400 transition hover:text-brand-300"
                    >
                      Next level <ChevronRight className="h-3 w-3" />
                    </button>
                  )}
                  {level === 4 && (
                    <span className="ml-auto text-xs text-suggestion">
                      Pipeline complete — from pseudo to production.
                    </span>
                  )}
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      <div className="mt-6 text-center text-sm text-neutral-500">
        {level === 1 && 'Level 1: Basic pseudo-localization. Proves your pipeline handles non-ASCII.'}
        {level === 2 && 'Level 2: kapi reads brand terms from your glossary and protects them. On Bowrain, that glossary is shared across the team.'}
        {level === 3 && 'Level 3: Expansion testing reveals UI truncation and layout breaks.'}
        {level === 4 && 'Level 4: Same pipeline, real language. Glossary and voice rules carry through.'}
      </div>
    </section>
  )
}

function renderPreserved(text: string, preserved: string[]) {
  if (preserved.length === 0) return text

  const parts: Array<{ text: string; isPreserved: boolean }> = []
  let remaining = text

  while (remaining.length > 0) {
    let earliestIndex = remaining.length
    let earliestTerm = ''

    for (const term of preserved) {
      const idx = remaining.indexOf(term)
      if (idx !== -1 && idx < earliestIndex) {
        earliestIndex = idx
        earliestTerm = term
      }
    }

    if (earliestTerm) {
      if (earliestIndex > 0) {
        parts.push({ text: remaining.slice(0, earliestIndex), isPreserved: false })
      }
      parts.push({ text: earliestTerm, isPreserved: true })
      remaining = remaining.slice(earliestIndex + earliestTerm.length)
    } else {
      parts.push({ text: remaining, isPreserved: false })
      remaining = ''
    }
  }

  return (
    <>
      {parts.map((part, i) =>
        part.isPreserved ? (
          <span key={i} className="rounded bg-preserved/10 px-0.5 text-preserved font-semibold">
            {part.text}
          </span>
        ) : (
          <span key={i}>{part.text}</span>
        )
      )}
    </>
  )
}
