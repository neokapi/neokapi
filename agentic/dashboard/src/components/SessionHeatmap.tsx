import { motion } from 'framer-motion';
import { useMemo, useState } from 'react';
import { useFilter } from '../context/FilterContext';
import { sessions } from '../data/sessions';

const dayLabels = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

function getHeatColor(count: number, maxCount: number): string {
  if (count === 0) return 'rgb(var(--bg-elevated))';
  const intensity = count / maxCount;
  // Warm color ramp: brown -> amber -> gold
  if (intensity < 0.25) return 'rgb(120 80 30)';
  if (intensity < 0.5) return 'rgb(160 110 30)';
  if (intensity < 0.75) return 'rgb(200 145 40)';
  return 'rgb(var(--accent))';
}

export default function SessionHeatmap() {
  const { selectedWorkspace } = useFilter();
  const [hoveredCell, setHoveredCell] = useState<{ date: string; count: number; x: number; y: number } | null>(null);

  const { grid, weeks, maxCount } = useMemo(() => {
    const filtered = selectedWorkspace
      ? sessions.filter((s) => s.workspace === selectedWorkspace)
      : sessions;

    const now = new Date();
    const numWeeks = 4;
    const cellMap = new Map<string, number>();

    for (const sess of filtered) {
      const d = new Date(sess.startTime);
      const dateStr = d.toISOString().slice(0, 10);
      cellMap.set(dateStr, (cellMap.get(dateStr) ?? 0) + 1);
    }

    const grid: { week: number; day: number; count: number; date: string }[] = [];
    let maxCount = 0;

    const weekLabels: string[] = [];
    for (let w = numWeeks - 1; w >= 0; w--) {
      const weekStart = new Date(now);
      weekStart.setDate(weekStart.getDate() - weekStart.getDay() + 1 - w * 7);
      weekLabels.push(
        weekStart.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
      );

      for (let d = 0; d < 7; d++) {
        const cellDate = new Date(weekStart);
        cellDate.setDate(cellDate.getDate() + d);
        const dateStr = cellDate.toISOString().slice(0, 10);
        const count = cellMap.get(dateStr) ?? 0;
        if (count > maxCount) maxCount = count;
        grid.push({ week: numWeeks - 1 - w, day: d, count, date: dateStr });
      }
    }

    return { grid, weeks: weekLabels, maxCount: Math.max(maxCount, 1) };
  }, [selectedWorkspace]);

  return (
    <motion.div
      className="rounded-xl p-5"
      style={{
        backgroundColor: 'rgb(var(--bg-card))',
        border: '1px solid rgb(var(--border))',
      }}
      initial={{ opacity: 0, y: 20 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true }}
      transition={{ duration: 0.5 }}
    >
      <h3
        className="mb-4 font-mono text-sm font-semibold"
        style={{ color: 'rgb(var(--text-primary))' }}
      >
        Session Activity
      </h3>

      <div className="relative flex gap-1">
        {/* Day labels */}
        <div className="flex flex-col gap-1 pr-2 pt-6">
          {dayLabels.map((label) => (
            <div
              key={label}
              className="flex items-center font-mono text-[10px]"
              style={{ height: '20px', color: 'rgb(var(--text-muted))' }}
            >
              {label}
            </div>
          ))}
        </div>

        {/* Grid */}
        <div className="flex-1">
          {/* Week labels */}
          <div className="mb-1 flex gap-1">
            {weeks.map((w, i) => (
              <div
                key={i}
                className="flex-1 text-center font-mono text-[10px]"
                style={{ color: 'rgb(var(--text-muted))' }}
              >
                {w}
              </div>
            ))}
          </div>

          {/* Cells */}
          {dayLabels.map((_, dayIdx) => (
            <div key={dayIdx} className="mb-1 flex gap-1">
              {Array.from({ length: weeks.length }).map((_, weekIdx) => {
                const cell = grid.find(
                  (c) => c.week === weekIdx && c.day === dayIdx
                );
                const count = cell?.count ?? 0;
                return (
                  <div
                    key={weekIdx}
                    className="flex-1 cursor-default rounded-md transition-colors"
                    style={{
                      height: '20px',
                      backgroundColor: getHeatColor(count, maxCount),
                      opacity: count === 0 ? 0.3 : 1,
                    }}
                    onMouseEnter={(e) => {
                      if (cell) {
                        const rect = e.currentTarget.getBoundingClientRect();
                        setHoveredCell({
                          date: cell.date,
                          count,
                          x: rect.left + rect.width / 2,
                          y: rect.top,
                        });
                      }
                    }}
                    onMouseLeave={() => setHoveredCell(null)}
                  />
                );
              })}
            </div>
          ))}
        </div>

        {/* Tooltip */}
        {hoveredCell && (
          <div
            className="pointer-events-none fixed z-50 rounded-lg px-2.5 py-1.5 shadow-lg"
            style={{
              left: hoveredCell.x,
              top: hoveredCell.y - 40,
              transform: 'translateX(-50%)',
              backgroundColor: 'rgb(var(--bg-base))',
              border: '1px solid rgb(var(--border))',
            }}
          >
            <div
              className="font-mono text-[10px]"
              style={{ color: 'rgb(var(--text-primary))' }}
            >
              {hoveredCell.date}: {hoveredCell.count} session
              {hoveredCell.count !== 1 ? 's' : ''}
            </div>
          </div>
        )}
      </div>

      {/* Legend */}
      <div className="mt-3 flex items-center justify-end gap-1.5">
        <span className="text-[10px]" style={{ color: 'rgb(var(--text-muted))' }}>
          Less
        </span>
        {[0, 0.25, 0.5, 0.75, 1].map((intensity, i) => (
          <div
            key={i}
            className="h-3 w-3 rounded-md"
            style={{
              backgroundColor:
                intensity === 0
                  ? 'rgb(var(--bg-elevated))'
                  : intensity < 0.3
                    ? 'rgb(120 80 30)'
                    : intensity < 0.55
                      ? 'rgb(160 110 30)'
                      : intensity < 0.8
                        ? 'rgb(200 145 40)'
                        : 'rgb(var(--accent))',
              opacity: intensity === 0 ? 0.3 : 1,
            }}
          />
        ))}
        <span className="text-[10px]" style={{ color: 'rgb(var(--text-muted))' }}>
          More
        </span>
      </div>
    </motion.div>
  );
}
