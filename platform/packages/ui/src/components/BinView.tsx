import type { ArchivedProject } from "../types/api";
import { Card } from "./ui/card";
import { Button } from "./ui/button";
import { Badge } from "./ui/badge";
import { Trash2, ArrowLeft } from "./icons";

export interface BinViewProps {
  projects: ArchivedProject[];
  loading?: boolean;
  onRestoreProject: (id: string) => void;
  onPermanentlyDelete: (id: string) => void;
  retentionDays?: number;
}

function relativeTime(dateStr?: string): string {
  if (!dateStr) return "";
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));
  if (diffDays === 0) return "today";
  if (diffDays === 1) return "yesterday";
  return `${diffDays} days ago`;
}

function daysRemaining(archivedAt?: string, retentionDays = 30): number {
  if (!archivedAt) return retentionDays;
  const archived = new Date(archivedAt);
  const expiresAt = new Date(archived.getTime() + retentionDays * 24 * 60 * 60 * 1000);
  const now = new Date();
  return Math.max(0, Math.ceil((expiresAt.getTime() - now.getTime()) / (1000 * 60 * 60 * 24)));
}

export function BinView({
  projects,
  loading,
  onRestoreProject,
  onPermanentlyDelete,
  retentionDays = 30,
}: BinViewProps) {
  return (
    <div className="flex-1 min-h-0 overflow-auto">
      <Card className="p-6 mb-4">
        <div className="mb-2">
          <h2 className="text-xl font-semibold">Bin</h2>
          <p className="text-[13px] text-muted-foreground mt-1">
            Archived projects are kept for {retentionDays} days before permanent deletion.
            You can restore them at any time during this period.
          </p>
        </div>
      </Card>

      <Card className="p-0 overflow-hidden">
        {projects.length === 0 && !loading && (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Trash2 className="w-10 h-10 text-muted-foreground/30 mb-3" />
            <p className="text-sm text-muted-foreground">The bin is empty</p>
            <p className="text-[12px] text-muted-foreground/60 mt-1">
              Archived projects and streams will appear here
            </p>
          </div>
        )}

        {projects.length > 0 && (
          <div className="divide-y divide-border/20">
            {projects.map((project) => {
              const remaining = daysRemaining(project.archived_at, retentionDays);
              return (
                <div
                  key={project.id}
                  className="flex items-center gap-4 px-5 py-4 hover:bg-accent/30 transition-colors"
                >
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-foreground">
                        {project.name}
                      </span>
                      <Badge variant="secondary" className="text-[10px]">
                        {project.default_source_language} → {project.target_languages.join(", ")}
                      </Badge>
                    </div>
                    <div className="flex items-center gap-2 mt-1 text-[12px] text-muted-foreground/60">
                      <span>Archived {relativeTime(project.archived_at)}</span>
                      <span>·</span>
                      <span className={remaining <= 7 ? "text-destructive" : ""}>
                        {remaining} {remaining === 1 ? "day" : "days"} remaining
                      </span>
                    </div>
                  </div>

                  <div className="flex items-center gap-2 shrink-0">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => onRestoreProject(project.id)}
                    >
                      <ArrowLeft className="w-3.5 h-3.5 mr-1.5" />
                      Restore
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-destructive hover:text-destructive"
                      onClick={() => onPermanentlyDelete(project.id)}
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
        )}

        {loading && (
          <div className="flex items-center justify-center py-8 text-sm text-muted-foreground">
            Loading...
          </div>
        )}
      </Card>
    </div>
  );
}
