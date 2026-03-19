import Header from './components/layout/Header';
import FilterToolbar from './components/FilterToolbar';
import StatsRow from './components/StatsRow';
import AgentCard from './components/AgentCard';
import SessionTable from './components/SessionTable';
import ToolUsageChart from './components/ToolUsageChart';
import SessionHeatmap from './components/SessionHeatmap';
import ActivityFeed from './components/ActivityFeed';
import IssuesFeed from './components/IssuesFeed';
import { Separator } from './components/ui/separator';
import { FilterProvider, useFilter } from './context/FilterContext';
import { ThemeProvider } from './context/ThemeContext';
import { agents } from './data/agents';

function DashboardContent() {
  const { workspace, agent } = useFilter();

  let filteredAgents = agents;
  if (workspace) filteredAgents = filteredAgents.filter((a) => a.workspace === workspace);
  if (agent) filteredAgents = filteredAgents.filter((a) => a.id === agent);

  return (
    <div className="min-h-screen bg-background">
      <Header />

      <div className="sticky top-[53px] z-40 border-b bg-background/95 backdrop-blur">
        <div className="mx-auto max-w-7xl">
          <FilterToolbar />
        </div>
      </div>

      {/* Stats */}
      <section className="px-4 py-8 sm:px-6">
        <div className="mx-auto max-w-7xl">
          <StatsRow />
        </div>
      </section>

      <div className="mx-auto max-w-7xl px-6">
        <Separator />
      </div>

      {/* Agent Cards */}
      <section className="px-4 py-8 sm:px-6">
        <div className="mx-auto max-w-7xl">
          <h2 className="mb-4 text-lg font-semibold">Agent Roster</h2>
          <div className="flex gap-4 overflow-x-auto pb-4">
            {filteredAgents.map((a) => (
              <AgentCard key={a.id} agent={a} />
            ))}
          </div>
        </div>
      </section>

      <div className="mx-auto max-w-7xl px-6">
        <Separator />
      </div>

      {/* Session Table */}
      <section className="px-4 py-8 sm:px-6">
        <div className="mx-auto max-w-7xl">
          <h2 className="mb-4 text-lg font-semibold">Sessions</h2>
          <SessionTable />
        </div>
      </section>

      <div className="mx-auto max-w-7xl px-6">
        <Separator />
      </div>

      {/* Tool usage chart + Session heatmap */}
      <section className="px-4 py-8 sm:px-6">
        <div className="mx-auto grid max-w-7xl gap-6 grid-cols-1 lg:grid-cols-2">
          <ToolUsageChart />
          <SessionHeatmap />
        </div>
      </section>

      <div className="mx-auto max-w-7xl px-6">
        <Separator />
      </div>

      {/* Activity feed + Issues feed */}
      <section className="px-4 py-8 sm:px-6">
        <div className="mx-auto grid max-w-7xl gap-6 grid-cols-1 lg:grid-cols-2">
          <ActivityFeed />
          <IssuesFeed />
        </div>
      </section>

      {/* Footer */}
      <footer className="border-t px-4 py-8 sm:px-6">
        <div className="mx-auto max-w-7xl text-center">
          <p className="font-mono text-xs text-muted-foreground">
            Bowrain Agentic Operations Dashboard — Powered by{' '}
            <a
              href="https://github.com/neokapi/neokapi"
              target="_blank"
              rel="noopener noreferrer"
              className="hover:underline"
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
