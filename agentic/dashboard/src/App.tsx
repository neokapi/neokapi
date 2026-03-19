import Header from './components/layout/Header';
import FilterBar from './components/FilterBar';
import StatsRow from './components/StatsRow';
import AgentCard from './components/AgentCard';
import ContentTabs from './components/ContentTabs';
import SessionHeatmap from './components/SessionHeatmap';
import BowrainContext from './components/BowrainContext';
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

      {/* FilterBar — sticky below header */}
      <div className="sticky top-[53px] z-40 border-b bg-background/95 backdrop-blur">
        <div className="mx-auto max-w-7xl">
          <FilterBar />
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
          <div className="grid gap-4 grid-cols-2 md:grid-cols-4">
            {filteredAgents.map((a) => (
              <AgentCard key={a.id} agent={a} />
            ))}
          </div>
        </div>
      </section>

      <div className="mx-auto max-w-7xl px-6">
        <Separator />
      </div>

      {/* Content Tabs */}
      <section className="px-4 py-8 sm:px-6">
        <div className="mx-auto max-w-7xl">
          <ContentTabs />
        </div>
      </section>

      <div className="mx-auto max-w-7xl px-6">
        <Separator />
      </div>

      {/* Bottom row: Heatmap + Bowrain Context */}
      <section className="px-4 py-8 sm:px-6">
        <div className="mx-auto grid max-w-7xl gap-6 grid-cols-1 lg:grid-cols-2">
          <SessionHeatmap />
          <BowrainContext />
        </div>
      </section>

      {/* Footer */}
      <footer className="border-t px-4 py-8 sm:px-6">
        <div className="mx-auto max-w-7xl text-center">
          <p className="font-mono text-xs text-muted-foreground">
            Bowrain Agentic Operations Dashboard &mdash; Powered by{' '}
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
