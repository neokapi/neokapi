import { motion, AnimatePresence } from 'framer-motion';
import { activityFeed } from '../data/activity';
import { accentColorMap } from '../data/agents';
import { useFilter } from '../context/FilterContext';

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
  const { selectedWorkspace } = useFilter();

  const filtered = selectedWorkspace
    ? activityFeed.filter((e) => e.workspace === selectedWorkspace)
    : activityFeed;

  return (
    <div
      className="flex h-full flex-col rounded-xl"
      style={{
        backgroundColor: 'rgb(var(--bg-card))',
        border: '1px solid rgb(var(--border))',
      }}
    >
      <div
        className="px-4 py-3"
        style={{ borderBottom: '1px solid rgb(var(--border))' }}
      >
        <h3
          className="font-mono text-sm font-semibold"
          style={{ color: 'rgb(var(--text-primary))' }}
        >
          Activity Feed
        </h3>
      </div>
      <div
        className="relative flex-1 overflow-y-auto p-3"
        style={{ maxHeight: '600px', scrollBehavior: 'smooth' }}
      >
        <AnimatePresence>
          {filtered.map((entry, i) => {
            const color = accentColorMap[entry.accentColor] || '#d9a03c';
            return (
              <motion.div
                key={entry.id}
                className="mb-2 rounded-lg p-3"
                style={{
                  backgroundColor: 'rgb(var(--bg-elevated))',
                  borderLeft: `3px solid ${color}`,
                }}
                initial={{ opacity: 0, x: -20 }}
                animate={{ opacity: 1, x: 0 }}
                transition={{ duration: 0.3, delay: i * 0.03 }}
              >
                <div className="flex items-start gap-2">
                  <span className="flex-shrink-0 text-lg">{entry.agentAvatar}</span>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span
                        className="text-xs font-semibold"
                        style={{ color }}
                      >
                        {entry.agentName}
                      </span>
                      <span
                        className="rounded-full px-1.5 py-0.5 font-mono text-[9px]"
                        style={{
                          backgroundColor: 'rgb(var(--bg-card))',
                          color: 'rgb(var(--text-muted))',
                        }}
                      >
                        {entry.workspace}
                      </span>
                      <span
                        className="font-mono text-[10px]"
                        style={{ color: 'rgb(var(--text-muted))' }}
                      >
                        {formatRelativeTime(entry.timestamp)}
                      </span>
                    </div>
                    <p
                      className="mt-0.5 text-xs leading-relaxed"
                      style={{ color: 'rgb(var(--text-secondary))' }}
                    >
                      {entry.action}
                    </p>
                    {entry.toolsUsed.length > 0 && (
                      <div className="mt-1.5 flex flex-wrap gap-1">
                        {entry.toolsUsed.map((tool) => (
                          <span
                            key={tool}
                            className="rounded-md px-1.5 py-0.5 font-mono text-[9px]"
                            style={{
                              backgroundColor: 'rgb(var(--bg-card))',
                              color: 'rgb(var(--text-muted))',
                            }}
                          >
                            {tool}
                          </span>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              </motion.div>
            );
          })}
        </AnimatePresence>
        {/* Fade gradient at bottom */}
        <div
          className="pointer-events-none sticky bottom-0 left-0 right-0 h-12"
          style={{
            background: `linear-gradient(to top, rgb(var(--bg-card)), transparent)`,
          }}
        />
      </div>
    </div>
  );
}
