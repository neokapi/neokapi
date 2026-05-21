import { Terminal, Menu, X } from 'lucide-react'
import { useState } from 'react'

// lucide-react no longer ships brand icons, so render the GitHub mark inline.
function GithubIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M12 .5C5.37.5 0 5.87 0 12.5c0 5.3 3.44 9.8 8.21 11.39.6.11.82-.26.82-.58 0-.29-.01-1.05-.02-2.06-3.34.73-4.04-1.61-4.04-1.61-.55-1.39-1.34-1.76-1.34-1.76-1.09-.75.08-.73.08-.73 1.2.08 1.84 1.24 1.84 1.24 1.07 1.83 2.81 1.3 3.5.99.11-.78.42-1.3.76-1.6-2.67-.3-5.47-1.33-5.47-5.93 0-1.31.47-2.38 1.24-3.22-.12-.3-.54-1.52.12-3.18 0 0 1.01-.32 3.3 1.23a11.5 11.5 0 0 1 6 0c2.29-1.55 3.3-1.23 3.3-1.23.66 1.66.24 2.88.12 3.18.77.84 1.24 1.91 1.24 3.22 0 4.61-2.8 5.62-5.48 5.92.43.37.81 1.1.81 2.22 0 1.61-.01 2.9-.01 3.29 0 .32.22.7.83.58A12 12 0 0 0 24 12.5C24 5.87 18.63.5 12 .5z" />
    </svg>
  )
}

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
            <GithubIcon className="h-4 w-4" />
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
