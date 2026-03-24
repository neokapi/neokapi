interface HeatmapDay {
  date: string;
  count: number;
}

interface ActivityHeatmapProps {
  days: HeatmapDay[];
  weeks?: number;
}

const CELL = 12;
const GAP = 2;
const STEP = CELL + GAP;
const DAYS_IN_WEEK = 7;
const MONTH_LABELS = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "May",
  "Jun",
  "Jul",
  "Aug",
  "Sep",
  "Oct",
  "Nov",
  "Dec",
];
const DAY_LABELS = ["Mon", "Wed", "Fri"];
const DAY_LABEL_INDICES = [1, 3, 5]; // Mon=1, Wed=3, Fri=5

function intensityLevel(count: number, max: number): number {
  if (count === 0 || max === 0) return 0;
  const ratio = count / max;
  if (ratio <= 0.25) return 1;
  if (ratio <= 0.5) return 2;
  if (ratio <= 0.75) return 3;
  return 4;
}

const LIGHT_COLORS = ["var(--color-muted, #ebedf0)", "#9be9a8", "#40c463", "#30a14e", "#216e39"];

const DARK_COLORS = ["var(--color-muted, #161b22)", "#0e4429", "#006d32", "#26a641", "#39d353"];

function useIsDark(): boolean {
  if (typeof document === "undefined") return false;
  return document.documentElement.classList.contains("dark");
}

export function ActivityHeatmap({ days, weeks = 52 }: ActivityHeatmapProps) {
  const isDark = useIsDark();
  const colors = isDark ? DARK_COLORS : LIGHT_COLORS;

  // Build lookup map from date string to count.
  const countMap = new Map<string, number>();
  for (const d of days) {
    countMap.set(d.date, d.count);
  }

  // Build the grid: weeks × 7 days, ending today.
  const today = new Date();
  const totalDays = weeks * DAYS_IN_WEEK;
  const startDate = new Date(today);
  startDate.setDate(startDate.getDate() - totalDays + 1);
  // Align to the nearest preceding Sunday (start of week).
  const dayOfWeek = startDate.getDay();
  startDate.setDate(startDate.getDate() - dayOfWeek);

  const grid: { date: string; count: number; weekIdx: number; dayIdx: number }[] = [];
  const current = new Date(startDate);
  let weekIdx = 0;
  let dayIdx = 0;

  while (current <= today) {
    const dateStr = current.toISOString().slice(0, 10);
    grid.push({
      date: dateStr,
      count: countMap.get(dateStr) ?? 0,
      weekIdx,
      dayIdx,
    });
    current.setDate(current.getDate() + 1);
    dayIdx++;
    if (dayIdx >= DAYS_IN_WEEK) {
      dayIdx = 0;
      weekIdx++;
    }
  }

  const maxCount = Math.max(1, ...grid.map((c) => c.count));
  const totalWeeks = weekIdx + (dayIdx > 0 ? 1 : 0);
  const totalActivities = grid.reduce((sum, c) => sum + c.count, 0);

  // Compute month label positions.
  const monthLabels: { label: string; x: number }[] = [];
  let lastMonth = -1;
  for (const cell of grid) {
    const month = new Date(cell.date).getMonth();
    if (month !== lastMonth && cell.dayIdx === 0) {
      monthLabels.push({
        label: MONTH_LABELS[month],
        x: cell.weekIdx,
      });
      lastMonth = month;
    }
  }

  const LEFT_PAD = 30;
  const TOP_PAD = 18;
  const svgWidth = LEFT_PAD + totalWeeks * STEP;
  const svgHeight = TOP_PAD + DAYS_IN_WEEK * STEP + 24;

  return (
    <div className="space-y-2">
      <svg
        width="100%"
        viewBox={`0 0 ${svgWidth} ${svgHeight}`}
        className="overflow-visible"
        role="img"
        aria-label={`Activity heatmap showing ${totalActivities} activities in the last year`}
      >
        {/* Month labels */}
        {monthLabels.map((m) => (
          <text
            key={`${m.label}-${m.x}`}
            x={LEFT_PAD + m.x * STEP}
            y={TOP_PAD - 6}
            className="fill-muted-foreground"
            fontSize="9"
          >
            {m.label}
          </text>
        ))}

        {/* Day labels */}
        {DAY_LABELS.map((label, i) => (
          <text
            key={label}
            x={LEFT_PAD - 6}
            y={TOP_PAD + DAY_LABEL_INDICES[i] * STEP + CELL - 2}
            textAnchor="end"
            className="fill-muted-foreground"
            fontSize="9"
          >
            {label}
          </text>
        ))}

        {/* Cells */}
        {grid.map((cell) => {
          const level = intensityLevel(cell.count, maxCount);
          return (
            <rect
              key={cell.date}
              x={LEFT_PAD + cell.weekIdx * STEP}
              y={TOP_PAD + cell.dayIdx * STEP}
              width={CELL}
              height={CELL}
              rx={2}
              fill={colors[level]}
            >
              <title>
                {cell.date}: {cell.count} {cell.count === 1 ? "activity" : "activities"}
              </title>
            </rect>
          );
        })}

        {/* Legend */}
        <text
          x={svgWidth - 5 * STEP - 30}
          y={svgHeight - 4}
          className="fill-muted-foreground"
          fontSize="9"
        >
          Less
        </text>
        {colors.map((color, i) => (
          <rect
            key={i}
            x={svgWidth - (5 - i) * STEP - 2}
            y={svgHeight - 14}
            width={CELL}
            height={CELL}
            rx={2}
            fill={color}
          />
        ))}
        <text x={svgWidth} y={svgHeight - 4} className="fill-muted-foreground" fontSize="9">
          More
        </text>
      </svg>
      <div className="text-xs text-muted-foreground">
        {totalActivities.toLocaleString()} activities in the last year
      </div>
    </div>
  );
}
