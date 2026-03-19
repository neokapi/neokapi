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
  const color = accentColorMap[agent.accentColor] || '#d9a03c';

  const statusColors = {
    active: 'rgb(var(--success))',
    idle: 'rgb(var(--warning))',
    sleeping: 'rgb(var(--text-muted))',
  };

  const statusLabel = {
    active: 'Active',
    idle: 'Idle',
    sleeping: 'Sleeping',
  };

  return (
    <motion.div
      className="group relative min-w-[260px] max-w-[320px] flex-shrink-0 overflow-hidden rounded-xl p-5"
      style={{
        backgroundColor: 'rgb(var(--bg-card))',
        border: '1px solid rgb(var(--border))',
        backdropFilter: 'blur(12px)',
      }}
      initial={{ opacity: 0, y: 30 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true }}
      transition={{ duration: 0.5, delay: index * 0.1 }}
      whileHover={{ scale: 1.02 }}
      onMouseEnter={(e) => {
        (e.currentTarget as HTMLElement).style.boxShadow = `0 0 30px 0 ${color}25`;
        (e.currentTarget as HTMLElement).style.borderColor = `${color}40`;
      }}
      onMouseLeave={(e) => {
        (e.currentTarget as HTMLElement).style.boxShadow = 'none';
        (e.currentTarget as HTMLElement).style.borderColor = 'rgb(var(--border))';
      }}
    >
      {/* Top accent line - 2px */}
      <div
        className="absolute inset-x-0 top-0"
        style={{ height: '2px', backgroundColor: color }}
      />

      {/* Header: avatar + status */}
      <div className="flex items-start justify-between">
        <span className="text-3xl sm:text-4xl">{agent.avatar}</span>
        <div className="flex items-center gap-1.5">
          <span
            className="inline-block h-2 w-2 rounded-full"
            style={{
              backgroundColor: statusColors[agent.status],
              boxShadow: agent.status === 'active' ? `0 0 8px ${statusColors[agent.status]}` : 'none',
            }}
          />
          <span
            className="font-mono text-xs"
            style={{ color: 'rgb(var(--text-muted))' }}
          >
            {statusLabel[agent.status]}
          </span>
        </div>
      </div>

      {/* Name + title */}
      <h3
        className="mt-3 text-base font-semibold sm:text-lg"
        style={{ color: 'rgb(var(--text-primary))' }}
      >
        {agent.name}
      </h3>
      <p className="text-sm" style={{ color: 'rgb(var(--text-secondary))' }}>
        {agent.title}
      </p>

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
        <span
          className="flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs"
          style={{
            backgroundColor: 'rgb(var(--bg-elevated))',
            color: 'rgb(var(--text-secondary))',
            border: '1px solid rgb(var(--border))',
          }}
        >
          <Cpu size={10} />
          {agent.model}
        </span>
      </div>

      {/* Schedule + Language */}
      <div className="mt-3 space-y-1.5">
        <div
          className="flex items-center gap-1.5 text-xs"
          style={{ color: 'rgb(var(--text-muted))' }}
        >
          <Clock size={11} />
          {agent.schedule}
        </div>
        {agent.targetLanguage && (
          <div
            className="flex items-center gap-1.5 text-xs"
            style={{ color: 'rgb(var(--text-muted))' }}
          >
            <Globe size={11} />
            {agent.targetLanguage}
          </div>
        )}
      </div>

      {/* Last session info */}
      <div
        className="mt-3 rounded-lg p-2.5"
        style={{ backgroundColor: 'rgb(var(--bg-elevated))' }}
      >
        <div
          className="flex items-center gap-1.5 text-xs"
          style={{ color: 'rgb(var(--text-muted))' }}
        >
          {agent.lastSession.status === 'succeeded' ? (
            <CheckCircle2 size={11} style={{ color: 'rgb(var(--success))' }} />
          ) : (
            <XCircle size={11} style={{ color: 'rgb(var(--danger))' }} />
          )}
          <span>Last session: {agent.lastSession.duration}</span>
          <span className="ml-auto font-mono text-[10px]">
            {formatRelativeTime(agent.lastSession.time)}
          </span>
        </div>
      </div>

      {/* Personality traits */}
      <div className="mt-3 flex flex-wrap gap-1.5">
        {agent.personality.map((trait) => (
          <span
            key={trait}
            className="rounded-md px-2 py-0.5 text-xs"
            style={{
              backgroundColor: 'rgb(var(--bg-elevated))',
              color: 'rgb(var(--text-secondary))',
            }}
          >
            {trait}
          </span>
        ))}
      </div>

      {/* Stats row */}
      <div
        className="mt-4 grid grid-cols-3 gap-2 pt-3"
        style={{ borderTop: '1px solid rgb(var(--border))' }}
      >
        <div className="text-center">
          <div
            className="font-mono text-sm font-semibold"
            style={{ color: 'rgb(var(--text-primary))' }}
          >
            {agent.stats.sessionsThisWeek}
          </div>
          <div className="text-[10px]" style={{ color: 'rgb(var(--text-muted))' }}>
            sessions/wk
          </div>
        </div>
        <div className="text-center">
          <div
            className="flex items-center justify-center gap-0.5 font-mono text-sm font-semibold"
            style={{ color: 'rgb(var(--text-primary))' }}
          >
            <Zap size={10} style={{ color: 'rgb(var(--accent))' }} />
            {agent.stats.toolCallsToday}
          </div>
          <div className="text-[10px]" style={{ color: 'rgb(var(--text-muted))' }}>
            tools today
          </div>
        </div>
        <div className="text-center">
          <div
            className="font-mono text-sm font-semibold"
            style={{ color: 'rgb(var(--text-primary))' }}
          >
            {agent.stats.issuesFiled}
          </div>
          <div className="text-[10px]" style={{ color: 'rgb(var(--text-muted))' }}>
            issues
          </div>
        </div>
      </div>
    </motion.div>
  );
}
