import { useMemo } from 'react';
import { Bar, BarChart, XAxis, YAxis } from 'recharts';
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from '@/components/ui/chart';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useFilter } from '../context/FilterContext';
import { sessions } from '../data/sessions';

const chartConfig = {
  count: {
    label: 'Calls',
    color: 'var(--color-chart-1)',
  },
} satisfies ChartConfig;

export default function ToolUsageChart() {
  const { filters } = useFilter();

  const data = useMemo(() => {
    let filtered = sessions;
    if (filters.workspace) {
      filtered = filtered.filter((s) => s.workspace === filters.workspace);
    }
    if (filters.agent) {
      filtered = filtered.filter((s) => s.agentId === filters.agent);
    }

    const toolMap = new Map<string, number>();
    for (const sess of filtered) {
      for (const tc of sess.toolCalls) {
        toolMap.set(tc.tool, (toolMap.get(tc.tool) ?? 0) + 1);
      }
    }

    const result = [...toolMap.entries()]
      .map(([tool, count]) => ({ tool, count }))
      .sort((a, b) => b.count - a.count);

    return result;
  }, [filters.workspace, filters.agent]);

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-semibold">Tool Usage</CardTitle>
      </CardHeader>
      <CardContent>
        <ChartContainer config={chartConfig} className="h-[300px] w-full">
          <BarChart
            data={data}
            layout="vertical"
            margin={{ left: 0, right: 16, top: 0, bottom: 0 }}
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
            <Bar
              dataKey="count"
              fill="var(--color-count)"
              radius={[0, 4, 4, 0]}
              barSize={20}
            />
          </BarChart>
        </ChartContainer>
      </CardContent>
    </Card>
  );
}
