import { Line, LineChart, YAxis } from "recharts";

/**
 * Tiny 12-week trend sparkline for table rows. Fixed-size (no
 * ResponsiveContainer) so it renders identically in tables, Storybook and
 * jsdom.
 */
export function TrendSparkline({
  data,
  width = 96,
  height = 28,
}: {
  /** Weekly values, oldest first. */
  data: number[];
  width?: number;
  height?: number;
}) {
  const points = data.map((value, week) => ({ week, value }));
  return (
    <LineChart
      width={width}
      height={height}
      data={points}
      margin={{ top: 4, right: 2, bottom: 2, left: 2 }}
      accessibilityLayer={false}
    >
      <YAxis hide domain={["dataMin", "dataMax"]} />
      <Line
        type="monotone"
        dataKey="value"
        stroke="var(--chart-1, oklch(0.646 0.222 41.116))"
        strokeWidth={1.5}
        dot={false}
        isAnimationActive={false}
      />
    </LineChart>
  );
}
