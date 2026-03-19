import { motion } from 'framer-motion';
import { workspaces } from '../data/workspaces';
import { sessions } from '../data/sessions';
import { issues } from '../data/issues';

export default function HeroSection() {
  const activeWorkspaces = workspaces.filter((w) => w.status === 'active').length;

  const now = Date.now();
  const weekMs = 7 * 24 * 3_600_000;
  const dayMs = 24 * 3_600_000;

  const sessionsThisWeek = sessions.filter(
    (s) => now - new Date(s.startTime).getTime() < weekMs
  ).length;

  const toolCallsToday = sessions
    .filter((s) => now - new Date(s.startTime).getTime() < dayMs)
    .reduce((sum, s) => sum + s.toolCalls.length, 0);

  const totalIssues = issues.length;

  const stats = [
    { label: 'Active Workspaces', value: activeWorkspaces.toString() },
    { label: 'Sessions This Week', value: sessionsThisWeek.toString() },
    { label: 'Tool Calls Today', value: toolCallsToday.toString() },
    { label: 'Issues Filed', value: totalIssues.toString() },
  ];

  return (
    <section className="relative overflow-hidden px-4 py-16 sm:px-6 sm:py-20">
      {/* Radial gradient background */}
      <div
        className="pointer-events-none absolute inset-0"
        style={{
          background: `radial-gradient(ellipse at center top, rgb(var(--accent) / 0.06) 0%, transparent 60%)`,
        }}
      />

      <div className="relative mx-auto max-w-4xl text-center">
        <motion.h1
          className="font-display text-4xl font-normal tracking-tight sm:text-5xl md:text-6xl lg:text-7xl"
          style={{ color: 'rgb(var(--text-primary))' }}
          initial={{ opacity: 0, y: 30 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.8 }}
        >
          Agent{' '}
          <span style={{ color: 'rgb(var(--accent))' }}>Operations</span>
        </motion.h1>

        <motion.p
          className="mx-auto mt-4 max-w-2xl text-base sm:mt-6 sm:text-lg"
          style={{ color: 'rgb(var(--text-secondary))' }}
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.8, delay: 0.2 }}
        >
          What are my agents doing? Sessions, tools, and workspace activity.
        </motion.p>

        <motion.div
          className="mt-8 grid grid-cols-2 gap-4 sm:mt-12 sm:gap-6 md:grid-cols-4"
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.8, delay: 0.4 }}
        >
          {stats.map((stat) => (
            <div
              key={stat.label}
              className="animate-pulse-border rounded-xl p-4"
              style={{
                backgroundColor: 'rgb(var(--bg-card))',
                border: '1px solid rgb(var(--accent) / 0.2)',
              }}
            >
              <div
                className="font-mono text-2xl font-bold sm:text-3xl"
                style={{ color: 'rgb(var(--accent))' }}
              >
                {stat.value}
              </div>
              <div
                className="mt-1 text-xs sm:text-sm"
                style={{ color: 'rgb(var(--text-muted))' }}
              >
                {stat.label}
              </div>
            </div>
          ))}
        </motion.div>
      </div>
    </section>
  );
}
