import { Terminal, Menu, X } from 'lucide-react'
import { useState } from 'react'
import { GithubIcon } from './GithubIcon'

export function Nav() {
  const [open, setOpen] = useState(false)

  return (
    <nav className="fixed top-0 left-0 right-0 z-50 border-b border-neutral-800/50 bg-neutral-950/80 backdrop-blur-xl">
      <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-6">
        <a href="#" className="flex items-center gap-2 text-lg font-semibold tracking-tight text-white">
          <div className="flex h-7 w-7 items-center justify-center rounded-md bg-brand-500 text-sm font-bold text-white">B</div>
          bowrain
        </a>

        <div className="hidden items-center gap-8 md:flex">
          <a href="#pseudo-challenge" className="text-sm text-neutral-400 transition hover:text-white">Pseudo Challenge</a>
          <a href="#brand-challenge" className="text-sm text-neutral-400 transition hover:text-white">On-Brand Challenge</a>
          <a href="#platform" className="text-sm text-neutral-400 transition hover:text-white">Platform</a>
          <a href="#open-source" className="text-sm text-neutral-400 transition hover:text-white">Open Source</a>
          <a href="#plans" className="text-sm text-neutral-400 transition hover:text-white">Plans</a>
        </div>

        <div className="hidden items-center gap-3 md:flex">
          <a
            href="https://github.com/neokapi"
            target="_blank"
            rel="noopener"
            className="flex items-center gap-2 rounded-lg border border-neutral-700 px-3 py-1.5 text-sm text-neutral-300 transition hover:border-neutral-500 hover:text-white"
          >
            <GithubIcon className="h-4 w-4" />
            GitHub
          </a>
          <a
            href="#get-started"
            className="flex items-center gap-2 rounded-lg bg-brand-500 px-3 py-1.5 text-sm font-medium text-white transition hover:bg-brand-600"
          >
            <Terminal className="h-4 w-4" />
            Get Started
          </a>
        </div>

        <button onClick={() => setOpen(!open)} className="md:hidden text-neutral-400">
          {open ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
        </button>
      </div>

      {open && (
        <div className="border-t border-neutral-800/50 bg-neutral-950/95 px-6 py-4 md:hidden">
          <div className="flex flex-col gap-3">
            <a href="#pseudo-challenge" onClick={() => setOpen(false)} className="text-sm text-neutral-400">Pseudo Challenge</a>
            <a href="#brand-challenge" onClick={() => setOpen(false)} className="text-sm text-neutral-400">On-Brand Challenge</a>
            <a href="#platform" onClick={() => setOpen(false)} className="text-sm text-neutral-400">Platform</a>
            <a href="#open-source" onClick={() => setOpen(false)} className="text-sm text-neutral-400">Open Source</a>
            <a href="#plans" onClick={() => setOpen(false)} className="text-sm text-neutral-400">Plans</a>
          </div>
        </div>
      )}
    </nav>
  )
}
