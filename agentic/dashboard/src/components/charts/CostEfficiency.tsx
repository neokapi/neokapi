import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import { costEfficiency } from '../../data/metrics';

export default function CostEfficiency() {
  return (
    <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-bg-card)] p-5">
      <h3 className="mb-4 font-[family-name:var(--font-mono)] text-sm font-semibold text-[var(--color-text-primary)]">
        Cost per Word ($)
      </h3>
      <ResponsiveContainer width="100%" height={280}>
        <AreaChart data={costEfficiency}>
          <defs>
            <linearGradient id="costGradient" x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="#10b981" stopOpacity={0.4} />
              <stop offset="95%" stopColor="#10b981" stopOpacity={0.05} />
            </linearGradient>
          </defs>
          <CartesianGrid strokeDasharray="3 3" stroke="#2a2a3a" />
          <XAxis dataKey="week" tick={{ fill: '#8888a0', fontSize: 11 }} stroke="#2a2a3a" />
          <YAxis tick={{ fill: '#8888a0', fontSize: 11 }} stroke="#2a2a3a" tickFormatter={(v) => `$${v}`} />
          <Tooltip
            contentStyle={{
              backgroundColor: '#16161f',
              border: '1px solid #2a2a3a',
              borderRadius: '8px',
              color: '#e8e8ed',
              fontSize: '12px',
            }}
            formatter={(value) => [`$${Number(value).toFixed(3)}`, 'Cost/word']}
          />
          <Area type="monotone" dataKey="value" stroke="#10b981" fill="url(#costGradient)" strokeWidth={2} />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}
