import { useState, useEffect } from "react";
import type { ProjectInfo } from "../types/api";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "./ui/dialog";
import { LocaleSelect, MultiLocaleSelect } from "./LocaleSelect";

/** Common languages used as fallback when workspace has no languages configured. */
const DEFAULT_LANGUAGES = [
  "en", "fr", "de", "es", "it", "pt", "nl", "ja", "ko", "zh",
  "ar", "ru", "sv", "nb", "da", "fi", "pl", "cs", "tr", "th",
];

export interface ProjectFormData {
  name: string;
  default_source_language: string;
  target_languages: string[];
  target_language_mode: string;
}

export interface ProjectFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (data: ProjectFormData) => void;
  /** When set, the dialog operates in edit mode with pre-populated fields. */
  editProject?: ProjectInfo;
  /** Workspace languages — when set, locale pickers are restricted to these. */
  workspaceLanguages?: string[];
}

export function ProjectFormDialog({
  open,
  onOpenChange,
  onSubmit,
  editProject,
  workspaceLanguages,
}: ProjectFormDialogProps) {
  const [name, setName] = useState("");
  const [sourceLang, setSourceLang] = useState("en");
  const [targetLangs, setTargetLangs] = useState<string[]>([]);
  const [targetMode, setTargetMode] = useState<"defined" | "open">("defined");

  const isEdit = !!editProject;
  const effectiveLangs = workspaceLanguages && workspaceLanguages.length > 0
    ? workspaceLanguages
    : DEFAULT_LANGUAGES;

  useEffect(() => {
    if (open && editProject) {
      setName(editProject.name);
      setSourceLang(editProject.default_source_language);
      setTargetLangs([...editProject.target_languages]);
      setTargetMode((editProject.target_language_mode as "defined" | "open") || "defined");
    } else if (open && !editProject) {
      // Set sensible defaults from workspace languages
      if (effectiveLangs.length > 0) {
        setSourceLang(effectiveLangs[0]);
        setTargetLangs(effectiveLangs.length > 1 ? [effectiveLangs[1]] : []);
      }
    }
  }, [open, editProject, effectiveLangs.length > 0, workspaceLanguages]);

  const handleSubmit = () => {
    if (!name.trim()) return;
    if (targetMode === "defined" && targetLangs.length === 0) return;
    onSubmit({
      name: name.trim(),
      default_source_language: sourceLang,
      target_languages: targetLangs,
      target_language_mode: targetMode,
    });
  };

  const handleOpenChange = (v: boolean) => {
    if (!v) {
      setName("");
      setSourceLang(effectiveLangs.length > 0 ? effectiveLangs[0] : "en");
      setTargetLangs([]);
      setTargetMode("defined");
    }
    onOpenChange(v);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        size="md"
        data-testid={isEdit ? "edit-project-dialog" : "create-project-dialog"}
        onInteractOutside={(e: Event) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>{isEdit ? "Edit Project" : "Create Project"}</DialogTitle>
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

          <div>
            <Label className="text-muted-foreground">Default Source Language</Label>
            <p className="text-[11px] text-muted-foreground mb-1">
              The primary language of source content. Individual items can override this.
            </p>
            {isEdit ? (
              <div className="px-3 py-2 rounded-md border border-border/50 text-sm text-muted-foreground bg-muted/30">
                {sourceLang}
              </div>
            ) : (
              <LocaleSelect
                value={sourceLang}
                onChange={setSourceLang}
                codes={effectiveLangs}
                data-testid="source-lang-input"
              />
            )}
          </div>

          <div>
            <Label className="text-muted-foreground">Target Languages</Label>
            <div className="flex gap-2 mt-1 mb-2">
              <button
                type="button"
                onClick={() => setTargetMode("defined")}
                className={`
                  px-3 py-1.5 rounded-md text-[12px] font-medium border cursor-pointer
                  transition-colors bg-transparent
                  ${targetMode === "defined"
                    ? "border-primary/50 bg-primary/5 text-foreground ring-1 ring-primary/20"
                    : "border-border/50 text-muted-foreground hover:text-foreground hover:border-border"
                  }
                `}
              >
                Defined list
              </button>
              <button
                type="button"
                onClick={() => setTargetMode("open")}
                className={`
                  px-3 py-1.5 rounded-md text-[12px] font-medium border cursor-pointer
                  transition-colors bg-transparent
                  ${targetMode === "open"
                    ? "border-primary/50 bg-primary/5 text-foreground ring-1 ring-primary/20"
                    : "border-border/50 text-muted-foreground hover:text-foreground hover:border-border"
                  }
                `}
              >
                Open contributions
              </button>
            </div>
            <p className="text-[11px] text-muted-foreground mb-2">
              {targetMode === "defined"
                ? "Only these languages can be translated to."
                : "Contributors can add new languages. New languages are auto-added to the list."
              }
            </p>
            <MultiLocaleSelect
              value={targetLangs}
              onChange={setTargetLangs}
              codes={effectiveLangs}
              data-testid="target-langs-input"
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={!name.trim() || (targetMode === "defined" && targetLangs.length === 0)}
            data-testid={isEdit ? "edit-project-submit" : "create-project-submit"}
          >
            {isEdit ? "Save" : "Create"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
