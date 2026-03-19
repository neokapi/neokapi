import { Search, X, ExternalLink } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useFilter } from '@/context/FilterContext';
import { workspaces } from '@/data/workspaces';
import { agents } from '@/data/agents';

const presets = [
  { id: 'all', label: 'All' },
  { id: 'active', label: 'Active Jobs' },
  { id: 'failed', label: 'Failed Jobs' },
  { id: 'today', label: 'Today' },
] as const;

export default function FilterBar() {
  const {
    workspace,
    agent,
    status,
    search,
    preset,
    setWorkspace,
    setAgent,
    setStatus,
    setSearch,
    setPreset,
    clearFilters,
  } = useFilter();

  const filteredAgents = workspace
    ? agents.filter((a) => a.workspace === workspace)
    : agents;

  const uniqueAgents = Array.from(
    new Map(filteredAgents.map((a) => [a.id, a])).values()
  );

  const activeWs = workspaces.find((w) => w.id === workspace);
  const bowrainUrl = activeWs
    ? `https://dev.bowrain.cloud/${activeWs.slug}`
    : 'https://dev.bowrain.cloud';

  const hasActiveFilters = workspace || agent || status || search || preset;

  function handlePreset(id: string) {
    if (preset === id) {
      setPreset(null);
      setStatus(null);
      return;
    }
    setPreset(id);
    switch (id) {
      case 'all':
        clearFilters();
        break;
      case 'active':
        setStatus('running');
        break;
      case 'failed':
        setStatus('failed');
        break;
      case 'today':
        setStatus(null);
        break;
    }
  }

  // Active filter tokens for display
  const tokens: { key: string; value: string; onRemove: () => void }[] = [];
  if (workspace) {
    const ws = workspaces.find((w) => w.id === workspace);
    tokens.push({
      key: 'workspace',
      value: ws?.name ?? workspace,
      onRemove: () => setWorkspace(null),
    });
  }
  if (agent) {
    const ag = agents.find((a) => a.id === agent);
    tokens.push({
      key: 'agent',
      value: ag?.name ?? agent,
      onRemove: () => setAgent(null),
    });
  }
  if (status) {
    tokens.push({
      key: 'status',
      value: status,
      onRemove: () => setStatus(null),
    });
  }

  return (
    <div className="space-y-2 px-4 py-3 sm:px-6">
      {/* Row 1: Filters + Search + Bowrain link */}
      <div className="flex flex-wrap items-center gap-2">
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
              <SelectItem
                key={ws.id}
                value={ws.id}
                disabled={ws.status === 'idle'}
              >
                {ws.name}
                {ws.status === 'idle' ? ' (coming soon)' : ''}
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

        <Select
          value={status ?? '__all__'}
          onValueChange={(v) => setStatus(v === '__all__' ? null : v)}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">All Statuses</SelectItem>
            <SelectItem value="succeeded">Succeeded</SelectItem>
            <SelectItem value="failed">Failed</SelectItem>
            <SelectItem value="running">Running</SelectItem>
          </SelectContent>
        </Select>

        {/* Search */}
        <div className="relative flex-1 min-w-[180px]">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search summaries..."
            className="h-8 w-full rounded-lg border border-input bg-transparent pl-8 pr-3 text-sm outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50"
          />
        </div>

        {/* Bowrain link */}
        <Button
          variant="ghost"
          size="sm"
          render={
            <a
              href={bowrainUrl}
              target="_blank"
              rel="noopener noreferrer"
            />
          }
          className="ml-auto gap-1.5 text-muted-foreground hover:text-foreground"
        >
          View in Bowrain
          <ExternalLink className="h-3.5 w-3.5" />
        </Button>
      </div>

      {/* Row 2: Presets + Active filter tokens */}
      <div className="flex flex-wrap items-center gap-2">
        {presets.map((p) => (
          <Button
            key={p.id}
            variant={preset === p.id ? 'default' : 'outline'}
            size="xs"
            onClick={() => handlePreset(p.id)}
          >
            {p.label}
          </Button>
        ))}

        {tokens.length > 0 && (
          <div className="ml-2 flex flex-wrap items-center gap-1">
            {tokens.map((token) => (
              <Badge
                key={token.key}
                variant="secondary"
                className="gap-1 pl-1.5 pr-1 py-0 text-[11px] font-mono"
              >
                <span className="text-muted-foreground">{token.key}:</span>
                <span>{token.value}</span>
                <button
                  onClick={token.onRemove}
                  className="ml-0.5 p-0 bg-transparent border-none cursor-pointer text-muted-foreground hover:text-foreground transition-colors"
                >
                  <X className="w-3 h-3" />
                </button>
              </Badge>
            ))}
            {hasActiveFilters && (
              <button
                onClick={clearFilters}
                className="ml-1 text-[11px] text-muted-foreground hover:text-foreground bg-transparent border-none cursor-pointer"
              >
                Clear all
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
