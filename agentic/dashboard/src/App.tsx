import Header from './components/layout/Header';
import FilterToolbar from './components/FilterToolbar';
import StatsRow from './components/StatsRow';
import AgentCard from './components/AgentCard';
import SessionTable from './components/SessionTable';
import ToolUsageChart from './components/ToolUsageChart';
import SessionHeatmap from './components/SessionHeatmap';
import ActivityFeed from './components/ActivityFeed';
import IssuesFeed from './components/IssuesFeed';
import { FilterProvider, useFilter } from './context/FilterContext';
import { ThemeProvider } from './context/ThemeContext';
import { Separator } from '@/components/ui/separator';
import { agents } from './data/agents';

function DashboardContent() {
  const { filters } = useFilter();

  const filteredAgents = filters.workspace
    ? agents.filter((a) => a.workspace === filters.workspace)
    : agents;

  return (
    <div className="min-h-screen bg-background">
      <Header />
      <FilterToolbar />

      <main className="mx-auto max-w-7xl space-y-8 px-4 py-8 sm:px-6">
        {/* Stats Row */}
        <StatsRow />

        <Separator />

        {/* Agent Cards */}
        <section>
          <h2 className="mb-4 text-lg font-semibold">Agents</h2>
          <div className="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {filteredAgents.map((agent) => (
              <AgentCard key={agent.id} agent={agent} />
            ))}
          </div>
        </section>

        <Separator />

        {/* Session Table */}
        <section>
          <h2 className="mb-4 text-lg font-semibold">Sessions</h2>
          <SessionTable />
        </section>

        <Separator />

        {/* Two columns: Tool usage chart | Session heatmap */}
        <div className="grid gap-6 grid-cols-1 lg:grid-cols-2">
          <ToolUsageChart />
          <SessionHeatmap />
        </div>

        <Separator />

        {/* Two columns: Activity feed | Issues */}
        <div className="grid gap-6 grid-cols-1 lg:grid-cols-2">
          <ActivityFeed />
          <IssuesFeed />
        </div>
      </main>

      {/* Footer */}
      <footer className="border-t px-4 py-8 sm:px-6">
        <div className="mx-auto max-w-7xl text-center">
          <p className="font-mono text-xs text-muted-foreground">
            Bowrain Agentic Operations Dashboard — Powered by{' '}
            <a
              href="https://github.com/neokapi/neokapi"
              target="_blank"
              rel="noopener noreferrer"
              className="text-foreground hover:underline"
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
