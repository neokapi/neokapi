import { useFilter } from '../context/FilterContext';
import { workspaces } from '../data/workspaces';

const statusDot: Record<string, string> = {
  active: "#22c55e",
  idle: "#6b7280",
  paused: "#f59e0b",
};

export default function WorkspaceSelector() {
  const { selectedWorkspace, setSelectedWorkspace } = useFilter();

  const options = [
    { id: null, label: "All Workspaces", disabled: false, agentCount: workspaces.reduce((s, w) => s + w.agentCount, 0), status: "active" as const },
    ...workspaces.map((w) => ({ id: w.id, label: w.name, disabled: w.status === "idle", agentCount: w.agentCount, status: w.status })),
  ];

  return (
    <div className="flex items-center gap-2 overflow-x-auto px-6 py-3">
      {options.map((opt) => {
        const isActive = selectedWorkspace === opt.id;
        return (
          <button
            key={opt.id ?? "all"}
            onClick={() => !opt.disabled && setSelectedWorkspace(opt.id)}
            disabled={opt.disabled}
            className={`flex items-center gap-2 whitespace-nowrap rounded-full px-4 py-1.5 text-sm font-medium transition-all ${
              opt.disabled
                ? "cursor-not-allowed border border-[var(--color-border)] bg-[var(--color-bg-card)] text-[var(--color-text-muted)] opacity-50"
                : isActive
                ? "border border-[var(--color-accent-amber)] bg-[var(--color-accent-amber)]/15 text-[var(--color-accent-amber)]"
                : "border border-[var(--color-border)] bg-[var(--color-bg-card)] text-[var(--color-text-secondary)] hover:border-[var(--color-text-muted)] hover:text-[var(--color-text-primary)]"
            }`}
          >
            <span
              className="inline-block h-2 w-2 rounded-full"
              style={{
                backgroundColor: statusDot[opt.status],
                boxShadow: opt.status === "active" && isActive ? `0 0 6px ${statusDot[opt.status]}` : "none",
              }}
            />
            {opt.label}
            {opt.agentCount > 0 && (
              <span className="rounded-full bg-[var(--color-bg-elevated)] px-1.5 py-0.5 font-[family-name:var(--font-mono)] text-[10px]">
                {opt.agentCount}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
}
