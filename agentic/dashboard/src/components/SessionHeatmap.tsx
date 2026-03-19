import { motion } from 'framer-motion';
import { useMemo } from 'react';
import { useFilter } from '../context/FilterContext';
import { sessions } from '../data/sessions';

const dayLabels = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

function getHeatColor(count: number, maxCount: number): string {
  if (count === 0) return "var(--color-bg-elevated)";
  const intensity = count / maxCount;
  if (intensity < 0.25) return "#78350f"; // dark amber
  if (intensity < 0.5) return "#b45309";  // amber
  if (intensity < 0.75) return "#d97706"; // warm amber
  return "#f59e0b"; // gold
}

export default function SessionHeatmap() {
  const { selectedWorkspace } = useFilter();

  const { grid, weeks, maxCount } = useMemo(() => {
    const filtered = selectedWorkspace
      ? sessions.filter((s) => s.workspace === selectedWorkspace)
      : sessions;

    // Build a grid of weeks x days
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

    // Build grid for past numWeeks weeks
    const weekLabels: string[] = [];
    for (let w = numWeeks - 1; w >= 0; w--) {
      const weekStart = new Date(now);
      weekStart.setDate(weekStart.getDate() - weekStart.getDay() + 1 - w * 7); // Monday of that week
      weekLabels.push(weekStart.toLocaleDateString("en-US", { month: "short", day: "numeric" }));

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
      className="rounded-xl border border-[var(--color-border)] bg-[var(--color-bg-card)] p-5"
      initial={{ opacity: 0, y: 20 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true }}
      transition={{ duration: 0.5 }}
    >
      <h3 className="mb-4 font-[family-name:var(--font-mono)] text-sm font-semibold text-[var(--color-text-primary)]">
        Session Activity
      </h3>

      <div className="flex gap-1">
        {/* Day labels */}
        <div className="flex flex-col gap-1 pr-2 pt-6">
          {dayLabels.map((label) => (
            <div key={label} className="flex h-5 items-center font-[family-name:var(--font-mono)] text-[10px] text-[var(--color-text-muted)]">
              {label}
            </div>
          ))}
        </div>

        {/* Grid */}
        <div className="flex-1">
          {/* Week labels */}
          <div className="mb-1 flex gap-1">
            {weeks.map((w, i) => (
              <div key={i} className="flex-1 text-center font-[family-name:var(--font-mono)] text-[10px] text-[var(--color-text-muted)]">
                {w}
              </div>
            ))}
          </div>

          {/* Cells: rows = days (0-6), columns = weeks */}
          {dayLabels.map((_, dayIdx) => (
            <div key={dayIdx} className="flex gap-1 mb-1">
              {Array.from({ length: weeks.length }).map((_, weekIdx) => {
                const cell = grid.find((c) => c.week === weekIdx && c.day === dayIdx);
                const count = cell?.count ?? 0;
                return (
                  <div
                    key={weekIdx}
                    className="flex-1 rounded-sm transition-colors"
                    style={{
                      height: "20px",
                      backgroundColor: getHeatColor(count, maxCount),
                      opacity: count === 0 ? 0.3 : 1,
                    }}
                    title={cell ? `${cell.date}: ${count} session${count !== 1 ? "s" : ""}` : ""}
                  />
                );
              })}
            </div>
          ))}
        </div>
      </div>

      {/* Legend */}
      <div className="mt-3 flex items-center justify-end gap-1.5">
        <span className="text-[10px] text-[var(--color-text-muted)]">Less</span>
        {[0, 0.25, 0.5, 0.75, 1].map((intensity, i) => (
          <div
            key={i}
            className="h-3 w-3 rounded-sm"
            style={{
              backgroundColor: intensity === 0 ? "var(--color-bg-elevated)" :
                intensity < 0.25 ? "#78350f" :
                intensity < 0.5 ? "#b45309" :
                intensity < 0.75 ? "#d97706" : "#f59e0b",
              opacity: intensity === 0 ? 0.3 : 1,
            }}
          />
        ))}
        <span className="text-[10px] text-[var(--color-text-muted)]">More</span>
      </div>
    </motion.div>
  );
}
