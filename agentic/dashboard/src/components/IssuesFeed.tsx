import { motion } from 'framer-motion';
import { CircleDot, CheckCircle2 } from 'lucide-react';
import { issues } from '../data/issues';
import { accentColorMap, agents } from '../data/agents';
import { useFilter } from '../context/FilterContext';

export default function IssuesFeed() {
  const { selectedWorkspace } = useFilter();

  const filtered = selectedWorkspace
    ? issues.filter((iss) => iss.workspace === selectedWorkspace)
    : issues;

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
          Agent-Filed Issues
        </h3>
      </div>
      <div
        className="relative flex-1 overflow-y-auto"
        style={{ maxHeight: '600px' }}
      >
        <div>
          {filtered.map((issue, i) => {
            const agent = agents.find((a) => a.id === issue.agentId);
            const agentColor = agent
              ? accentColorMap[agent.accentColor] || '#d9a03c'
              : '#d9a03c';

            return (
              <motion.div
                key={issue.id}
                className="flex items-start gap-3 px-4 py-3 transition-colors min-h-[44px]"
                style={{
                  borderBottom: '1px solid rgb(var(--border))',
                  borderLeft: `3px solid ${agentColor}`,
                }}
                initial={{ opacity: 0 }}
                whileInView={{ opacity: 1 }}
                viewport={{ once: true }}
                transition={{ duration: 0.3, delay: i * 0.05 }}
                onMouseEnter={(e) => {
                  (e.currentTarget as HTMLElement).style.backgroundColor =
                    'rgb(var(--bg-elevated))';
                }}
                onMouseLeave={(e) => {
                  (e.currentTarget as HTMLElement).style.backgroundColor =
                    'transparent';
                }}
              >
                {/* Status icon */}
                {issue.status === 'open' ? (
                  <CircleDot
                    size={14}
                    className="mt-0.5 flex-shrink-0"
                    style={{ color: 'rgb(var(--success))' }}
                  />
                ) : (
                  <CheckCircle2
                    size={14}
                    className="mt-0.5 flex-shrink-0"
                    style={{ color: 'rgb(var(--text-muted))' }}
                  />
                )}

                {/* Content */}
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-1.5">
                    <span
                      className="text-xs font-medium"
                      style={{ color: 'rgb(var(--text-primary))' }}
                    >
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
                  <div
                    className="mt-1 flex flex-wrap items-center gap-2 text-[10px]"
                    style={{ color: 'rgb(var(--text-muted))' }}
                  >
                    <span>{issue.agentAvatar}</span>
                    <span>{issue.agentName}</span>
                    <span
                      className="rounded-full px-1.5 py-0.5 font-mono"
                      style={{ backgroundColor: 'rgb(var(--bg-elevated))' }}
                    >
                      {issue.workspace}
                    </span>
                    <span className="font-mono">
                      {issue.daysAgo === 0
                        ? 'today'
                        : issue.daysAgo === 1
                          ? 'yesterday'
                          : `${issue.daysAgo}d ago`}
                    </span>
                  </div>
                </div>
              </motion.div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
