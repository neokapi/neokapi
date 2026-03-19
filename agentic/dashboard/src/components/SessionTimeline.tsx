import { motion } from 'framer-motion';
import { useState, useMemo } from 'react';
import { useFilter } from '../context/FilterContext';
import { sessions } from '../data/sessions';
import { agents, accentColorMap } from '../data/agents';
import SessionDetail from './SessionDetail';

type TimeRange = 'day' | 'week' | '2weeks';

export default function SessionTimeline() {
  const { selectedWorkspace } = useFilter();
  const [timeRange, setTimeRange] = useState<TimeRange>('week');
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null);
  const [hoveredSessionId, setHoveredSessionId] = useState<string | null>(null);

  const filteredSessions = useMemo(() => {
    let s = sessions;
    if (selectedWorkspace) {
      s = s.filter((sess) => sess.workspace === selectedWorkspace);
    }
    const now = Date.now();
    const rangeMs =
      timeRange === 'day'
        ? 24 * 3_600_000
        : timeRange === 'week'
          ? 7 * 24 * 3_600_000
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
    timeRange === 'day'
      ? 24 * 3_600_000
      : timeRange === 'week'
        ? 7 * 24 * 3_600_000
        : 14 * 24 * 3_600_000;
  const rangeStart = now - rangeMs;

  const selectedSession = selectedSessionId
    ? sessions.find((s) => s.id === selectedSessionId) ?? null
    : null;

  // Generate time labels
  const timeLabels = useMemo(() => {
    const labels: { label: string; position: number }[] = [];
    if (timeRange === 'day') {
      for (let h = 0; h <= 24; h += 4) {
        const t = now - (24 - h) * 3_600_000;
        const d = new Date(t);
        labels.push({ label: `${d.getHours()}:00`, position: (h / 24) * 100 });
      }
    } else if (timeRange === 'week') {
      for (let d = 7; d >= 0; d--) {
        const t = now - d * 24 * 3_600_000;
        const date = new Date(t);
        labels.push({
          label: date.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' }),
          position: ((7 - d) / 7) * 100,
        });
      }
    } else {
      for (let d = 14; d >= 0; d -= 2) {
        const t = now - d * 24 * 3_600_000;
        const date = new Date(t);
        labels.push({
          label: date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' }),
          position: ((14 - d) / 14) * 100,
        });
      }
    }
    return labels;
  }, [timeRange, now]);

  // "Now" indicator position
  const nowPosition = 100; // always at the right edge

  // Mobile: vertical list of sessions
  const mobileSessionList = useMemo(() => {
    return [...filteredSessions].sort(
      (a, b) => new Date(b.startTime).getTime() - new Date(a.startTime).getTime()
    );
  }, [filteredSessions]);

  return (
    <motion.section
      className="px-4 py-16 sm:px-6"
      initial={{ opacity: 0, y: 30 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true }}
      transition={{ duration: 0.6 }}
    >
      <div className="mx-auto max-w-7xl">
        <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <h2
            className="font-display text-xl font-normal sm:text-2xl"
            style={{ color: 'rgb(var(--text-primary))' }}
          >
            Session Timeline
          </h2>
          <div
            className="flex gap-1 self-start rounded-lg p-1"
            style={{
              backgroundColor: 'rgb(var(--bg-card))',
              border: '1px solid rgb(var(--border))',
            }}
          >
            {(['day', 'week', '2weeks'] as const).map((r) => (
              <button
                key={r}
                onClick={() => setTimeRange(r)}
                className="min-h-[36px] rounded-md px-3 py-1 text-xs font-medium transition-colors sm:min-h-0"
                style={{
                  backgroundColor:
                    timeRange === r ? 'rgb(var(--accent))' : 'transparent',
                  color:
                    timeRange === r
                      ? 'rgb(var(--bg-base))'
                      : 'rgb(var(--text-secondary))',
                }}
              >
                {r === 'day' ? '24h' : r === 'week' ? '7d' : '14d'}
              </button>
            ))}
          </div>
        </div>

        {/* Desktop: Gantt-style timeline */}
        <div
          className="hidden overflow-hidden rounded-xl sm:block"
          style={{
            backgroundColor: 'rgb(var(--bg-card))',
            border: '1px solid rgb(var(--border))',
          }}
        >
          {/* Time axis header */}
          <div
            className="relative h-8"
            style={{
              marginLeft: '160px',
              borderBottom: '1px solid rgb(var(--border))',
              backgroundColor: 'rgb(var(--bg-elevated))',
            }}
          >
            {timeLabels.map((tl, i) => (
              <span
                key={i}
                className="absolute top-1/2 -translate-x-1/2 -translate-y-1/2 font-mono text-[10px]"
                style={{ left: `${tl.position}%`, color: 'rgb(var(--text-muted))' }}
              >
                {tl.label}
              </span>
            ))}
          </div>

          {/* Agent rows */}
          {visibleAgents.length === 0 ? (
            <div
              className="p-8 text-center text-sm"
              style={{ color: 'rgb(var(--text-muted))' }}
            >
              No sessions in this time range.
            </div>
          ) : (
            visibleAgents.map((agent) => {
              const color = accentColorMap[agent.accentColor] || '#d9a03c';
              const agentSessions = filteredSessions.filter(
                (s) => s.agentId === agent.id
              );

              return (
                <div
                  key={agent.id}
                  className="flex"
                  style={{
                    borderBottom: '1px solid rgb(var(--border))',
                  }}
                >
                  {/* Agent label with avatar and color dot */}
                  <div
                    className="flex w-[160px] flex-shrink-0 items-center gap-2 px-3 py-3"
                    style={{ borderRight: '1px solid rgb(var(--border))' }}
                  >
                    <span className="text-lg">{agent.avatar}</span>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-1.5">
                        <span
                          className="inline-block h-1.5 w-1.5 flex-shrink-0 rounded-full"
                          style={{ backgroundColor: color }}
                        />
                        <span
                          className="truncate text-xs font-medium"
                          style={{ color: 'rgb(var(--text-primary))' }}
                        >
                          {agent.name}
                        </span>
                      </div>
                      <div
                        className="truncate font-mono text-[10px]"
                        style={{ color: 'rgb(var(--text-muted))' }}
                      >
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
                        className="absolute top-0 h-full"
                        style={{
                          left: `${tl.position}%`,
                          width: '1px',
                          backgroundColor: 'rgb(var(--border))',
                          opacity: 0.2,
                        }}
                      />
                    ))}

                    {/* "Now" indicator */}
                    <div
                      className="absolute top-0 h-full"
                      style={{
                        left: `${nowPosition}%`,
                        width: '2px',
                        backgroundColor: 'rgb(var(--accent))',
                        opacity: 0.6,
                        zIndex: 5,
                      }}
                    />

                    {/* Session bars */}
                    {agentSessions.map((sess) => {
                      const startMs = new Date(sess.startTime).getTime();
                      const leftPct =
                        ((startMs - rangeStart) / rangeMs) * 100;
                      const widthPct = Math.max(
                        0.3,
                        ((sess.durationSecs * 1000) / rangeMs) * 100
                      );
                      const isFailed = sess.status === 'failed';
                      const isHovered = hoveredSessionId === sess.id;

                      return (
                        <motion.div
                          key={sess.id}
                          className="absolute top-1/2 -translate-y-1/2 cursor-pointer"
                          style={{
                            left: `${Math.max(0, Math.min(leftPct, 99.5))}%`,
                            width: `${widthPct}%`,
                            height: isHovered ? '32px' : '28px',
                            borderRadius: '14px',
                            background: isFailed
                              ? 'transparent'
                              : `linear-gradient(90deg, ${color}cc, ${color}ee)`,
                            opacity: isFailed ? 0.5 : 0.9,
                            border: isFailed
                              ? `1px dashed ${color}`
                              : 'none',
                            zIndex: isHovered ? 10 : 1,
                            transition: 'height 0.15s ease, z-index 0s',
                          }}
                          onClick={() => setSelectedSessionId(sess.id)}
                          onMouseEnter={() => setHoveredSessionId(sess.id)}
                          onMouseLeave={() => setHoveredSessionId(null)}
                        >
                          {/* Tooltip */}
                          {isHovered && (
                            <div
                              className="pointer-events-none absolute bottom-full left-1/2 z-20 mb-2 w-64 -translate-x-1/2 rounded-lg p-2.5 shadow-lg"
                              style={{
                                backgroundColor: 'rgb(var(--bg-base))',
                                border: '1px solid rgb(var(--border))',
                              }}
                            >
                              <div
                                className="text-xs font-medium"
                                style={{ color: 'rgb(var(--text-primary))' }}
                              >
                                {sess.agentName}
                              </div>
                              <div
                                className="mt-0.5 text-[10px]"
                                style={{ color: 'rgb(var(--text-muted))' }}
                              >
                                {sess.summary}
                              </div>
                              <div
                                className="mt-1 font-mono text-[10px]"
                                style={{ color: 'rgb(var(--text-muted))' }}
                              >
                                {Math.floor(sess.durationSecs / 60)}m{' '}
                                {sess.durationSecs % 60}s
                              </div>
                            </div>
                          )}
                        </motion.div>
                      );
                    })}
                  </div>
                </div>
              );
            })
          )}
        </div>

        {/* Mobile: vertical session list */}
        <div className="space-y-2 sm:hidden">
          {mobileSessionList.length === 0 ? (
            <div
              className="rounded-xl p-8 text-center text-sm"
              style={{
                backgroundColor: 'rgb(var(--bg-card))',
                border: '1px solid rgb(var(--border))',
                color: 'rgb(var(--text-muted))',
              }}
            >
              No sessions in this time range.
            </div>
          ) : (
            mobileSessionList.map((sess) => {
              const agent = agents.find((a) => a.id === sess.agentId);
              const color = agent
                ? accentColorMap[agent.accentColor] || '#d9a03c'
                : '#d9a03c';
              return (
                <div
                  key={sess.id}
                  className="cursor-pointer rounded-lg p-3"
                  style={{
                    backgroundColor: 'rgb(var(--bg-card))',
                    borderLeft: `3px solid ${color}`,
                    border: '1px solid rgb(var(--border))',
                    borderLeftWidth: '3px',
                    borderLeftColor: color,
                  }}
                  onClick={() => setSelectedSessionId(sess.id)}
                >
                  <div className="flex items-center gap-2">
                    <span className="text-lg">{agent?.avatar}</span>
                    <span
                      className="text-sm font-medium"
                      style={{ color: 'rgb(var(--text-primary))' }}
                    >
                      {sess.agentName}
                    </span>
                    <span
                      className="ml-auto font-mono text-[10px]"
                      style={{ color: 'rgb(var(--text-muted))' }}
                    >
                      {Math.floor(sess.durationSecs / 60)}m
                    </span>
                  </div>
                  <p
                    className="mt-1 text-xs leading-relaxed"
                    style={{ color: 'rgb(var(--text-secondary))' }}
                  >
                    {sess.summary}
                  </p>
                  <div
                    className="mt-1 font-mono text-[10px]"
                    style={{ color: 'rgb(var(--text-muted))' }}
                  >
                    {new Date(sess.startTime).toLocaleDateString('en-US', {
                      weekday: 'short',
                      month: 'short',
                      day: 'numeric',
                      hour: '2-digit',
                      minute: '2-digit',
                    })}
                  </div>
                </div>
              );
            })
          )}
        </div>

        {/* Legend (desktop only) */}
        <div
          className="mt-3 hidden items-center gap-4 text-[10px] sm:flex"
          style={{ color: 'rgb(var(--text-muted))' }}
        >
          <span className="flex items-center gap-1.5">
            <span
              className="inline-block h-3 w-6"
              style={{
                borderRadius: '6px',
                backgroundColor: 'rgb(var(--accent))',
                opacity: 0.9,
              }}
            />{' '}
            Succeeded
          </span>
          <span className="flex items-center gap-1.5">
            <span
              className="inline-block h-3 w-6"
              style={{
                borderRadius: '6px',
                border: '1px dashed rgb(var(--accent))',
                opacity: 0.5,
              }}
            />{' '}
            Failed
          </span>
          <span
            className="flex items-center gap-1.5"
          >
            <span
              className="inline-block h-3"
              style={{
                width: '2px',
                backgroundColor: 'rgb(var(--accent))',
                opacity: 0.6,
              }}
            />{' '}
            Now
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
