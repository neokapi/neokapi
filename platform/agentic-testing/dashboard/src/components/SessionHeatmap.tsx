import { useMemo, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useApi } from "@/context/ApiContext";

const dayLabels = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

function getHeatClasses(count: number, maxCount: number): string {
  if (count === 0) return "bg-muted opacity-30";
  const intensity = count / maxCount;
  if (intensity < 0.25) return "bg-chart-5/40";
  if (intensity < 0.5) return "bg-chart-4/60";
  if (intensity < 0.75) return "bg-chart-1/70";
  return "bg-chart-1";
}

export default function SessionHeatmap() {
  const api = useApi();
  const [hoveredCell, setHoveredCell] = useState<{
    date: string;
    count: number;
    x: number;
    y: number;
  } | null>(null);

  const { grid, weeks, maxCount } = useMemo(() => {
    const now = new Date();
    const numWeeks = 4;
    const cellMap = new Map<string, number>();

    // Count audit log events per day
    for (const entry of api.auditLog) {
      const d = new Date(entry.created_at);
      const dateStr = d.toISOString().slice(0, 10);
      cellMap.set(dateStr, (cellMap.get(dateStr) ?? 0) + 1);
    }

    const grid: { week: number; day: number; count: number; date: string }[] = [];
    let maxCount = 0;

    const weekLabels: string[] = [];
    for (let w = numWeeks - 1; w >= 0; w--) {
      const weekStart = new Date(now);
      weekStart.setDate(weekStart.getDate() - weekStart.getDay() + 1 - w * 7);
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
  }, [api.auditLog]);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">
          Event Activity
          {api.connected && (
            <span className="ml-2 text-xs font-normal text-muted-foreground">(from audit log)</span>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="relative flex gap-1">
          {/* Day labels */}
          <div className="flex flex-col gap-1 pr-2 pt-6">
            {dayLabels.map((label) => (
              <div
                key={label}
                className="flex h-5 items-center font-mono text-[10px] text-muted-foreground"
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
                  className="flex-1 text-center font-mono text-[10px] text-muted-foreground"
                >
                  {w}
                </div>
              ))}
            </div>

            {/* Cells */}
            {dayLabels.map((_, dayIdx) => (
              <div key={dayIdx} className="mb-1 flex gap-1">
                {Array.from({ length: weeks.length }).map((_, weekIdx) => {
                  const cell = grid.find((c) => c.week === weekIdx && c.day === dayIdx);
                  const count = cell?.count ?? 0;
                  return (
                    <div
                      key={weekIdx}
                      className={`h-5 flex-1 cursor-default rounded-md ${getHeatClasses(count, maxCount)}`}
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
              className="pointer-events-none fixed z-50 rounded-lg border bg-background px-2.5 py-1.5 shadow-lg"
              style={{
                left: hoveredCell.x,
                top: hoveredCell.y - 40,
                transform: "translateX(-50%)",
              }}
            >
              <div className="font-mono text-[10px]">
                {hoveredCell.date}: {hoveredCell.count} event
                {hoveredCell.count !== 1 ? "s" : ""}
              </div>
            </div>
          )}
        </div>

        {/* Legend */}
        <div className="mt-3 flex items-center justify-end gap-1.5">
          <span className="text-[10px] text-muted-foreground">Less</span>
          <div className="h-3 w-3 rounded-md bg-muted opacity-30" />
          <div className="h-3 w-3 rounded-md bg-chart-5/40" />
          <div className="h-3 w-3 rounded-md bg-chart-4/60" />
          <div className="h-3 w-3 rounded-md bg-chart-1/70" />
          <div className="h-3 w-3 rounded-md bg-chart-1" />
          <span className="text-[10px] text-muted-foreground">More</span>
        </div>
      </CardContent>
    </Card>
  );
}
