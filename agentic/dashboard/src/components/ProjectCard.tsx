import { motion } from 'framer-motion';
import { ExternalLink } from 'lucide-react';
import type { Project } from '../data/projects';

const langColors: Record<string, string> = {
  'fr-FR': '#3b82f6',
  'de-DE': '#f43f5e',
  'ja-JP': '#8b5cf6',
};

interface ProjectCardProps {
  project: Project;
  index: number;
}

export default function ProjectCard({ project, index }: ProjectCardProps) {
  return (
    <motion.div
      className="rounded-xl border border-[var(--color-border)] bg-[var(--color-bg-card)] p-5"
      initial={{ opacity: 0, y: 20 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true }}
      transition={{ duration: 0.5, delay: index * 0.1 }}
    >
      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <h3 className="text-lg font-semibold text-[var(--color-text-primary)]">
            {project.name}
          </h3>
          <a
            href={`https://github.com/${project.upstream}`}
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-1 text-xs text-[var(--color-text-muted)] transition-colors hover:text-[var(--color-text-secondary)]"
          >
            {project.upstream}
            <ExternalLink size={10} />
          </a>
        </div>
        <span className="rounded-full border border-[var(--color-border)] bg-[var(--color-bg-elevated)] px-2 py-0.5 text-xs text-[var(--color-text-muted)]">
          {project.license}
        </span>
      </div>

      {/* Format pills */}
      <div className="mt-3 flex gap-1.5">
        {project.formatTypes.map((fmt) => (
          <span
            key={fmt}
            className="rounded-md bg-[var(--color-bg-elevated)] px-2 py-0.5 font-[family-name:var(--font-mono)] text-[10px] text-[var(--color-text-secondary)]"
          >
            {fmt}
          </span>
        ))}
      </div>

      {/* Language progress bars */}
      <div className="mt-4 space-y-3">
        {project.languages.map((lang) => {
          const color = langColors[lang.locale] || '#94a3b8';
          return (
            <div key={lang.locale}>
              <div className="mb-1 flex items-center justify-between">
                <span className="text-xs text-[var(--color-text-secondary)]">
                  {lang.label}
                </span>
                <span className="font-[family-name:var(--font-mono)] text-xs text-[var(--color-text-muted)]">
                  {lang.blocksTranslated}/{lang.blocksTotal}
                </span>
              </div>
              <div className="h-1.5 w-full overflow-hidden rounded-full bg-[var(--color-bg-elevated)]">
                <motion.div
                  className="h-full rounded-full"
                  style={{ backgroundColor: color }}
                  initial={{ width: 0 }}
                  whileInView={{ width: `${lang.progress}%` }}
                  viewport={{ once: true }}
                  transition={{ duration: 1, delay: 0.3 }}
                />
              </div>
            </div>
          );
        })}
      </div>

      {/* TM reuse gauge */}
      <div className="mt-4 flex items-center justify-between border-t border-[var(--color-border)] pt-3">
        <span className="text-xs text-[var(--color-text-muted)]">TM Reuse Rate</span>
        <div className="flex items-center gap-2">
          <div className="h-1.5 w-16 overflow-hidden rounded-full bg-[var(--color-bg-elevated)]">
            <div
              className="h-full rounded-full bg-[var(--color-accent-amber)]"
              style={{ width: `${project.tmReuseRate}%` }}
            />
          </div>
          <span className="font-[family-name:var(--font-mono)] text-xs font-semibold text-[var(--color-accent-amber)]">
            {project.tmReuseRate}%
          </span>
        </div>
      </div>
    </motion.div>
  );
}
