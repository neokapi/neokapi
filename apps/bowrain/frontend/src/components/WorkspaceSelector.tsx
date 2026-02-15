import { useState, useEffect } from "react";
import { Button, cn } from "@gokapi/ui";
import { Loader2, Building2, ArrowLeft } from "lucide-react";

export interface WorkspaceOption {
  id: string;
  slug: string;
  name: string;
  description: string;
  role: string;
}

interface WorkspaceSelectorProps {
  userName?: string;
  onSelect: (ws: WorkspaceOption) => void;
  onBack: () => void;
  getWorkspaces: () => Promise<WorkspaceOption[]>;
}

export function WorkspaceSelector({ userName, onSelect, onBack, getWorkspaces }: WorkspaceSelectorProps) {
  const [workspaces, setWorkspaces] = useState<WorkspaceOption[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getWorkspaces()
      .then(setWorkspaces)
      .catch((e) => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false));
  }, [getWorkspaces]);

  return (
    <div className="flex flex-col items-center justify-center h-full gap-8">
      <div className="text-center">
        <Building2 className="w-12 h-12 text-primary mx-auto mb-4" />
        <h2 className="text-2xl font-semibold mb-2">
          {userName ? `Welcome, ${userName}` : "Select Workspace"}
        </h2>
        <p className="text-muted-foreground text-sm">
          Choose a workspace to start working on translations.
        </p>
      </div>

      <div className="w-full max-w-lg space-y-3">
        {loading && (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="w-6 h-6 animate-spin text-muted-foreground" />
          </div>
        )}

        {error && (
          <p className="text-sm text-destructive text-center">{error}</p>
        )}

        {!loading && workspaces.length === 0 && !error && (
          <p className="text-sm text-muted-foreground text-center py-4">
            No workspaces available. Contact your administrator.
          </p>
        )}

        {workspaces.map((ws) => (
          <button
            key={ws.slug}
            onClick={() => onSelect(ws)}
            className={cn(
              "w-full text-left p-4 rounded-lg border border-border",
              "hover:bg-accent hover:border-primary/30 transition-colors",
              "focus:outline-none focus:ring-2 focus:ring-ring",
            )}
          >
            <div className="flex items-center justify-between">
              <div>
                <h3 className="font-medium">{ws.name}</h3>
                {ws.description && (
                  <p className="text-sm text-muted-foreground mt-1">{ws.description}</p>
                )}
              </div>
              <span className="text-xs text-muted-foreground capitalize bg-muted px-2 py-1 rounded">
                {ws.role}
              </span>
            </div>
          </button>
        ))}

        <div className="text-center pt-4">
          <Button variant="ghost" size="sm" onClick={onBack}>
            <ArrowLeft className="w-4 h-4 mr-2" />
            Back
          </Button>
        </div>
      </div>
    </div>
  );
}
