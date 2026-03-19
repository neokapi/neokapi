import { useFilter } from '../context/FilterContext';
import { workspaces } from '../data/workspaces';

export default function WorkspaceSelector() {
  const { selectedWorkspace, setSelectedWorkspace } = useFilter();

  const options = [
    {
      id: null,
      label: 'All Workspaces',
      disabled: false,
      agentCount: workspaces.reduce((s, w) => s + w.agentCount, 0),
      status: 'active' as const,
    },
    ...workspaces.map((w) => ({
      id: w.id,
      label: w.name,
      disabled: w.status === 'idle',
      agentCount: w.agentCount,
      status: w.status,
    })),
  ];

  return (
    <div className="flex items-center gap-2 overflow-x-auto px-4 py-3 sm:px-6">
      {options.map((opt) => {
        const isActive = selectedWorkspace === opt.id;
        const statusColor =
          opt.status === 'active'
            ? 'rgb(var(--success))'
            : opt.status === 'paused'
              ? 'rgb(var(--warning))'
              : 'rgb(var(--text-muted))';

        return (
          <button
            key={opt.id ?? 'all'}
            onClick={() => !opt.disabled && setSelectedWorkspace(opt.id)}
            disabled={opt.disabled}
            className="flex min-h-[44px] items-center gap-2 whitespace-nowrap rounded-full px-4 py-1.5 text-sm font-medium transition-all sm:min-h-0"
            style={{
              opacity: opt.disabled ? 0.5 : 1,
              cursor: opt.disabled ? 'not-allowed' : 'pointer',
              backgroundColor: isActive
                ? 'rgb(var(--accent) / 0.15)'
                : 'rgb(var(--bg-card))',
              borderWidth: '1px',
              borderStyle: 'solid',
              borderColor: isActive
                ? 'rgb(var(--accent))'
                : 'rgb(var(--border))',
              color: isActive
                ? 'rgb(var(--accent))'
                : 'rgb(var(--text-secondary))',
            }}
          >
            <span
              className="inline-block h-2 w-2 rounded-full"
              style={{
                backgroundColor: statusColor,
                boxShadow:
                  opt.status === 'active' && isActive
                    ? `0 0 6px ${statusColor}`
                    : 'none',
              }}
            />
            {opt.label}
            {opt.agentCount > 0 && (
              <span
                className="rounded-full px-1.5 py-0.5 font-mono text-[10px]"
                style={{ backgroundColor: 'rgb(var(--bg-elevated))' }}
              >
                {opt.agentCount}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
}
