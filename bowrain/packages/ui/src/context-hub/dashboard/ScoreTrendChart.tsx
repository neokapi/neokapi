// A compact brand-compliance trend (AD-021). Renders the per-day average score
// over time as a filled area, on a fixed 0–100 axis so the slope is honest.
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@neokapi/ui-primitives";
import type { ScoreTrend } from "../../brand/types";

const config: ChartConfig = {
  score: { label: "Avg score", color: "var(--chart-2, oklch(0.6 0.118 184.704))" },
};

function shortDate(value: string): string {
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleDateString(undefined, { day: "numeric", month: "short" });
}

export function ScoreTrendChart({
  trends,
  height = 176,
}: {
  trends: ScoreTrend[];
  height?: number;
}) {
  const data = trends.map((t) => ({
    date: t.date,
    score: Math.round(t.avg_score),
    count: t.count,
  }));

  return (
    <div style={{ height }}>
      <ChartContainer config={config} className="h-full w-full">
        <AreaChart data={data} margin={{ left: 0, right: 8, top: 6, bottom: 0 }}>
          <defs>
            <linearGradient id="brandScoreFill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="var(--color-score)" stopOpacity={0.25} />
              <stop offset="95%" stopColor="var(--color-score)" stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid vertical={false} strokeDasharray="3 3" className="stroke-border/60" />
          <XAxis
            dataKey="date"
            tickLine={false}
            axisLine={false}
            tick={{ fontSize: 11 }}
            minTickGap={28}
            tickFormatter={shortDate}
          />
          <YAxis
            domain={[0, 100]}
            width={28}
            tickLine={false}
            axisLine={false}
            tick={{ fontSize: 11 }}
            ticks={[0, 50, 100]}
          />
          <ChartTooltip
            content={<ChartTooltipContent labelFormatter={(label) => shortDate(String(label))} />}
          />
          <Area
            type="monotone"
            dataKey="score"
            stroke="var(--color-score)"
            fill="url(#brandScoreFill)"
            strokeWidth={2}
            dot={false}
          />
        </AreaChart>
      </ChartContainer>
    </div>
  );
}
