import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from 'recharts';
import { qualityScores } from '../../data/metrics';

export default function QualityTrends() {
  return (
    <div className="rounded-xl border border-[var(--color-border)] bg-[var(--color-bg-card)] p-5">
      <h3 className="mb-4 font-[family-name:var(--font-mono)] text-sm font-semibold text-[var(--color-text-primary)]">
        QA Pass Rate (%)
      </h3>
      <ResponsiveContainer width="100%" height={280}>
        <LineChart data={qualityScores}>
          <CartesianGrid strokeDasharray="3 3" stroke="#2a2a3a" />
          <XAxis dataKey="week" tick={{ fill: '#8888a0', fontSize: 11 }} stroke="#2a2a3a" />
          <YAxis tick={{ fill: '#8888a0', fontSize: 11 }} stroke="#2a2a3a" domain={[60, 100]} />
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
          <Line type="monotone" dataKey="french" stroke="#3b82f6" strokeWidth={2} dot={{ r: 2 }} />
          <Line type="monotone" dataKey="german" stroke="#f43f5e" strokeWidth={2} dot={{ r: 2 }} />
          <Line type="monotone" dataKey="japanese" stroke="#8b5cf6" strokeWidth={2} dot={{ r: 2 }} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
