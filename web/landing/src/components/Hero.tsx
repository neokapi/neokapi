import { useState, useEffect, useCallback } from 'react'
import { ChevronRight, WifiOff, FileText, Languages } from 'lucide-react'

const COMMANDS = [
  {
    cmd: 'kapi extract quarterly-report.docx -o strings.json',
    lines: [
      { text: 'Reading quarterly-report.docx (DOCX detected)', color: 'text-neutral-500' },
      { text: 'Extracted 184 translatable blocks', color: 'text-brand-400' },
      { text: 'Structure, styles, fields and placeholders preserved', color: 'text-neutral-400' },
      { text: 'Wrote strings.json', color: 'text-brand-300' },
      { text: 'Round-trip safe — `kapi merge` writes the .docx back, faithfully', color: 'text-accent-amber' },
    ],
  },
  {
    cmd: 'kapi ai-translate -i strings.json -o strings.de.json --target-lang de',
    lines: [
      { text: 'Reading strings.json (JSON detected)', color: 'text-neutral-500' },
      { text: '  ai-translate    provider: anthropic    184 blocks', color: 'text-brand-400' },
      { text: 'Wrote strings.de.json', color: 'text-brand-300' },
      { text: 'Placeholders, markup and inline tags kept intact', color: 'text-accent-amber' },
    ],
  },
  {
    cmd: 'kapi verify --target-lang de',
    lines: [
      { text: 'brand         PASS  (0 findings)', color: 'text-brand-400' },
      { text: 'terminology   PASS  (0 findings)', color: 'text-brand-400' },
      { text: 'qa            FAIL  (1 finding)', color: 'text-accent-rose' },
      { text: '  ERROR de: placeholder {count} dropped in target', color: 'text-accent-rose' },
      { text: 'FAIL — gate blocked; fix and re-run (exit 1)', color: 'text-brand-300' },
    ],
  },
]

const AXES = [
  { icon: FileText, label: 'Any format, faithfully' },
  { icon: Languages, label: 'Translate · check · transform' },
  { icon: WifiOff, label: 'Offline by default' },
]

