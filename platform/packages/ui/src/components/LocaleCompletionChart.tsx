import { Bar, BarChart, XAxis, YAxis, CartesianGrid, Tooltip } from "recharts";
import type { LocaleTranslationStats } from "../types/api";
import { LanguageLabel, localeDisplayName } from "./LanguageLabel";
import { Card, CardHeader, CardTitle, CardContent } from "./ui/card";
import { ChartContainer, type ChartConfig } from "./ui/chart";

interface LocaleCompletionChartProps {
  localeStats: LocaleTranslationStats[];
}

const chartConfig: ChartConfig = {
  percentage: {
    label: "Completion",
    color: "var(--chart-1, oklch(0.646 0.222 41.116))",
  },
};

interface TooltipPayload {
  payload?: {
    locale?: string;
    localeCode?: string;
    displayName?: string;
    percentage?: number;
    translated_words?: number;
    total_words?: number;
    translated_blocks?: number;
    total_blocks?: number;
  };
}

function CustomTooltip({ active, payload }: { active?: boolean; payload?: TooltipPayload[] }) {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  if (!d) return null;
  return (
    <div className="rounded-lg border border-border/50 bg-background px-2.5 py-1.5 text-xs shadow-xl">
      <div className="font-medium">
        <LanguageLabel code={d.localeCode ?? d.locale ?? ""} displayName={d.displayName} hideCode />
        : {d.percentage}% complete
      </div>
      <div className="text-muted-foreground">
        {(d.translated_words ?? 0).toLocaleString()} / {(d.total_words ?? 0).toLocaleString()} words
      </div>
      <div className="text-muted-foreground">
        {(d.translated_blocks ?? 0).toLocaleString()} / {(d.total_blocks ?? 0).toLocaleString()}{" "}
        blocks
      </div>
    </div>
  );
}

export function LocaleCompletionChart({ localeStats }: LocaleCompletionChartProps) {
  const data = localeStats.map((l) => ({
    locale: l.display_name ?? localeDisplayName(l.locale, "short"),
    localeCode: l.locale,
    displayName: l.display_name,
    percentage: Math.round(l.percentage * 10) / 10,
    translated_words: l.translated_words,
    total_words: l.total_words,
    translated_blocks: l.translated_blocks,
    total_blocks: l.total_blocks,
  }));

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">Completion by Language</CardTitle>
      </CardHeader>
      <CardContent>
        <ChartContainer config={chartConfig} className="aspect-auto h-[250px] w-full">
          <BarChart data={data} layout="vertical" margin={{ left: 0, right: 16 }}>
            <CartesianGrid horizontal={false} />
            <YAxis
              dataKey="locale"
              type="category"
              tickLine={false}
              axisLine={false}
              width={100}
              tick={{ fontSize: 11 }}
            />
            <XAxis
              type="number"
              domain={[0, 100]}
              tickLine={false}
              axisLine={false}
              tickFormatter={(v: number) => `${v}%`}
            />
            <Tooltip content={<CustomTooltip />} />
            <Bar
              dataKey="percentage"
              fill="var(--color-percentage)"
              radius={[0, 4, 4, 0]}
              maxBarSize={24}
            />
          </BarChart>
        </ChartContainer>
      </CardContent>
    </Card>
  );
}
