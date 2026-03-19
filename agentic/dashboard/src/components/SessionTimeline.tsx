import { motion } from 'framer-motion';
import { useState, useMemo } from 'react';
import { useFilter } from '../context/FilterContext';
import { sessions } from '../data/sessions';
import { agents, accentColorMap } from '../data/agents';
import SessionDetail from './SessionDetail';

type TimeRange = "day" | "week" | "2weeks";

export default function SessionTimeline() {
  const { selectedWorkspace } = useFilter();
  const [timeRange, setTimeRange] = useState<TimeRange>("week");
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null);

  const filteredSessions = useMemo(() => {
    let s = sessions;
    if (selectedWorkspace) {
      s = s.filter((sess) => sess.workspace === selectedWorkspace);
    }
    const now = Date.now();
    const rangeMs =
      timeRange === "day" ? 24 * 3_600_000
      : timeRange === "week" ? 7 * 24 * 3_600_000
      : 14 * 24 * 3_600_000;
    s = s.filter((sess) => now - new Date(sess.startTime).getTime() < rangeMs);
    return s;
  }, [selectedWorkspace, timeRange]);

  const visibleAgents = useMemo(() => {
    const ids = new Set(filteredSessions.map((s) => s.agentId));
    return agents.filter((a) => ids.has(a.id));
  }, [filteredSessions]);

  const now = Date.now();
  const rangeMs =
    timeRange === "day" ? 24 * 3_600_000
    : timeRange === "week" ? 7 * 24 * 3_600_000
    : 14 * 24 * 3_600_000;
  const rangeStart = now - rangeMs;

  const selectedSession = selectedSessionId
    ? sessions.find((s) => s.id === selectedSessionId) ?? null
    : null;

  // Generate time labels
  const timeLabels = useMemo(() => {
    const labels: { label: string; position: number }[] = [];
    if (timeRange === "day") {
      for (let h = 0; h <= 24; h += 4) {
        const t = now - (24 - h) * 3_600_000;
        const d = new Date(t);
        labels.push({ label: `${d.getHours()}:00`, position: (h / 24) * 100 });
      }
    } else if (timeRange === "week") {
      for (let d = 7; d >= 0; d--) {
        const t = now - d * 24 * 3_600_000;
        const date = new Date(t);
        labels.push({
          label: date.toLocaleDateString("en-US", { weekday: "short", month: "short", day: "numeric" }),
          position: ((7 - d) / 7) * 100,
        });
      }
    } else {
      for (let d = 14; d >= 0; d -= 2) {
        const t = now - d * 24 * 3_600_000;
        const date = new Date(t);
        labels.push({
          label: date.toLocaleDateString("en-US", { month: "short", day: "numeric" }),
          position: ((14 - d) / 14) * 100,
        });
      }
    }
    return labels;
  }, [timeRange, now]);

  return (
    <motion.section
      className="px-6 py-8"
      initial={{ opacity: 0, y: 30 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true }}
      transition={{ duration: 0.6 }}
    >
      <div className="mx-auto max-w-7xl">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="font-[family-name:var(--font-display)] text-2xl font-bold text-[var(--color-text-primary)]">
            Session Timeline
          </h2>
          <div className="flex gap-1 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg-card)] p-1">
            {(["day", "week", "2weeks"] as const).map((r) => (
              <button
                key={r}
                onClick={() => setTimeRange(r)}
                className={`rounded-md px-3 py-1 text-xs font-medium transition-colors ${
                  timeRange === r
                    ? "bg-[var(--color-accent-amber)] text-[var(--color-bg-primary)]"
                    : "text-[var(--color-text-secondary)] hover:text-[var(--color-text-primary)]"
                }`}
              >
                {r === "day" ? "24h" : r === "week" ? "7d" : "14d"}
              </button>
            ))}
          </div>
        </div>

        <div className="overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-bg-card)]">
          {/* Time axis header */}
          <div className="relative h-8 border-b border-[var(--color-border)] bg-[var(--color-bg-elevated)]" style={{ marginLeft: "140px" }}>
            {timeLabels.map((tl, i) => (
              <span
                key={i}
                className="absolute top-1/2 -translate-x-1/2 -translate-y-1/2 font-[family-name:var(--font-mono)] text-[10px] text-[var(--color-text-muted)]"
                style={{ left: `${tl.position}%` }}
              >
                {tl.label}
              </span>
            ))}
          </div>

          {/* Agent rows */}
          {visibleAgents.length === 0 ? (
            <div className="p-8 text-center text-sm text-[var(--color-text-muted)]">
              No sessions in this time range.
            </div>
          ) : (
            visibleAgents.map((agent) => {
              const color = accentColorMap[agent.accentColor] || "#f59e0b";
              const agentSessions = filteredSessions.filter((s) => s.agentId === agent.id);

              return (
                <div key={agent.id} className="flex border-b border-[var(--color-border)] last:border-b-0">
                  {/* Agent label */}
                  <div className="flex w-[140px] flex-shrink-0 items-center gap-2 border-r border-[var(--color-border)] px-3 py-3">
                    <span className="text-lg">{agent.avatar}</span>
                    <div className="min-w-0">
                      <div className="truncate text-xs font-medium text-[var(--color-text-primary)]">
                        {agent.name}
                      </div>
                      <div className="truncate font-[family-name:var(--font-mono)] text-[10px] text-[var(--color-text-muted)]">
                        {agent.role}
                      </div>
                    </div>
                  </div>

                  {/* Timeline area */}
                  <div className="relative h-16 flex-1">
                    {/* Vertical grid lines */}
                    {timeLabels.map((tl, i) => (
                      <div
                        key={i}
                        className="absolute top-0 h-full w-px bg-[var(--color-border)]"
                        style={{ left: `${tl.position}%`, opacity: 0.3 }}
                      />
                    ))}

                    {/* Session bars */}
                    {agentSessions.map((sess) => {
                      const startMs = new Date(sess.startTime).getTime();
                      const leftPct = ((startMs - rangeStart) / rangeMs) * 100;
                      // Min width 0.3% for visibility
                      const widthPct = Math.max(0.3, (sess.durationSecs * 1000 / rangeMs) * 100);
                      const isFailed = sess.status === "failed";

                      return (
                        <motion.div
                          key={sess.id}
                          className="absolute top-1/2 -translate-y-1/2 cursor-pointer rounded-sm"
                          style={{
                            left: `${Math.max(0, Math.min(leftPct, 99.5))}%`,
                            width: `${widthPct}%`,
                            height: "28px",
                            backgroundColor: color,
                            opacity: isFailed ? 0.4 : 0.85,
                            border: isFailed ? `1px dashed ${color}` : "none",
                          }}
                          whileHover={{ scale: 1.1, zIndex: 10 }}
                          onClick={() => setSelectedSessionId(sess.id)}
                          title={`${sess.agentName} - ${sess.summary}`}
                        >
                          {/* Tooltip on hover */}
                          <div className="pointer-events-none absolute bottom-full left-1/2 z-20 mb-2 hidden w-64 -translate-x-1/2 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg-primary)] p-2 shadow-lg group-hover:block">
                            <div className="text-xs font-medium text-[var(--color-text-primary)]">{sess.agentName}</div>
                            <div className="text-[10px] text-[var(--color-text-muted)]">{sess.summary}</div>
                          </div>
                        </motion.div>
                      );
                    })}
                  </div>
                </div>
              );
            })
          )}
        </div>

        {/* Legend */}
        <div className="mt-3 flex items-center gap-4 text-[10px] text-[var(--color-text-muted)]">
          <span className="flex items-center gap-1.5">
            <span className="inline-block h-3 w-6 rounded-sm bg-[var(--color-accent-amber)] opacity-85" /> Succeeded
          </span>
          <span className="flex items-center gap-1.5">
            <span className="inline-block h-3 w-6 rounded-sm border border-dashed border-[var(--color-accent-amber)] bg-[var(--color-accent-amber)] opacity-40" /> Failed
          </span>
          <span className="ml-auto">Click a session bar for details</span>
        </div>
      </div>

      {/* Session detail panel */}
      {selectedSession && (
        <SessionDetail
          session={selectedSession}
          onClose={() => setSelectedSessionId(null)}
        />
      )}
    </motion.section>
  );
}
