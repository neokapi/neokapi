import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from 'recharts';
import { translationProgress } from '../../data/metrics';

export default function TranslationProgress() {
  return (
    <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-bg-card)] p-5">
      <h3 className="mb-4 font-[family-name:var(--font-mono)] text-sm font-semibold text-[var(--color-text-primary)]">
        Translation Progress (%)
      </h3>
      <ResponsiveContainer width="100%" height={280}>
        <AreaChart data={translationProgress}>
          <CartesianGrid strokeDasharray="3 3" stroke="#2a2a3a" />
          <XAxis dataKey="week" tick={{ fill: '#8888a0', fontSize: 11 }} stroke="#2a2a3a" />
          <YAxis tick={{ fill: '#8888a0', fontSize: 11 }} stroke="#2a2a3a" domain={[0, 100]} />
          <Tooltip
            contentStyle={{
              backgroundColor: '#16161f',
              border: '1px solid #2a2a3a',
              borderRadius: '8px',
              color: '#e8e8ed',
              fontSize: '12px',
            }}
          />
          <Legend wrapperStyle={{ fontSize: '11px', color: '#8888a0' }} />
          <Area type="monotone" dataKey="french" stackId="1" stroke="#3b82f6" fill="#3b82f6" fillOpacity={0.3} />
          <Area type="monotone" dataKey="german" stackId="1" stroke="#f43f5e" fill="#f43f5e" fillOpacity={0.3} />
          <Area type="monotone" dataKey="japanese" stackId="1" stroke="#8b5cf6" fill="#8b5cf6" fillOpacity={0.3} />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}
