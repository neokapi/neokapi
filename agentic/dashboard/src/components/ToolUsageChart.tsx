import { motion } from 'framer-motion';
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell } from 'recharts';
import { useMemo } from 'react';
import { useFilter } from '../context/FilterContext';
import { sessions } from '../data/sessions';
import { agents, accentColorMap } from '../data/agents';
import { useTheme } from '../context/ThemeContext';

interface ToolData {
  tool: string;
  count: number;
  byAgent: Record<string, number>;
}

export default function ToolUsageChart() {
  const { selectedWorkspace } = useFilter();
  const { theme } = useTheme();

  const data = useMemo(() => {
    const filtered = selectedWorkspace
      ? sessions.filter((s) => s.workspace === selectedWorkspace)
      : sessions;

    const toolMap = new Map<
      string,
      { count: number; byAgent: Record<string, number> }
    >();

    for (const sess of filtered) {
      for (const tc of sess.toolCalls) {
        const entry = toolMap.get(tc.tool) ?? { count: 0, byAgent: {} };
        entry.count++;
        entry.byAgent[sess.agentId] =
          (entry.byAgent[sess.agentId] ?? 0) + 1;
        toolMap.set(tc.tool, entry);
      }
    }

    const result: ToolData[] = [];
    for (const [tool, val] of toolMap.entries()) {
      result.push({ tool, count: val.count, byAgent: val.byAgent });
    }
    result.sort((a, b) => b.count - a.count);
    return result;
  }, [selectedWorkspace]);

  const toolColors = useMemo(() => {
    const colorMap: Record<string, string> = {};
    for (const d of data) {
      let maxAgent = '';
      let maxCount = 0;
      for (const [agentId, count] of Object.entries(d.byAgent)) {
        if (count > maxCount) {
          maxCount = count;
          maxAgent = agentId;
        }
      }
      const agent = agents.find((a) => a.id === maxAgent);
      colorMap[d.tool] = agent
        ? accentColorMap[agent.accentColor] || '#d9a03c'
        : '#d9a03c';
    }
    return colorMap;
  }, [data]);

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const CustomTooltip = ({ active, payload }: { active?: boolean; payload?: any[] }) => {
    if (!active || !payload?.[0]) return null;
    const d = payload[0].payload as ToolData;
    return (
      <div
        className="rounded-lg p-2.5 shadow-lg"
        style={{
          backgroundColor: 'rgb(var(--bg-base))',
          border: '1px solid rgb(var(--border))',
        }}
      >
        <div
          className="font-mono text-xs font-semibold"
          style={{ color: 'rgb(var(--text-primary))' }}
        >
          {d.tool}
        </div>
        <div
          className="mt-1 text-[10px]"
          style={{ color: 'rgb(var(--text-muted))' }}
        >
          {d.count} calls total
        </div>
        <div className="mt-1 space-y-0.5">
          {Object.entries(d.byAgent)
            .sort(([, a], [, b]) => b - a)
            .map(([agentId, count]) => {
              const agent = agents.find((a) => a.id === agentId);
              return (
                <div
                  key={agentId}
                  className="flex items-center gap-1.5 text-[10px]"
                  style={{ color: 'rgb(var(--text-secondary))' }}
                >
                  <span>{agent?.avatar}</span>
                  <span>
                    {agent?.name}: {count}
                  </span>
                </div>
              );
            })}
        </div>
      </div>
    );
  };

  const axisTextColor =
    theme === 'dark' ? 'rgb(160, 152, 140)' : 'rgb(100, 90, 80)';
  const mutedColor =
    theme === 'dark' ? 'rgb(100, 94, 86)' : 'rgb(155, 148, 138)';

  return (
    <motion.div
      className="rounded-xl p-5"
      style={{
        backgroundColor: 'rgb(var(--bg-card))',
        border: '1px solid rgb(var(--border))',
      }}
      initial={{ opacity: 0, y: 20 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true }}
      transition={{ duration: 0.5 }}
    >
      <h3
        className="mb-4 font-mono text-sm font-semibold"
        style={{ color: 'rgb(var(--text-primary))' }}
      >
        Tool Usage
      </h3>
      <ResponsiveContainer width="100%" height={300}>
        <BarChart
          data={data}
          layout="vertical"
          margin={{ left: 20, right: 20, top: 5, bottom: 5 }}
        >
          <XAxis
            type="number"
            tick={{ fontSize: 10, fill: mutedColor }}
            axisLine={false}
            tickLine={false}
          />
          <YAxis
            type="category"
            dataKey="tool"
            tick={{ fontSize: 11, fill: axisTextColor }}
            axisLine={false}
            tickLine={false}
            width={110}
          />
          <Tooltip content={<CustomTooltip />} cursor={false} />
          <Bar dataKey="count" radius={[0, 4, 4, 0]} barSize={20}>
            {data.map((d) => (
              <Cell key={d.tool} fill={toolColors[d.tool]} fillOpacity={0.8} />
            ))}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </motion.div>
  );
}
