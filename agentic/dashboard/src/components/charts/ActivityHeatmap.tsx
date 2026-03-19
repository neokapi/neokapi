import { motion } from 'framer-motion';
import { activityHeatmap } from '../../data/metrics';

const dayLabels = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

function getColor(count: number): string {
  if (count === 0) return '#1c1c28';
  if (count <= 3) return '#78500a';
  if (count <= 6) return '#92610d';
  if (count <= 9) return '#b7791f';
  if (count <= 12) return '#d69e2e';
  if (count <= 15) return '#ecc94b';
  return '#fbbf24';
}

export default function ActivityHeatmap() {
  const maxWeek = 12;

  return (
    <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-bg-card)] p-5">
      <h3 className="mb-4 font-[family-name:var(--font-mono)] text-sm font-semibold text-[var(--color-text-primary)]">
        Activity Heatmap
      </h3>
      <div className="flex gap-1">
        {/* Day labels */}
        <div className="flex flex-col gap-1 pr-2">
          {dayLabels.map((d) => (
            <div
              key={d}
              className="flex h-4 items-center text-[10px] text-[var(--color-text-muted)]"
            >
              {d}
            </div>
          ))}
        </div>
        {/* Grid */}
        <div className="flex gap-1">
          {Array.from({ length: maxWeek }).map((_, w) => (
            <div key={w} className="flex flex-col gap-1">
              {Array.from({ length: 7 }).map((_, d) => {
                const cell = activityHeatmap.find((c) => c.week === w && c.day === d);
                const count = cell?.count || 0;
                return (
                  <motion.div
                    key={d}
                    className="h-4 w-4 rounded-sm"
                    style={{ backgroundColor: getColor(count) }}
                    initial={{ opacity: 0 }}
                    whileInView={{ opacity: 1 }}
                    viewport={{ once: true }}
                    transition={{ duration: 0.3, delay: (w * 7 + d) * 0.005 }}
                    title={`W${w + 1} ${dayLabels[d]}: ${count} actions`}
                  />
                );
              })}
            </div>
          ))}
        </div>
      </div>
      {/* Legend */}
      <div className="mt-3 flex items-center gap-2">
        <span className="text-[10px] text-[var(--color-text-muted)]">Less</span>
        {[0, 3, 6, 9, 12, 16].map((v) => (
          <div
            key={v}
            className="h-3 w-3 rounded-sm"
            style={{ backgroundColor: getColor(v) }}
          />
        ))}
        <span className="text-[10px] text-[var(--color-text-muted)]">More</span>
      </div>
    </div>
  );
}
