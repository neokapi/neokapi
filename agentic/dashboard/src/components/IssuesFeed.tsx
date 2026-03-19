import { motion } from 'framer-motion';
import { CircleDot, CheckCircle2 } from 'lucide-react';
import { issues } from '../data/issues';
import { useFilter } from '../context/FilterContext';

export default function IssuesFeed() {
  const { selectedWorkspace } = useFilter();

  const filtered = selectedWorkspace
    ? issues.filter((iss) => iss.workspace === selectedWorkspace)
    : issues;

  return (
    <div className="flex h-full flex-col rounded-xl border border-[var(--color-border)] bg-[var(--color-bg-card)]">
      <div className="border-b border-[var(--color-border)] px-4 py-3">
        <h3 className="font-[family-name:var(--font-mono)] text-sm font-semibold text-[var(--color-text-primary)]">
          Agent-Filed Issues
        </h3>
      </div>
      <div className="relative flex-1 overflow-y-auto" style={{ maxHeight: '600px' }}>
        <div className="divide-y divide-[var(--color-border)]">
          {filtered.map((issue, i) => (
            <motion.div
              key={issue.id}
              className="flex items-start gap-3 px-4 py-3 transition-colors hover:bg-[var(--color-bg-elevated)]"
              initial={{ opacity: 0 }}
              whileInView={{ opacity: 1 }}
              viewport={{ once: true }}
              transition={{ duration: 0.3, delay: i * 0.05 }}
            >
              {/* Status icon */}
              {issue.status === 'open' ? (
                <CircleDot size={14} className="mt-0.5 flex-shrink-0 text-green-500" />
              ) : (
                <CheckCircle2 size={14} className="mt-0.5 flex-shrink-0 text-[var(--color-accent-violet)]" />
              )}

              {/* Content */}
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-1.5">
                  <span className="text-xs font-medium text-[var(--color-text-primary)]">
                    {issue.title}
                  </span>
                  {issue.labels.map((label) => (
                    <span
                      key={label.name}
                      className="rounded-full px-1.5 py-0.5 text-[9px] font-medium"
                      style={{
                        backgroundColor: `${label.color}20`,
                        color: label.color,
                        border: `1px solid ${label.color}40`,
                      }}
                    >
                      {label.name}
                    </span>
                  ))}
                </div>
                <div className="mt-1 flex items-center gap-2 text-[10px] text-[var(--color-text-muted)]">
                  <span>{issue.agentAvatar}</span>
                  <span>{issue.agentName}</span>
                  <span className="rounded-full bg-[var(--color-bg-elevated)] px-1.5 py-0.5 font-[family-name:var(--font-mono)]">
                    {issue.workspace}
                  </span>
                  <span>
                    {issue.daysAgo === 0
                      ? 'today'
                      : issue.daysAgo === 1
                      ? 'yesterday'
                      : `${issue.daysAgo}d ago`}
                  </span>
                </div>
              </div>
            </motion.div>
          ))}
        </div>
      </div>
    </div>
  );
}
