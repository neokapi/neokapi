import { Bar, BarChart, XAxis, YAxis, CartesianGrid, Tooltip, Legend } from "recharts";
import type { LocaleTranslationStats } from "../types/api";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
} from "@neokapi/ui-primitives/components/ui/card";
import {
  ChartContainer,
  ChartLegendContent,
  type ChartConfig,
} from "@neokapi/ui-primitives/components/ui/chart";

interface WordCountChartProps {
  localeStats: LocaleTranslationStats[];
}

const chartConfig: ChartConfig = {
  translated: {
    label: "Translated",
    color: "var(--chart-2, oklch(0.6 0.118 184.704))",
  },
  remaining: {
    label: "Remaining",
    color: "var(--chart-5, oklch(0.769 0.188 70.08))",
  },
};

export function WordCountChart({ localeStats }: WordCountChartProps) {
  const data = localeStats.map((l) => ({
    locale: l.locale,
    translated: l.translated_words,
    remaining: l.total_words - l.translated_words,
  }));

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">Word Count by Language</CardTitle>
      </CardHeader>
      <CardContent>
        <ChartContainer config={chartConfig} className="aspect-auto h-[250px] w-full">
          <BarChart data={data} margin={{ left: 0, right: 16 }}>
            <CartesianGrid vertical={false} />
            <XAxis dataKey="locale" tickLine={false} axisLine={false} tick={{ fontSize: 11 }} />
            <YAxis
              tickLine={false}
              axisLine={false}
              tickFormatter={(v: number) => (v >= 1000 ? `${(v / 1000).toFixed(0)}k` : String(v))}
            />
            <Tooltip
              content={({ active, payload, label }) => {
                if (!active || !payload?.length) return null;
                return (
                  <div className="rounded-lg border border-border/50 bg-background px-2.5 py-1.5 text-xs shadow-xl">
                    <div className="font-medium mb-1">{label}</div>
                    {payload.map((item) => (
                      <div key={String(item.dataKey)} className="flex justify-between gap-4">
                        <span className="text-muted-foreground">
                          {item.dataKey === "translated" ? "Translated" : "Remaining"}
                        </span>
                        <span className="font-mono tabular-nums">
                          {(item.value as number).toLocaleString()}
                        </span>
                      </div>
                    ))}
                  </div>
                );
              }}
            />
            <Legend content={<ChartLegendContent />} />
            <Bar
              dataKey="translated"
              stackId="words"
              fill="var(--color-translated)"
              radius={[0, 0, 0, 0]}
              maxBarSize={40}
            />
            <Bar
              dataKey="remaining"
              stackId="words"
              fill="var(--color-remaining)"
              radius={[4, 4, 0, 0]}
              maxBarSize={40}
            />
          </BarChart>
        </ChartContainer>
      </CardContent>
    </Card>
  );
}
