import { useFilter } from '@/context/FilterContext';
import { workspaces } from '@/data/workspaces';
import { agents } from '@/data/agents';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

export default function FilterToolbar() {
  const { workspace, agent, setWorkspace, setAgent } = useFilter();

  const filteredAgents = workspace
    ? agents.filter((a) => a.workspace === workspace)
    : agents;

  // Deduplicate agent names (in case an agent appears in multiple workspaces)
  const uniqueAgents = Array.from(
    new Map(filteredAgents.map((a) => [a.id, a])).values()
  );

  return (
    <div className="flex items-center gap-3 px-4 py-3 sm:px-6">
      <Select
        value={workspace ?? '__all__'}
        onValueChange={(v) => setWorkspace(v === '__all__' ? null : v)}
      >
        <SelectTrigger>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__all__">All Workspaces</SelectItem>
          {workspaces.map((ws) => (
            <SelectItem key={ws.id} value={ws.id}>
              {ws.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select
        value={agent ?? '__all__'}
        onValueChange={(v) => setAgent(v === '__all__' ? null : v)}
      >
        <SelectTrigger>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__all__">All Agents</SelectItem>
          {uniqueAgents.map((a) => (
            <SelectItem key={a.id} value={a.id}>
              {a.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
