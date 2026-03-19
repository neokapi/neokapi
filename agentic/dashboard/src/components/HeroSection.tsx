import { motion } from 'framer-motion';
import { agents } from '../data/agents';
import { projects } from '../data/projects';
import { issues } from '../data/issues';
import { tmGrowth } from '../data/metrics';

export default function HeroSection() {
  const totalBlocks = agents.reduce((sum, a) => sum + a.stats.blocksProcessed, 0);
  const totalTM = tmGrowth[tmGrowth.length - 1].value;
  const totalIssues = issues.length;
  const activeProjects = projects.length;

  const stats = [
    { label: "Blocks Translated", value: totalBlocks.toLocaleString() },
    { label: "TM Entries", value: totalTM.toLocaleString() },
    { label: "Issues Filed", value: totalIssues.toString() },
    { label: "Active Projects", value: activeProjects.toString() },
  ];

  return (
    <section className="relative overflow-hidden px-6 py-20">
      {/* Animated background dots */}
      <div className="pointer-events-none absolute inset-0">
        {Array.from({ length: 20 }).map((_, i) => (
          <motion.div
            key={i}
            className="absolute h-1 w-1 rounded-full bg-[var(--color-accent-amber)]"
            style={{
              left: `${(i * 37) % 100}%`,
              top: `${(i * 23) % 100}%`,
              opacity: 0.15,
            }}
            animate={{
              y: [0, -20, 0],
              opacity: [0.1, 0.3, 0.1],
            }}
            transition={{
              duration: 3 + (i % 3),
              repeat: Infinity,
              delay: i * 0.2,
            }}
          />
        ))}
      </div>

      <div className="relative mx-auto max-w-4xl text-center">
        <motion.h1
          className="font-[family-name:var(--font-display)] text-5xl font-bold tracking-tight text-[var(--color-text-primary)] md:text-6xl lg:text-7xl"
          initial={{ opacity: 0, y: 30 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.8 }}
        >
          Bowrain{' '}
          <span className="text-[var(--color-accent-amber)]">Agentic</span>{' '}
          Testing
        </motion.h1>

        <motion.p
          className="mx-auto mt-6 max-w-2xl text-lg text-[var(--color-text-secondary)]"
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.8, delay: 0.2 }}
        >
          7 AI agents localizing open source projects in real-time
        </motion.p>

        <motion.div
          className="mt-12 grid grid-cols-2 gap-6 md:grid-cols-4"
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.8, delay: 0.4 }}
        >
          {stats.map((stat) => (
            <div
              key={stat.label}
              className="rounded-xl border border-[var(--color-border)] bg-[var(--color-bg-card)] p-4"
            >
              <div className="font-[family-name:var(--font-mono)] text-2xl font-bold text-[var(--color-accent-amber)]">
                {stat.value}
              </div>
              <div className="mt-1 text-sm text-[var(--color-text-muted)]">
                {stat.label}
              </div>
            </div>
          ))}
        </motion.div>
      </div>
    </section>
  );
}
