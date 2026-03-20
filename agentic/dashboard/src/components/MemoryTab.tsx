import { Badge } from '@/components/ui/badge';
import { useApi } from '@/context/ApiContext';

export default function MemoryTab() {
  const api = useApi();

  // Memory is not yet available from the Bowrain API.
  // Show a clean empty state rather than mock data.
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        Git log of agent-memory repo -- what agents learned per session
      </p>

      <div className="rounded-lg border px-6 py-12 text-center">
        <p className="text-sm font-medium text-muted-foreground">
          Memory not connected
        </p>
        <p className="mt-1 text-xs text-muted-foreground/60">
          {api.connected
            ? 'Agent memory tracking is not yet wired to the dashboard.'
            : 'Connect to the Bowrain API to see agent memory updates.'}
        </p>
        <div className="mt-3 flex justify-center gap-2">
          <Badge variant="outline" className="text-[10px]">
            agent-memory
          </Badge>
          <Badge variant="outline" className="text-[10px]">
            GitHub
          </Badge>
        </div>
      </div>
    </div>
  );
}
