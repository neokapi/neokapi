import { motion, AnimatePresence } from 'framer-motion';
import { activityFeed } from '../data/activity';
import { accentColorMap } from '../data/agents';

function formatRelativeTime(date: Date): string {
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60_000);
  if (diffMins < 1) return 'just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  return `${Math.floor(diffHours / 24)}d ago`;
}

export default function ActivityFeed() {
  return (
    <div className="flex h-full flex-col rounded-xl border border-[var(--color-border)] bg-[var(--color-bg-card)]">
      <div className="border-b border-[var(--color-border)] px-4 py-3">
        <h3 className="font-[family-name:var(--font-mono)] text-sm font-semibold text-[var(--color-text-primary)]">
          Activity Feed
        </h3>
      </div>
      <div className="relative flex-1 overflow-y-auto p-3" style={{ maxHeight: '600px' }}>
        <AnimatePresence>
          {activityFeed.map((entry, i) => {
            const color = accentColorMap[entry.accentColor] || '#f59e0b';
            return (
              <motion.div
                key={entry.id}
                className="mb-2 rounded-lg bg-[var(--color-bg-elevated)] p-3"
                initial={{ opacity: 0, x: -20 }}
                animate={{ opacity: 1, x: 0 }}
                transition={{ duration: 0.3, delay: i * 0.03 }}
              >
                <div className="flex items-start gap-2">
                  <span className="flex-shrink-0 text-lg">{entry.agentAvatar}</span>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span
                        className="text-xs font-semibold"
                        style={{ color }}
                      >
                        {entry.agentName}
                      </span>
                      <span className="font-[family-name:var(--font-mono)] text-[10px] text-[var(--color-text-muted)]">
                        {formatRelativeTime(entry.timestamp)}
                      </span>
                    </div>
                    <p className="mt-0.5 text-xs leading-relaxed text-[var(--color-text-secondary)]">
                      {entry.action}
                    </p>
                  </div>
                </div>
              </motion.div>
            );
          })}
        </AnimatePresence>
        {/* Fade gradient at bottom */}
        <div className="pointer-events-none sticky bottom-0 left-0 right-0 h-12 bg-gradient-to-t from-[var(--color-bg-card)] to-transparent" />
      </div>
    </div>
  );
}
