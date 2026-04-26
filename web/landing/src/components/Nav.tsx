import { Github, Terminal, Menu, X } from 'lucide-react'
import { useState } from 'react'

export function Nav() {
  const [open, setOpen] = useState(false)

  return (
    <nav className="fixed top-0 left-0 right-0 z-50 border-b border-brand-500/8 bg-surface-950/85 backdrop-blur-xl">
      <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-6">
        <a href="#" className="flex items-center gap-2.5 font-display text-lg font-bold tracking-tight text-white">
          <img src={`${import.meta.env.BASE_URL}hero-logo.png`} alt="Neokapi" className="h-8 w-8 rounded-lg" />
          neokapi
        </a>

        <div className="hidden items-center gap-8 md:flex">
          <a href="#features" className="text-sm text-neutral-400 transition hover:text-brand-400">Features</a>
          <a href="#cli" className="text-sm text-neutral-400 transition hover:text-brand-400">CLI</a>
          <a href="#desktop" className="text-sm text-neutral-400 transition hover:text-brand-400">Desktop</a>
          <a href="#pipeline" className="text-sm text-neutral-400 transition hover:text-brand-400">Architecture</a>
          <a href="#get-started" className="text-sm text-neutral-400 transition hover:text-brand-400">Get Started</a>
        </div>

        <div className="hidden items-center gap-3 md:flex">
          <a
            href="https://github.com/neokapi/neokapi"
            target="_blank"
            rel="noopener"
            className="flex items-center gap-2 rounded-lg border border-surface-600 px-3 py-1.5 text-sm text-neutral-300 transition hover:border-brand-500/30 hover:text-white"
          >
            <Github className="h-4 w-4" />
            GitHub
          </a>
          <a
            href="#get-started"
            className="flex items-center gap-2 rounded-lg bg-brand-500 px-3 py-1.5 text-sm font-medium text-surface-950 transition hover:bg-brand-400"
          >
            <Terminal className="h-4 w-4" />
            Get Started
          </a>
        </div>

        <button onClick={() => setOpen(!open)} className="text-neutral-400 md:hidden">
          {open ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
        </button>
      </div>

      {open && (
        <div className="border-t border-surface-700 bg-surface-950/95 px-6 py-4 md:hidden">
          <div className="flex flex-col gap-3">
            <a href="#features" onClick={() => setOpen(false)} className="text-sm text-neutral-400">Features</a>
            <a href="#cli" onClick={() => setOpen(false)} className="text-sm text-neutral-400">CLI</a>
            <a href="#desktop" onClick={() => setOpen(false)} className="text-sm text-neutral-400">Desktop</a>
            <a href="#pipeline" onClick={() => setOpen(false)} className="text-sm text-neutral-400">Architecture</a>
            <a href="#get-started" onClick={() => setOpen(false)} className="text-sm text-neutral-400">Get Started</a>
            <div className="mt-2 flex items-center gap-3">
              <a href="https://github.com/neokapi/neokapi" target="_blank" rel="noopener" className="text-sm text-neutral-400">GitHub</a>
            </div>
          </div>
        </div>
      )}
    </nav>
  )
}
