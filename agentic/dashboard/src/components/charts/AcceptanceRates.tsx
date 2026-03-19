import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from 'recharts';
import { aiAcceptanceRates } from '../../data/metrics';

export default function AcceptanceRates() {
  return (
    <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-bg-card)] p-5">
      <h3 className="mb-4 font-[family-name:var(--font-mono)] text-sm font-semibold text-[var(--color-text-primary)]">
        AI Suggestion Acceptance Rates (%)
      </h3>
      <ResponsiveContainer width="100%" height={280}>
        <BarChart data={aiAcceptanceRates} layout="vertical">
          <CartesianGrid strokeDasharray="3 3" stroke="#2a2a3a" horizontal={false} />
          <XAxis type="number" tick={{ fill: '#8888a0', fontSize: 11 }} stroke="#2a2a3a" domain={[0, 100]} />
          <YAxis type="category" dataKey="translator" tick={{ fill: '#8888a0', fontSize: 11 }} stroke="#2a2a3a" width={80} />
          <Tooltip
            contentStyle={{
              backgroundColor: '#16161f',
              border: '1px solid #2a2a3a',
              borderRadius: '8px',
              color: '#e8e8ed',
              fontSize: '12px',
            }}
          />
          <Legend wrapperStyle={{ fontSize: '11px' }} />
          <Bar dataKey="accepted" stackId="a" fill="#22c55e" name="Accepted" />
          <Bar dataKey="edited" stackId="a" fill="#f59e0b" name="Edited" />
          <Bar dataKey="rejected" stackId="a" fill="#ef4444" name="Rejected" />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}
