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
import { ThemeProvider } from './context/ThemeContext';
import { agents } from './data/agents';
import { workspaces } from './data/workspaces';

const sectionVariants = {
  hidden: { opacity: 0, y: 24 },
  visible: { opacity: 1, y: 0 },
};

const staggerContainer = {
  visible: {
    transition: {
      staggerChildren: 0.08,
    },
  },
};

function SectionDivider() {
  return (
    <div className="mx-auto max-w-7xl px-6">
      <div
        style={{
          height: '1px',
          background: `linear-gradient(to right, transparent, rgb(var(--border)), transparent)`,
        }}
      />
    </div>
  );
}

function DashboardContent() {
  const { selectedWorkspace } = useFilter();

  const filteredAgents = selectedWorkspace
    ? agents.filter((a) => a.workspace === selectedWorkspace)
    : agents;

  return (
    <div className="min-h-screen" style={{ backgroundColor: 'rgb(var(--bg-base))' }}>
      <Header />

      {/* Workspace selector bar */}
      <div
        className="sticky top-[53px] z-40"
        style={{
          borderBottom: '1px solid rgb(var(--border))',
          backgroundColor: 'rgb(var(--bg-base) / 0.95)',
          backdropFilter: 'blur(8px)',
        }}
      >
        <div className="mx-auto max-w-7xl">
          <WorkspaceSelector />
        </div>
      </div>

      <HeroSection />

      {/* Workspace overview cards -- only when "All Workspaces" */}
      {!selectedWorkspace && (
        <motion.section
          className="px-4 py-16 sm:px-6"
          variants={sectionVariants}
          initial="hidden"
          whileInView="visible"
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
        >
          <div className="mx-auto max-w-7xl">
            <h2
              className="font-display mb-6 text-xl font-normal sm:text-2xl"
              style={{ color: 'rgb(var(--text-primary))' }}
            >
              Workspaces
            </h2>
            <motion.div
              className="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3"
              variants={staggerContainer}
              initial="hidden"
              whileInView="visible"
              viewport={{ once: true }}
            >
              {workspaces.map((ws, i) => (
                <WorkspaceCard key={ws.id} workspace={ws} index={i} />
              ))}
            </motion.div>
          </div>
        </motion.section>
      )}

      <SectionDivider />

      {/* Agent Cards */}
      <motion.section
        className="px-4 py-16 sm:px-6"
        variants={sectionVariants}
        initial="hidden"
        whileInView="visible"
        viewport={{ once: true }}
        transition={{ duration: 0.6 }}
      >
        <div className="mx-auto max-w-7xl">
          <h2
            className="font-display mb-6 text-xl font-normal sm:text-2xl"
            style={{ color: 'rgb(var(--text-primary))' }}
          >
            Agent Roster
          </h2>
          {/* Mobile: vertical grid, Desktop: horizontal scroll */}
          <div className="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:hidden">
            {filteredAgents.map((agent, i) => (
              <AgentCard key={agent.id} agent={agent} index={i} />
            ))}
          </div>
          <div className="hidden lg:flex gap-4 overflow-x-auto pb-4">
            {filteredAgents.map((agent, i) => (
              <AgentCard key={agent.id} agent={agent} index={i} />
            ))}
          </div>
        </div>
      </motion.section>

      <SectionDivider />

      <HandoffChain />

      <SectionDivider />

      {/* Session Timeline -- THE CENTERPIECE */}
      <SessionTimeline />

      <SectionDivider />

      {/* Two-column: Tool usage chart | Session heatmap */}
      <section className="px-4 py-16 sm:px-6">
        <div className="mx-auto grid max-w-7xl gap-6 grid-cols-1 lg:grid-cols-2">
          <ToolUsageChart />
          <SessionHeatmap />
        </div>
      </section>

      <SectionDivider />

      {/* Activity feed + Issues feed side by side */}
      <section className="px-4 py-16 sm:px-6">
        <div className="mx-auto grid max-w-7xl gap-6 grid-cols-1 lg:grid-cols-2">
          <ActivityFeed />
          <IssuesFeed />
        </div>
      </section>

      {/* Footer */}
      <footer
        className="px-4 py-8 sm:px-6"
        style={{ borderTop: '1px solid rgb(var(--border))' }}
      >
        <div className="mx-auto max-w-7xl text-center">
          <p
            className="font-mono text-xs"
            style={{ color: 'rgb(var(--text-muted))' }}
          >
            Bowrain Agentic Operations Dashboard — Powered by{' '}
            <a
              href="https://github.com/neokapi/neokapi"
              target="_blank"
              rel="noopener noreferrer"
              className="hover:underline"
              style={{ color: 'rgb(var(--accent))' }}
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
    <ThemeProvider>
      <FilterProvider>
        <DashboardContent />
      </FilterProvider>
    </ThemeProvider>
  );
}

export default App;