export function Hero() {
  const [cmdIndex, setCmdIndex] = useState(0)
  const [lineIndex, setLineIndex] = useState(0)
  const [typing, setTyping] = useState(true)
  const [charIndex, setCharIndex] = useState(0)

  const current = COMMANDS[cmdIndex]

  const resetAndNext = useCallback(() => {
    setCmdIndex(i => (i + 1) % COMMANDS.length)
    setLineIndex(0)
    setTyping(true)
    setCharIndex(0)
  }, [])

  // Type out command character by character
  useEffect(() => {
    if (!typing) return
    if (charIndex < current.cmd.length) {
      const timer = setTimeout(() => setCharIndex(c => c + 1), 18)
      return () => clearTimeout(timer)
    }
    // Done typing, start showing output
    const timer = setTimeout(() => {
      setTyping(false)
      setLineIndex(0)
    }, 400)
    return () => clearTimeout(timer)
  }, [typing, charIndex, current.cmd.length])

  // Reveal output lines one by one
  useEffect(() => {
    if (typing) return
    if (lineIndex < current.lines.length) {
      const timer = setTimeout(() => setLineIndex(l => l + 1), 280)
      return () => clearTimeout(timer)
    }
    // All lines shown, wait then cycle
    const timer = setTimeout(resetAndNext, 8000)
    return () => clearTimeout(timer)
  }, [typing, lineIndex, current.lines.length, resetAndNext])

  return (
    <section className="relative flex min-h-[92vh] flex-col items-center justify-center px-6 pt-24">
      {/* Background effects */}
      <div className="pointer-events-none absolute inset-0 overflow-hidden">
        <div className="absolute left-1/2 top-[20%] h-[500px] w-[700px] -translate-x-1/2 -translate-y-1/2 rounded-full bg-brand-500/[0.04] blur-[150px]" />
        <div className="absolute right-[15%] top-[60%] h-[300px] w-[300px] rounded-full bg-forest-500/[0.03] blur-[100px]" />
      </div>

      <div className="dot-grid pointer-events-none absolute inset-0 opacity-40" />

      <div className="relative z-10 mx-auto max-w-5xl text-center">
        {/* Badge */}
        <div className="animate-fade-in-up mb-8 inline-flex items-center gap-2.5 rounded-full border border-brand-500/15 bg-brand-500/[0.06] px-4 py-1.5 text-sm">
          <span className="h-1.5 w-1.5 rounded-full bg-brand-400 animate-pulse-glow" />
          <span className="text-brand-300">Open-source engine</span>
          <span className="text-neutral-500">|</span>
          <span className="text-neutral-400">Offline-first · no lock-in</span>
        </div>

        {/* Headline with mascot */}
        <div className="animate-fade-in-up-delay-1 flex flex-col items-center gap-4">
          <img
            src={`${import.meta.env.BASE_URL}hero-logo.png`}
            alt="Neokapi mascot"
            className="h-28 w-28 animate-float drop-shadow-[0_0_30px_rgba(37,194,160,0.2)]"
          />
          <h1 className="font-display text-4xl font-extrabold leading-[1.1] tracking-tight text-white sm:text-5xl md:text-6xl lg:text-[4.2rem]">
            Read, change, and ship content in{' '}
            <span className="bg-gradient-to-r from-brand-400 via-brand-300 to-forest-400 bg-clip-text text-transparent text-glow">
              any format
            </span>
            .
          </h1>
        </div>

        <p className="animate-fade-in-up-delay-2 mx-auto mt-6 max-w-2xl text-lg leading-relaxed text-neutral-400 md:text-xl">
          An open engine that extracts what matters from any file, lets you
          translate, check, and transform it, and writes it back faithfully —
          structure, markup, and placeholders intact. AI translation, translation
          memory, terminology and brand checks built in.{' '}
          <span className="text-neutral-300">Offline by default — your content stays yours.</span>
        </p>

        {/* Three axes */}
        <div className="animate-fade-in-up-delay-2 mt-7 flex flex-wrap items-center justify-center gap-2.5">
          {AXES.map(({ icon: Icon, label }) => (
            <span
              key={label}
              className="inline-flex items-center gap-2 rounded-full border border-surface-700/60 bg-surface-900/50 px-3.5 py-1.5 text-xs font-medium text-neutral-300"
            >
              <Icon className="h-3.5 w-3.5 text-brand-400" />
              {label}
            </span>
          ))}
        </div>

        {/* CTAs */}
        <div className="animate-fade-in-up-delay-3 mt-10 flex flex-col items-center gap-4 sm:flex-row sm:justify-center">
          <a
            href="#get-started"
            className="group flex w-full items-center justify-center gap-2 rounded-xl bg-brand-500 px-7 py-3.5 font-display text-base font-semibold text-surface-950 transition hover:bg-brand-400 hover:shadow-[0_0_30px_rgba(37,194,160,0.2)] sm:w-auto"
          >
            Get Started
            <ChevronRight className="h-4 w-4 transition-transform group-hover:translate-x-0.5" />
          </a>
          <a
            href="#brand-loop"
            className="group flex w-full items-center justify-center gap-2 rounded-xl border border-surface-600 bg-surface-900/50 px-7 py-3.5 font-display text-base font-medium text-neutral-200 transition hover:border-brand-500/30 hover:text-white sm:w-auto"
          >
            How it works
            <ChevronRight className="h-4 w-4 transition-transform group-hover:translate-x-0.5" />
          </a>
        </div>

        {/* Terminal demo */}
        <div className="animate-fade-in-up-delay-4 mt-14 overflow-hidden rounded-2xl terminal-window shadow-2xl glow-teal">
          {/* Window chrome */}
          <div className="flex items-center gap-2 border-b border-brand-500/8 px-5 py-3">
            <div className="h-2.5 w-2.5 rounded-full bg-accent-rose/50" />
            <div className="h-2.5 w-2.5 rounded-full bg-accent-amber/50" />
            <div className="h-2.5 w-2.5 rounded-full bg-brand-500/50" />
            <span className="ml-3 font-mono text-xs text-neutral-600">~/project</span>
          </div>

          {/* Terminal content */}
          <div className="min-h-[280px] p-6 text-left font-mono text-sm">
            {/* Command line with typing */}
            <div className="flex items-start gap-2">
              <span className="text-brand-400 select-none">$</span>
              <span className="break-all text-neutral-200">
                {current.cmd.slice(0, charIndex)}
                {typing && <span className="cursor-blink text-brand-400">|</span>}
              </span>
            </div>

            {/* Output lines */}
            {!typing && (
              <div className="mt-3 space-y-1.5">
                {current.lines.slice(0, lineIndex).map((line, i) => (
                  <div
                    key={`${cmdIndex}-${i}`}
                    className={`${line.color}`}
                    style={{ animation: 'slide-in-right 0.3s ease-out forwards' }}
                  >
                    {line.text}
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Progress dots */}
          <div className="flex justify-center gap-2 pb-4">
            {COMMANDS.map((_, i) => (
              <div
                key={i}
                className={`h-1 rounded-full transition-all duration-500 ${
                  i === cmdIndex ? 'w-6 bg-brand-400/50' : 'w-1.5 bg-surface-600'
                }`}
              />
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}
