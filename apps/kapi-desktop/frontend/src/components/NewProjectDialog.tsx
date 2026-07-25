import { useState } from "react";
import {
  Button,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  LocaleSelect,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { api } from "../hooks/useApi";
import { useLocales } from "../hooks/useLocales";

interface NewProjectDialogProps {
  onCreate: (name: string, sourceLang: string, savePath?: string) => void;
  onCancel: () => void;
  shortenHome: (path: string) => string;
}

export function NewProjectDialog({ onCreate, onCancel, shortenHome }: NewProjectDialogProps) {
  const { locales } = useLocales();
  const [name, setName] = useState("");
  const [sourceLang, setSourceLang] = useState("en");
  const [customPath, setCustomPath] = useState("");
  // eslint-disable-next-line no-control-regex -- intentional check for control characters in filenames
  const INVALID = /[<>:"/\\|?*\x00-\x1f]/;
  const trimmed = name.trim();
  const nameValid =
    trimmed.length > 0 && !INVALID.test(trimmed) && trimmed !== "." && trimmed !== "..";
  const canCreate = customPath ? true : nameValid;
  const saveDir = customPath ? customPath : nameValid ? `~/KapiProjects/${trimmed}` : "";

  const handleBrowse = async () => {
    const dir = await api.browseProjectLocation();
    if (dir) setCustomPath(shortenHome(dir));
  };

  const handleCreate = () => {
    // Pass the project folder; the backend writes the fixed kapi.yaml recipe inside it.
    if (canCreate) onCreate(trimmed, sourceLang, saveDir || undefined);
  };

  return (
    <Dialog
      open
      onOpenChange={(o) => {
        if (!o) onCancel();
      }}
    >
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>New Project</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label className="mb-1 block text-xs text-muted-foreground">
              {customPath ? t("Location") : t("Name")}
            </Label>
            <div className="flex items-center gap-1.5">
              <Input
                type="text"
                value={customPath || name}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                  if (customPath) return;
                  setName(e.target.value);
                }}
                onKeyDown={(e: React.KeyboardEvent<HTMLInputElement>) => {
                  if (e.key === "Enter") handleCreate();
                }}
                placeholder={customPath ? "" : "My App"}
                readOnly={!!customPath}
                autoFocus={!customPath}
                className={`flex-1 ${name && !nameValid && !customPath ? "border-destructive" : ""} ${customPath ? "text-muted-foreground" : ""}`}
              />
              <Button
                variant="outline"
                size="icon-sm"
                onClick={handleBrowse}
                className="shrink-0"
                aria-label="Choose location"
              >
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  width="16"
                  height="16"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.5"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <path d="m6 14 1.5-2.9A2 2 0 0 1 9.24 10H20a2 2 0 0 1 1.94 2.5l-1.54 6a2 2 0 0 1-1.95 1.5H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h3.9a2 2 0 0 1 1.69.9l.81 1.2a2 2 0 0 0 1.67.9H18a2 2 0 0 1 2 2v2" />
                </svg>
              </Button>
              {customPath && (
                <Button
                  variant="outline"
                  size="icon-sm"
                  onClick={() => setCustomPath("")}
                  className="shrink-0"
                  aria-label="Clear location"
                >
                  <svg
                    xmlns="http://www.w3.org/2000/svg"
                    width="16"
                    height="16"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="1.5"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  >
                    <path d="M18 6 6 18" />
                    <path d="m6 6 12 12" />
                  </svg>
                </Button>
              )}
            </div>
            {customPath ? (
              <p className="mt-1 text-xs">&nbsp;</p>
            ) : name && !nameValid ? (
              <p className="mt-1 text-xs text-destructive">Invalid directory name</p>
            ) : saveDir ? (
              <p className="mt-1 text-xs text-muted-foreground">{saveDir}</p>
            ) : (
              <p className="mt-1 text-xs">&nbsp;</p>
            )}
          </div>
          <div>
            <Label className="mb-1 block text-xs text-muted-foreground">
              {t("Source language")}
            </Label>
            <LocaleSelect
              value={sourceLang}
              onChange={setSourceLang}
              locales={locales}
              placeholder={t("Select source language...")}
            />
          </div>
          <div className="flex gap-2">
            <Button onClick={handleCreate} disabled={!canCreate} className="flex-1">
              Create Project
            </Button>
            <Button variant="outline" onClick={onCancel}>
              Cancel
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
