import { motion } from 'framer-motion';
import Header from './components/layout/Header';
import HeroSection from './components/HeroSection';
import AgentCard from './components/AgentCard';
import HandoffChain from './components/HandoffChain';
import ActivityFeed from './components/ActivityFeed';
import ProjectCard from './components/ProjectCard';
import TranslationProgress from './components/charts/TranslationProgress';
import TMGrowth from './components/charts/TMGrowth';
import QualityTrends from './components/charts/QualityTrends';
import AcceptanceRates from './components/charts/AcceptanceRates';
import CostEfficiency from './components/charts/CostEfficiency';
import ActivityHeatmap from './components/charts/ActivityHeatmap';
import IssuesFeed from './components/IssuesFeed';
import { agents } from './data/agents';
import { projects } from './data/projects';

function App() {
  return (
    <div className="min-h-screen bg-[var(--color-bg-primary)]">
      <Header />
      <HeroSection />

      {/* Agent Cards — horizontal scroll */}
      <motion.section
        className="px-6 py-8"
        initial={{ opacity: 0 }}
        whileInView={{ opacity: 1 }}
        viewport={{ once: true }}
        transition={{ duration: 0.6 }}
      >
        <div className="mx-auto max-w-7xl">
          <h2 className="mb-6 font-[family-name:var(--font-display)] text-2xl font-bold text-[var(--color-text-primary)]">
            Agent Roster
          </h2>
          <div className="flex gap-4 overflow-x-auto pb-4">
            {agents.map((agent, i) => (
              <AgentCard key={agent.id} agent={agent} index={i} />
            ))}
          </div>
        </div>
      </motion.section>

      <HandoffChain />

      {/* Activity + Projects side-by-side */}
      <section className="px-6 py-8">
        <div className="mx-auto grid max-w-7xl gap-6 lg:grid-cols-[1fr_2fr]">
          <ActivityFeed />
          <div>
            <h2 className="mb-4 font-[family-name:var(--font-display)] text-2xl font-bold text-[var(--color-text-primary)]">
              Projects
            </h2>
            <div className="grid gap-4 md:grid-cols-2">
              {projects.map((project, i) => (
                <ProjectCard key={project.name} project={project} index={i} />
              ))}
            </div>
          </div>
        </div>
      </section>

      {/* Charts section — 2-column grid */}
      <motion.section
        className="px-6 py-8"
        initial={{ opacity: 0, y: 30 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true }}
        transition={{ duration: 0.6 }}
      >
        <div className="mx-auto max-w-7xl">
          <h2 className="mb-6 font-[family-name:var(--font-display)] text-2xl font-bold text-[var(--color-text-primary)]">
            Metrics
          </h2>
          <div className="grid gap-6 md:grid-cols-2">
            <TranslationProgress />
            <TMGrowth />
            <QualityTrends />
            <AcceptanceRates />
            <CostEfficiency />
            <ActivityHeatmap />
          </div>
        </div>
      </motion.section>

      <IssuesFeed />

      {/* Footer */}
      <footer className="border-t border-[var(--color-border)] px-6 py-8">
        <div className="mx-auto max-w-7xl text-center">
          <p className="font-[family-name:var(--font-mono)] text-xs text-[var(--color-text-muted)]">
            Bowrain Agentic Testing Dashboard — Powered by{' '}
            <a
              href="https://github.com/neokapi/neokapi"
              target="_blank"
              rel="noopener noreferrer"
              className="text-[var(--color-accent-amber)] hover:underline"
            >
              neokapi
            </a>
          </p>
        </div>
      </footer>
    </div>
  );
}

export default App;
