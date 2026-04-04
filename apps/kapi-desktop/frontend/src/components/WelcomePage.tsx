import { useState, useEffect, useCallback } from "react";
import { FolderOpen, FilePlus, Workflow, Wrench, Puzzle, Settings } from "lucide-react";
import { Button, Label, Input, ScrollArea } from "@neokapi/ui-primitives";
import type { KapiProject, TabInfo } from "../types/api";
import { api } from "../hooks/useApi";
import { useShortenHome } from "../hooks/useShortenHome";

interface WelcomePageProps {
  onOpen: (tab: TabInfo) => void;
  onNew: (project: KapiProject, savePath?: string) => void;
  onSettings?: () => void;
}

const GET_STARTED = [
  {
    icon: <FilePlus size={18} />,
    title: "Create a project",
    description:
      "Define source and target languages, map your content files, and save as a portable Kapi project.",
  },
  {
    icon: <Workflow size={18} />,
    title: "Build a flow",
    description:
      "Chain tools into reusable pipelines — AI translation, quality checks, pseudo-translation, and more.",
  },
  {
    icon: <Wrench size={18} />,
    title: "Run tools",
    description:
      "Translate files with AI, pseudo-translate for testing, run QA checks, leverage TM — all with live progress.",
  },
  {
    icon: <Puzzle size={18} />,
    title: "Add plugins",
    description:
      "Install the Okapi Bridge for plugging into Okapi's filters and steps, or browse the registry.",
  },
];

