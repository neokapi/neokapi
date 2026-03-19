import { useMemo } from 'react';
import { BarChart, Bar, XAxis, YAxis } from 'recharts';
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from '@/components/ui/chart';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useFilter } from '@/context/FilterContext';
import { sessions } from '@/data/sessions';

export default function ToolUsageChart() {
  const { workspace } = useFilter();

  const data = useMemo(() => {
    const filtered = workspace
      ? sessions.filter((s) => s.workspace === workspace)
      : sessions;

    const toolMap = new Map<string, number>();

    for (const sess of filtered) {
      for (const tc of sess.toolCalls) {
        toolMap.set(tc.tool, (toolMap.get(tc.tool) ?? 0) + 1);
      }
    }

    const result: { tool: string; count: number }[] = [];
    for (const [tool, count] of toolMap.entries()) {
      result.push({ tool, count });
    }
    result.sort((a, b) => b.count - a.count);
    return result;
  }, [workspace]);

  const chartConfig: ChartConfig = {
    count: {
      label: 'Calls',
      color: 'var(--color-chart-1)',
    },
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">Tool Usage</CardTitle>
      </CardHeader>
      <CardContent>
        <ChartContainer config={chartConfig} className="h-[300px] w-full">
          <BarChart
            data={data}
            layout="vertical"
            margin={{ left: 20, right: 20, top: 5, bottom: 5 }}
          >
            <XAxis type="number" hide />
            <YAxis
              type="category"
              dataKey="tool"
              tickLine={false}
              axisLine={false}
              width={110}
              tick={{ fontSize: 11 }}
            />
            <ChartTooltip content={<ChartTooltipContent />} />
            <Bar dataKey="count" fill="var(--color-count)" radius={[0, 4, 4, 0]} barSize={20} />
          </BarChart>
        </ChartContainer>
      </CardContent>
    </Card>
  );
}
