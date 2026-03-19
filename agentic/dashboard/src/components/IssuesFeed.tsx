import { motion } from 'framer-motion';
import { CircleDot, CheckCircle2 } from 'lucide-react';
import { issues } from '../data/issues';

export default function IssuesFeed() {
  return (
    <motion.section
      className="px-6 py-12"
      initial={{ opacity: 0, y: 30 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true }}
      transition={{ duration: 0.6 }}
    >
      <div className="mx-auto max-w-5xl">
        <h2 className="mb-6 font-[family-name:var(--font-display)] text-2xl font-bold text-[var(--color-text-primary)]">
          Agent-Filed Issues
        </h2>
        <div className="overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-bg-card)]">
          <div className="divide-y divide-[var(--color-border)]">
            {issues.map((issue, i) => (
              <motion.div
                key={issue.id}
                className="flex items-start gap-3 px-5 py-3.5 transition-colors hover:bg-[var(--color-bg-elevated)]"
                initial={{ opacity: 0 }}
                whileInView={{ opacity: 1 }}
                viewport={{ once: true }}
                transition={{ duration: 0.3, delay: i * 0.05 }}
              >
                {/* Status icon */}
                {issue.status === 'open' ? (
                  <CircleDot size={16} className="mt-0.5 flex-shrink-0 text-green-500" />
                ) : (
                  <CheckCircle2 size={16} className="mt-0.5 flex-shrink-0 text-[var(--color-accent-violet)]" />
                )}

                {/* Content */}
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-medium text-[var(--color-text-primary)]">
                      {issue.title}
                    </span>
                    {issue.labels.map((label) => (
                      <span
                        key={label.name}
                        className="rounded-full px-2 py-0.5 text-[10px] font-medium"
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
                  <div className="mt-1 flex items-center gap-2 text-xs text-[var(--color-text-muted)]">
                    <span>{issue.agentAvatar}</span>
                    <span>{issue.agentName}</span>
                    <span>
                      {issue.daysAgo === 0
                        ? 'today'
                        : issue.daysAgo === 1
                        ? 'yesterday'
                        : `${issue.daysAgo} days ago`}
                    </span>
                  </div>
                </div>
              </motion.div>
            ))}
          </div>
        </div>
      </div>
    </motion.section>
  );
}
