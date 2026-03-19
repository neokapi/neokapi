import { useMemo } from 'react';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useFilter } from '../context/FilterContext';
import { workspaces } from '../data/workspaces';
import { agents } from '../data/agents';

export default function FilterToolbar() {
  const { filters, setSelectedWorkspace, setAgent } = useFilter();

  const filteredAgents = useMemo(() => {
    if (!filters.workspace) return agents;
    return agents.filter((a) => a.workspace === filters.workspace);
  }, [filters.workspace]);

  return (
    <div className="sticky top-[53px] z-40 border-b bg-background/95 backdrop-blur-sm">
      <div className="mx-auto flex max-w-7xl items-center gap-3 px-4 py-3 sm:px-6">
        <Select
          value={filters.workspace ?? 'all'}
          onValueChange={(v) => setSelectedWorkspace(v === 'all' ? null : v)}
        >
          <SelectTrigger className="w-[180px]">
            <SelectValue placeholder="Workspace" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Workspaces</SelectItem>
            {workspaces.map((ws) => (
              <SelectItem key={ws.id} value={ws.id}>
                {ws.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={filters.agent ?? 'all'}
          onValueChange={(v) => setAgent(v === 'all' ? null : v)}
        >
          <SelectTrigger className="w-[180px]">
            <SelectValue placeholder="Agent" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Agents</SelectItem>
            {filteredAgents.map((agent) => (
              <SelectItem key={agent.id} value={agent.id}>
                {agent.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}
