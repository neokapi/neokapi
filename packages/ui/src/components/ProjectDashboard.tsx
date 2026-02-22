import { useState } from "react";
import type { ProjectInfo } from "../types/api";
import { useLocales } from "../hooks/useLocales";
import { LocaleSelect, MultiLocaleSelect } from "./LocaleSelect";
import { Button } from "./ui/button";
import { CardContent, GlassCard } from "./ui/card";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from "./ui/dialog";
import { FolderOpen, ArrowRight } from "./icons";

interface ProjectDashboardProps {
  projects: ProjectInfo[];
  onCreateProject: (name: string, sourceLang: string, targetLangs: string[]) => void;
  onOpenProject: (project: ProjectInfo) => void;
}

export function ProjectDashboard({
  projects,
  onCreateProject,
  onOpenProject,
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

  const handleOpenChange = (open: boolean) => {
    if (!open) {
      setName("");
      setSourceLang("en");
      setTargetLangsList(["fr"]);
    }
    setShowCreate(open);
  };

  return (
    <>
      <GlassCard intensity="subtle" className="p-6">
        <div className="flex justify-between items-center mb-6">
          <div>
            <h2 className="text-xl font-semibold">Translation Projects</h2>
            <p className="text-[13px] text-muted-foreground mt-1">{projects.length} project{projects.length !== 1 ? "s" : ""}</p>
          </div>
          <Button onClick={() => setShowCreate(true)} data-testid="new-project-btn">
            New Project
          </Button>
        </div>

        {projects.length === 0 && (
          <div className="flex flex-col items-center text-center py-12" data-testid="empty-projects">
            <FolderOpen className="w-14 h-14 mb-5 text-primary opacity-40" />
            <h3 className="text-lg font-semibold mb-2">No projects yet</h3>
            <p className="text-sm text-muted-foreground mb-6">
              Create your first translation project to start localizing your content.
            </p>
            <Button onClick={() => setShowCreate(true)}>
              New Project
            </Button>
          </div>
        )}

        {projects.length > 0 && (
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
        )}
      </GlassCard>

      <Dialog open={showCreate} onOpenChange={handleOpenChange}>
        <DialogContent className="sm:max-w-[520px]" data-testid="create-project-dialog" onInteractOutside={(e: Event) => e.preventDefault()}>
          <DialogHeader>
            <DialogTitle>Create Translation Project</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-4 py-2">
            <div>
              <Label className="text-muted-foreground">Project Name</Label>
              <Input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="My Translation Project"
                data-testid="project-name-input"
                autoFocus
                className="mt-1"
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
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => handleOpenChange(false)}>
              Cancel
            </Button>
            <Button onClick={handleCreate} disabled={!name.trim() || targetLangsList.length === 0} data-testid="create-project-submit">
              Create
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
