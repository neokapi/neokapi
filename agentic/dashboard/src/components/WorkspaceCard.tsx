import { motion } from 'framer-motion';
import { ExternalLink, Users, Globe } from 'lucide-react';
import type { Workspace } from '../data/workspaces';
import { useFilter } from '../context/FilterContext';

interface WorkspaceCardProps {
  workspace: Workspace;
  index: number;
}

const statusColors: Record<string, string> = {
  active: 'rgb(var(--success))',
  idle: 'rgb(var(--text-muted))',
  paused: 'rgb(var(--warning))',
};

const statusLabels: Record<string, string> = {
  active: 'Active',
  idle: 'Idle',
  paused: 'Paused',
};

function formatRelativeTime(iso: string): string {
  if (!iso) return 'Never';
  const diffMs = Date.now() - new Date(iso).getTime();
  const diffMins = Math.floor(diffMs / 60_000);
  if (diffMins < 1) return 'just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  return `${Math.floor(diffHours / 24)}d ago`;
}

export default function WorkspaceCard({ workspace, index }: WorkspaceCardProps) {
  const { setSelectedWorkspace } = useFilter();
  const isClickable = workspace.status === 'active';

  return (
    <motion.div
      className={`rounded-xl p-5 ${
        isClickable
          ? 'cursor-pointer'
          : 'opacity-60'
      }`}
      style={{
        backgroundColor: 'rgb(var(--bg-card))',
        border: '1px solid rgb(var(--border))',
      }}
      initial={{ opacity: 0, y: 20 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true }}
      transition={{ duration: 0.5, delay: index * 0.1 }}
      onClick={() => isClickable && setSelectedWorkspace(workspace.id)}
      whileHover={isClickable ? { scale: 1.01 } : undefined}
    >
      <div className="flex items-start justify-between">
        <div>
          <h3
            className="text-base font-semibold sm:text-lg"
            style={{ color: 'rgb(var(--text-primary))' }}
          >
            {workspace.name}
          </h3>
          <a
            href={`https://github.com/${workspace.upstream}`}
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-1 text-xs transition-colors hover:underline"
            style={{ color: 'rgb(var(--text-muted))' }}
            onClick={(e) => e.stopPropagation()}
          >
            {workspace.upstream}
            <ExternalLink size={10} />
          </a>
        </div>
        <div className="flex items-center gap-1.5">
          <span
            className="inline-block h-2 w-2 rounded-full"
            style={{
              backgroundColor: statusColors[workspace.status],
              boxShadow:
                workspace.status === 'active'
                  ? `0 0 8px ${statusColors[workspace.status]}`
                  : 'none',
            }}
          />
          <span
            className="font-mono text-xs"
            style={{ color: 'rgb(var(--text-muted))' }}
          >
            {statusLabels[workspace.status]}
          </span>
        </div>
      </div>

      <p
        className="mt-2 text-sm"
        style={{ color: 'rgb(var(--text-secondary))' }}
      >
        {workspace.description}
      </p>

      <div
        className="mt-4 flex flex-wrap items-center gap-4 pt-3"
        style={{ borderTop: '1px solid rgb(var(--border))' }}
      >
        <div
          className="flex items-center gap-1.5 text-xs"
          style={{ color: 'rgb(var(--text-muted))' }}
        >
          <Users size={12} />
          {workspace.agentCount} agent{workspace.agentCount !== 1 ? 's' : ''}
        </div>
        <div
          className="flex items-center gap-1.5 text-xs"
          style={{ color: 'rgb(var(--text-muted))' }}
        >
          <Globe size={12} />
          {workspace.languages.join(', ')}
        </div>
        {workspace.lastActivity && (
          <div
            className="ml-auto font-mono text-[10px]"
            style={{ color: 'rgb(var(--text-muted))' }}
          >
            {formatRelativeTime(workspace.lastActivity)}
          </div>
        )}
      </div>
    </motion.div>
  );
}
