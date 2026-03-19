import { motion } from 'framer-motion';
import { Clock, Cpu, Globe, Zap, CheckCircle2, XCircle } from 'lucide-react';
import type { Agent } from '../data/agents';
import { accentColorMap } from '../data/agents';

interface AgentCardProps {
  agent: Agent;
  index: number;
}

function formatRelativeTime(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime();
  const diffMins = Math.floor(diffMs / 60_000);
  if (diffMins < 1) return 'just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  return `${Math.floor(diffHours / 24)}d ago`;
}

export default function AgentCard({ agent, index }: AgentCardProps) {
  const color = accentColorMap[agent.accentColor] || '#f59e0b';

  const statusColors = {
    active: '#22c55e',
    idle: '#f59e0b',
    sleeping: '#6b7280',
  };

  const statusLabel = {
    active: 'Active',
    idle: 'Idle',
    sleeping: 'Sleeping',
  };

  return (
    <motion.div
      className="group relative min-w-[280px] max-w-[320px] flex-shrink-0 overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-bg-card)] p-5"
      initial={{ opacity: 0, y: 30 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true }}
      transition={{ duration: 0.5, delay: index * 0.1 }}
      whileHover={{ scale: 1.02 }}
      style={{
        boxShadow: `0 0 0 0 ${color}00`,
      }}
      onMouseEnter={(e) => {
        (e.currentTarget as HTMLElement).style.boxShadow = `0 0 30px 0 ${color}20`;
      }}
      onMouseLeave={(e) => {
        (e.currentTarget as HTMLElement).style.boxShadow = `0 0 0 0 ${color}00`;
      }}
    >
      {/* Subtle top accent line */}
      <div
        className="absolute inset-x-0 top-0 h-0.5"
        style={{ backgroundColor: color }}
      />

      {/* Header: avatar + status */}
      <div className="flex items-start justify-between">
        <span className="text-4xl">{agent.avatar}</span>
        <div className="flex items-center gap-1.5">
          <span
            className="inline-block h-2 w-2 rounded-full"
            style={{
              backgroundColor: statusColors[agent.status],
              boxShadow: agent.status === 'active' ? `0 0 8px ${statusColors[agent.status]}` : 'none',
            }}
          />
          <span className="font-[family-name:var(--font-mono)] text-xs text-[var(--color-text-muted)]">
            {statusLabel[agent.status]}
          </span>
        </div>
      </div>

      {/* Name + title */}
      <h3 className="mt-3 text-lg font-semibold text-[var(--color-text-primary)]">
        {agent.name}
      </h3>
      <p className="text-sm text-[var(--color-text-secondary)]">{agent.title}</p>

      {/* Badges row */}
      <div className="mt-3 flex flex-wrap gap-2">
        <span
          className="rounded-full px-2.5 py-0.5 text-xs font-medium"
          style={{
            backgroundColor: `${color}20`,
            color: color,
            border: `1px solid ${color}40`,
          }}
        >
          {agent.role}
        </span>
        <span className="flex items-center gap-1 rounded-full border border-[var(--color-border)] bg-[var(--color-bg-elevated)] px-2.5 py-0.5 text-xs text-[var(--color-text-secondary)]">
          <Cpu size={10} />
          {agent.model}
        </span>
      </div>

      {/* Schedule + Language */}
      <div className="mt-3 space-y-1.5">
        <div className="flex items-center gap-1.5 text-xs text-[var(--color-text-muted)]">
          <Clock size={11} />
          {agent.schedule}
        </div>
        {agent.targetLanguage && (
          <div className="flex items-center gap-1.5 text-xs text-[var(--color-text-muted)]">
            <Globe size={11} />
            {agent.targetLanguage}
          </div>
        )}
      </div>

      {/* Last session info */}
      <div className="mt-3 rounded-lg bg-[var(--color-bg-elevated)] p-2.5">
        <div className="flex items-center gap-1.5 text-xs text-[var(--color-text-muted)]">
          {agent.lastSession.status === "succeeded" ? (
            <CheckCircle2 size={11} className="text-green-400" />
          ) : (
            <XCircle size={11} className="text-red-400" />
          )}
          <span>Last session: {agent.lastSession.duration}</span>
          <span className="ml-auto font-[family-name:var(--font-mono)] text-[10px]">
            {formatRelativeTime(agent.lastSession.time)}
          </span>
        </div>
      </div>

      {/* Personality traits */}
      <div className="mt-3 flex flex-wrap gap-1.5">
        {agent.personality.map((trait) => (
          <span
            key={trait}
            className="rounded-md bg-[var(--color-bg-elevated)] px-2 py-0.5 text-xs text-[var(--color-text-secondary)]"
          >
            {trait}
          </span>
        ))}
      </div>

      {/* Stats row — operations focus */}
      <div className="mt-4 grid grid-cols-3 gap-2 border-t border-[var(--color-border)] pt-3">
        <div className="text-center">
          <div className="font-[family-name:var(--font-mono)] text-sm font-semibold text-[var(--color-text-primary)]">
            {agent.stats.sessionsThisWeek}
          </div>
          <div className="text-[10px] text-[var(--color-text-muted)]">sessions/wk</div>
        </div>
        <div className="text-center">
          <div className="flex items-center justify-center gap-0.5 font-[family-name:var(--font-mono)] text-sm font-semibold text-[var(--color-text-primary)]">
            <Zap size={10} className="text-[var(--color-accent-amber)]" />
            {agent.stats.toolCallsToday}
          </div>
          <div className="text-[10px] text-[var(--color-text-muted)]">tools today</div>
        </div>
        <div className="text-center">
          <div className="font-[family-name:var(--font-mono)] text-sm font-semibold text-[var(--color-text-primary)]">
            {agent.stats.issuesFiled}
          </div>
          <div className="text-[10px] text-[var(--color-text-muted)]">issues</div>
        </div>
      </div>
    </motion.div>
  );
}