// Characters not allowed in directory names.
// eslint-disable-next-line no-control-regex -- intentional check for control characters in filenames
const INVALID_DIR_CHARS = /[<>:"/\\|?*\x00-\x1f]/;

function isValidDirName(name: string): boolean {
  if (!name.trim()) return false;
  if (INVALID_DIR_CHARS.test(name)) return false;
  if (name === "." || name === "..") return false;
  return true;
}

export function WelcomePage({ onOpen, onNew, onSettings }: WelcomePageProps) {
  const shortenHome = useShortenHome();
  const [error, setError] = useState<string | null>(null);
  const [showNewForm, setShowNewForm] = useState(false);
  const [newName, setNewName] = useState("");
  const [customPath, setCustomPath] = useState("");
  const [creating, setCreating] = useState(false);
  const [recentFiles, setRecentFiles] = useState<
    Array<{ path: string; name: string; opened_at: string }>
  >([]);

  useEffect(() => {
    void api.listRecentFiles().then((files) => {
      if (files) setRecentFiles(files);
    });
  }, []);

  const nameValid = isValidDirName(newName);
  const canCreate = customPath ? true : nameValid;
  const savePath = customPath || (nameValid ? `~/KapiProjects/${newName.trim()}` : "");

  const handleNew = useCallback(() => {
    setShowNewForm(true);
    setNewName("");
    setCustomPath("");
    setError(null);
  }, []);

  const handleBrowse = useCallback(async () => {
    try {
      const dir = await api.browseProjectLocation();
      if (dir) {
        setCustomPath(shortenHome(dir));
      }
    } catch (e) {
      setError(String(e));
    }
  }, []);

  const handleCreateProject = useCallback(async () => {
    if (!canCreate) return;
    setCreating(true);
    setError(null);
    try {
      const proj: KapiProject = {
        version: "v1",
        name: "",
        source_language: "en-US",
        target_languages: [],
        flows: {},
      };
      const path = savePath ? `${savePath}/project.kapi` : undefined;
      onNew(proj, path);
      setShowNewForm(false);
    } catch (e) {
      setError(String(e));
    } finally {
      setCreating(false);
    }
  }, [canCreate, savePath, onNew]);

  const handleOpen = useCallback(async () => {
    setError(null);
    try {
      const tab = await api.openProjectDialog();
      if (tab) onOpen(tab);
    } catch (e) {
      setError(String(e));
    }
  }, [onOpen]);

  const handleOpenRecent = useCallback(
    async (path: string) => {
      setError(null);
      try {
        const tab = await api.openProject(path);
        if (tab) onOpen(tab);
      } catch (e) {
        setError(`Failed to open ${path}: ${String(e)}`);
      }
    },
    [onOpen],
  );

  return (
    <div className="flex h-screen flex-col bg-background">
      <div
        className="fixed inset-x-0 top-0 z-10 flex h-12 items-center justify-end px-4"
        style={{ WebkitAppRegion: "drag" } as React.CSSProperties}
      >
        {onSettings && (
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onSettings}
            style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}
            aria-label="Settings"
          >
            <Settings size={16} />
          </Button>
        )}
      </div>

      <ScrollArea className="flex-1">
        <div className="mx-auto max-w-3xl px-8 pb-12 pt-20">
          {/* Hero */}
          <div className="mb-12 flex flex-col items-center text-center">
            <img src="/neokapi-logo.png" alt="neokapi" className="mb-4 h-24 w-24 drop-shadow-lg" />
            <h1 className="mb-1 text-3xl font-bold tracking-tight">Kapi</h1>
            <p className="mb-6 max-w-md text-base text-muted-foreground">
              Localization plumbing and glue for people, elves, and agents.
            </p>

            {showNewForm ? (
              <div className="w-full max-w-sm space-y-3">
                <div>
                  <Label className="mb-1 block text-left text-xs text-muted-foreground">
                    {customPath ? "Location" : "Name"}
                  </Label>
                  <div className="flex items-center gap-1.5">
                    <Input
                      type="text"
                      value={customPath || newName}
                      onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                        if (customPath) return;
                        setNewName(e.target.value);
                      }}
                      onKeyDown={(e: React.KeyboardEvent<HTMLInputElement>) => {
                        if (e.key === "Enter" && canCreate) void handleCreateProject();
                      }}
                      placeholder={customPath ? "" : "My App"}
                      readOnly={!!customPath}
                      autoFocus={!customPath}
                      className={`flex-1 ${
                        newName && !nameValid && !customPath ? "border-destructive" : ""
                      } ${customPath ? "text-muted-foreground" : ""}`}
                    />
                    <Button
                      variant="outline"
                      size="icon-sm"
                      onClick={handleBrowse}
                      className="shrink-0"
                      aria-label="Browse for location"
                      title="Choose location"
                    >
                      <FolderOpen size={16} />
                    </Button>
                    {customPath && (
                      <Button
                        variant="outline"
                        size="icon-sm"
                        onClick={() => setCustomPath("")}
                        className="shrink-0"
                        aria-label="Clear location"
                        title="Clear location"
                      >
                        <span className="text-sm">✕</span>
                      </Button>
                    )}
                  </div>
                  {customPath ? (
                    <p className="mt-1 text-left text-xs">&nbsp;</p>
                  ) : newName && !nameValid ? (
                    <p className="mt-1 text-left text-xs text-destructive">
                      Invalid directory name
                    </p>
                  ) : savePath ? (
                    <p className="mt-1 text-left text-xs text-muted-foreground">{savePath}</p>
                  ) : (
                    <p className="mt-1 text-left text-xs">&nbsp;</p>
                  )}
                </div>
                <div className="flex gap-2">
                  <Button
                    onClick={handleCreateProject}
                    disabled={!canCreate || creating}
                    className="flex-1"
                  >
                    {creating ? "Creating..." : "Create Project"}
                  </Button>
                  <Button variant="outline" onClick={() => setShowNewForm(false)}>
                    Cancel
                  </Button>
                </div>
              </div>
            ) : (
              <div className="flex gap-3">
                <Button onClick={handleNew} className="gap-2 px-6 shadow-sm">
                  <FilePlus size={16} />
                  New Project
                </Button>
                <Button
                  variant="outline"
                  onClick={handleOpen}
                  className="gap-2 px-6 shadow-sm"
                  aria-label="Open a Kapi project"
                >
                  <FolderOpen size={16} />
                  Open a Kapi project
                </Button>
              </div>
            )}

            {error && (
              <p className="mt-3 text-sm text-destructive" role="alert">
                {error}
              </p>
            )}
          </div>

          {/* Get started */}
          <section className="mb-12">
            <h2 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
              Get Started
            </h2>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              {GET_STARTED.map((item) => (
                <Button
                  key={item.title}
                  variant="outline"
                  onClick={handleNew}
                  className="group h-auto whitespace-normal rounded-xl p-4 text-left flex-col items-start hover:border-primary/30 hover:bg-accent/30"
                  aria-label={item.title}
                >
                  <div className="mb-2 flex items-center gap-2.5">
                    <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                      {item.icon}
                    </div>
                    <h3 className="text-sm font-medium">{item.title}</h3>
                  </div>
                  <p className="text-xs leading-relaxed text-muted-foreground">
                    {item.description}
                  </p>
                </Button>
              ))}
            </div>
          </section>

          {/* Recent projects */}
          <section>
            <h2 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
              Recent Projects
            </h2>
            {recentFiles.length > 0 ? (
              <div className="space-y-1">
                {recentFiles.map((file) => (
                  <Button
                    key={file.path}
                    variant="outline"
                    onClick={() => handleOpenRecent(file.path)}
                    className="flex w-full h-auto items-center gap-3 rounded-lg p-3 text-left hover:bg-accent/30"
                    aria-label={`Open ${file.name}`}
                  >
                    <FolderOpen size={16} className="shrink-0 text-muted-foreground" />
                    <div className="flex-1 truncate">
                      <div className="text-sm font-medium">{file.name}</div>
                      <div className="truncate text-xs text-muted-foreground">{file.path}</div>
                    </div>
                  </Button>
                ))}
              </div>
            ) : (
              <div className="rounded-xl border border-dashed border-border p-8 text-center">
                <FolderOpen size={24} className="mx-auto mb-2 text-muted-foreground/50" />
                <p className="text-sm text-muted-foreground">
                  No recent projects yet. Create a new project or open an existing Kapi project to
                  get started.
                </p>
              </div>
            )}
          </section>

          <footer className="mt-12 border-t border-border pt-4 text-center">
            <p className="text-xs text-muted-foreground">
              Kapi is part of <span className="font-medium text-foreground">neokapi</span>, the
              open-source localization framework.
            </p>
          </footer>
        </div>
      </ScrollArea>
    </div>
  );
}
