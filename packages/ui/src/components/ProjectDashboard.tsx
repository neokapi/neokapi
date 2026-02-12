import { useState } from "react";
import type { ProjectInfo } from "../types/api";
import { useLocales } from "../hooks/useLocales";
import { LocaleSelect, MultiLocaleSelect } from "./LocaleSelect";
import { Button } from "./ui/button";
import { Card, CardContent, GlassCard } from "./ui/card";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { FolderOpen, ArrowRight } from "./icons";

interface ProjectDashboardProps {
  projects: ProjectInfo[];
  onCreateProject: (name: string, sourceLang: string, targetLangs: string[]) => void;
  onOpenProject: (project: ProjectInfo) => void;
  /** Optional handler for "Open a Project" button (e.g. file dialog in desktop apps). */
  onOpenKaz?: () => void;
}

export function ProjectDashboard({
  projects,
  onCreateProject,
  onOpenProject,
  onOpenKaz,
}: ProjectDashboardProps) {
  const { getDisplayName } = useLocales();
  const [showCreate, setShowCreate] = useState(false);
  const [name, setName] = useState("");
  const [sourceLang, setSourceLang] = useState("en");
  const [targetLangsList, setTargetLangsList] = useState<string[]>(["fr"]);

  const handleCreate = () => {
    if (!name.trim()) return;
    if (targetLangsList.length === 0) return;
    onCreateProject(name.trim(), sourceLang, targetLangsList);
    setShowCreate(false);
    setName("");
    setTargetLangsList(["fr"]);
  };

  return (
    <div>
      <div className="flex justify-between items-center mb-6">
        <h2 className="text-xl font-semibold">Translation Projects</h2>
        <div className="flex gap-2">
          {onOpenKaz && (
            <Button variant="outline" onClick={onOpenKaz} data-testid="open-kaz-btn">
              Open a Project
            </Button>
          )}
          <Button onClick={() => setShowCreate(true)} data-testid="new-project-btn">
            New Project
          </Button>
        </div>
      </div>

      {showCreate && (
        <GlassCard intensity="subtle" className="mb-6" data-testid="create-project-dialog">
          <CardContent className="pt-6">
            <h3 className="text-base font-semibold mb-4">Create Translation Project</h3>
            <div className="flex flex-col gap-3">
              <div className="flex flex-col gap-1">
                <Label className="text-muted-foreground">Project Name</Label>
                <Input
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="My Translation Project"
                  data-testid="project-name-input"
                  autoFocus
                />
              </div>
              <div className="flex gap-3">
                <div className="flex flex-col gap-1 flex-1">
                  <Label className="text-muted-foreground">Source Language</Label>
                  <LocaleSelect
                    value={sourceLang}
                    onChange={setSourceLang}
                    data-testid="source-lang-input"
                  />
                </div>
                <div className="flex flex-col gap-1 flex-1">
                  <Label className="text-muted-foreground">Target Languages</Label>
                  <MultiLocaleSelect
                    value={targetLangsList}
                    onChange={setTargetLangsList}
                    data-testid="target-langs-input"
                  />
                </div>
              </div>
              <div className="flex gap-2 justify-end">
                <Button variant="outline" onClick={() => setShowCreate(false)}>
                  Cancel
                </Button>
                <Button onClick={handleCreate} data-testid="create-project-submit">
                  Create
                </Button>
              </div>
            </div>
          </CardContent>
        </GlassCard>
      )}

      {projects.length === 0 && !showCreate && (
        <div className="flex flex-col items-center justify-center p-12 bg-card rounded-lg border border-dashed border-border backdrop-blur-sm" data-testid="empty-projects">
          <FolderOpen className="w-12 h-12 mb-4 text-muted-foreground opacity-30" />
          <p className="text-muted-foreground">
            No projects yet. Create a new project to get started.
          </p>
        </div>
      )}

      <div className="grid grid-cols-[repeat(auto-fill,minmax(300px,1fr))] gap-4">
        {projects.map((p) => (
          <GlassCard
            key={p.id}
            intensity="medium"
            hover
            glow="violet"
            onClick={() => onOpenProject(p)}
            className="cursor-pointer transition-all"
            data-testid={`project-card-${p.id}`}
          >
            <CardContent className="pt-4">
              <h3 className="font-semibold text-base mb-2">{p.name}</h3>
              <div className="text-[13px] text-muted-foreground mb-2">
                {getDisplayName(p.source_locale)} <ArrowRight className="w-3 h-3 inline-block" /> {p.target_locales.map(l => getDisplayName(l)).join(", ")}
              </div>
              <div className="text-xs text-muted-foreground">
                {(p.items?.length ?? 0)} file{(p.items?.length ?? 0) !== 1 ? "s" : ""}
              </div>
            </CardContent>
          </GlassCard>
        ))}
      </div>
    </div>
  );
}
