import { useState, useEffect } from "react";
import type { ProjectInfo } from "../types/api";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "./ui/dialog";
import { LocaleSelect, MultiLocaleSelect } from "./LocaleSelect";

export interface ProjectFormData {
  name: string;
  source_locale: string;
  target_locales: string[];
}

export interface ProjectFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (data: ProjectFormData) => void;
  /** When set, the dialog operates in edit mode with pre-populated fields. */
  editProject?: ProjectInfo;
}

export function ProjectFormDialog({ open, onOpenChange, onSubmit, editProject }: ProjectFormDialogProps) {
  const [name, setName] = useState("");
  const [sourceLang, setSourceLang] = useState("en");
  const [targetLangs, setTargetLangs] = useState<string[]>(["fr"]);

  const isEdit = !!editProject;

  useEffect(() => {
    if (open && editProject) {
      setName(editProject.name);
      setSourceLang(editProject.source_locale);
      setTargetLangs([...editProject.target_locales]);
    }
  }, [open, editProject]);

  const handleSubmit = () => {
    if (!name.trim() || targetLangs.length === 0) return;
    onSubmit({
      name: name.trim(),
      source_locale: sourceLang,
      target_locales: targetLangs,
    });
  };

  const handleOpenChange = (v: boolean) => {
    if (!v) {
      setName("");
      setSourceLang("en");
      setTargetLangs(["fr"]);
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
          <DialogTitle>{isEdit ? "Edit Project" : "Create Translation Project"}</DialogTitle>
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
              {isEdit ? (
                <>
                  <div className="px-3 py-2 rounded-md border border-border/50 text-sm text-muted-foreground bg-muted/30">
                    {sourceLang}
                  </div>
                  <p className="text-[10px] text-muted-foreground mt-0.5">
                    Source language cannot be changed
                  </p>
                </>
              ) : (
                <LocaleSelect
                  value={sourceLang}
                  onChange={setSourceLang}
                  data-testid="source-lang-input"
                />
              )}
            </div>
            <div className="flex flex-col gap-1 flex-1">
              <Label className="text-muted-foreground">Target Languages</Label>
              <MultiLocaleSelect
                value={targetLangs}
                onChange={setTargetLangs}
                data-testid="target-langs-input"
              />
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={!name.trim() || targetLangs.length === 0}
            data-testid={isEdit ? "edit-project-submit" : "create-project-submit"}
          >
            {isEdit ? "Save" : "Create"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
