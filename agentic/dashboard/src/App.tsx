import { motion } from 'framer-motion';
import Header from './components/layout/Header';
import HeroSection from './components/HeroSection';
import WorkspaceSelector from './components/WorkspaceSelector';
import WorkspaceCard from './components/WorkspaceCard';
import AgentCard from './components/AgentCard';
import HandoffChain from './components/HandoffChain';
import SessionTimeline from './components/SessionTimeline';
import ToolUsageChart from './components/ToolUsageChart';
import SessionHeatmap from './components/SessionHeatmap';
import ActivityFeed from './components/ActivityFeed';
import IssuesFeed from './components/IssuesFeed';
import { FilterProvider, useFilter } from './context/FilterContext';
import { agents } from './data/agents';
import { workspaces } from './data/workspaces';

function DashboardContent() {
  const { selectedWorkspace } = useFilter();

  const filteredAgents = selectedWorkspace
    ? agents.filter((a) => a.workspace === selectedWorkspace)
    : agents;

  return (
    <div className="min-h-screen bg-[var(--color-bg-primary)]">
      <Header />

      {/* Workspace selector bar */}
      <div className="sticky top-[53px] z-40 border-b border-[var(--color-border)] bg-[var(--color-bg-primary)]/95 backdrop-blur-sm">
        <div className="mx-auto max-w-7xl">
          <WorkspaceSelector />
        </div>
      </div>

      <HeroSection />

      {/* Workspace overview cards — only when "All Workspaces" */}
      {!selectedWorkspace && (
        <motion.section
          className="px-6 py-8"
          initial={{ opacity: 0 }}
          whileInView={{ opacity: 1 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
        >
          <div className="mx-auto max-w-7xl">
            <h2 className="mb-6 font-[family-name:var(--font-display)] text-2xl font-bold text-[var(--color-text-primary)]">
              Workspaces
            </h2>
            <div className="grid gap-4 md:grid-cols-3">
              {workspaces.map((ws, i) => (
                <WorkspaceCard key={ws.id} workspace={ws} index={i} />
              ))}
            </div>
          </div>
        </motion.section>
      )}

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
            {filteredAgents.map((agent, i) => (
              <AgentCard key={agent.id} agent={agent} index={i} />
            ))}
          </div>
        </div>
      </motion.section>

      <HandoffChain />

      {/* Session Timeline — THE CENTERPIECE */}
      <SessionTimeline />

      {/* Two-column: Tool usage chart | Session heatmap */}
      <section className="px-6 py-8">
        <div className="mx-auto grid max-w-7xl gap-6 lg:grid-cols-2">
          <ToolUsageChart />
          <SessionHeatmap />
        </div>
      </section>

      {/* Activity feed + Issues feed side by side */}
      <section className="px-6 py-8">
        <div className="mx-auto grid max-w-7xl gap-6 lg:grid-cols-2">
          <ActivityFeed />
          <IssuesFeed />
        </div>
      </section>

      {/* Footer */}
      <footer className="border-t border-[var(--color-border)] px-6 py-8">
        <div className="mx-auto max-w-7xl text-center">
          <p className="font-[family-name:var(--font-mono)] text-xs text-[var(--color-text-muted)]">
            Bowrain Agentic Operations Dashboard — Powered by{' '}
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

function App() {
  return (
    <FilterProvider>
      <DashboardContent />
    </FilterProvider>
  );
}

export default App;
