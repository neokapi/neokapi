import { useState, useEffect, useCallback } from "react";
import { FolderOpen, FilePlus, Workflow, Wrench, Puzzle, Settings } from "lucide-react";
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
    api.listRecentFiles().then((files) => {
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
        setError(`Failed to open ${path}: ${e}`);
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
          <button
            onClick={onSettings}
            className="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}
            aria-label="Settings"
          >
            <Settings size={16} />
          </button>
        )}
      </div>

      <div className="flex-1 overflow-auto">
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
                  <label className="mb-1 block text-left text-xs text-muted-foreground">
                    {customPath ? "Location" : "Name"}
                  </label>
                  <div className="flex items-center gap-1.5">
                    <input
                      type="text"
                      value={customPath || newName}
                      onChange={(e) => {
                        if (customPath) return;
                        setNewName(e.target.value);
                      }}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" && canCreate) handleCreateProject();
                      }}
                      placeholder={customPath ? "" : "My App"}
                      readOnly={!!customPath}
                      autoFocus={!customPath}
                      className={`flex-1 rounded-lg border bg-transparent px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring ${
                        newName && !nameValid && !customPath
                          ? "border-destructive"
                          : "border-input"
                      } ${customPath ? "text-muted-foreground" : ""}`}
                    />
                    <button
                      onClick={handleBrowse}
                      className="shrink-0 rounded-lg border border-border p-2 text-muted-foreground hover:bg-accent hover:text-foreground"
                      aria-label="Browse for location"
                      title="Choose location"
                    >
                      <FolderOpen size={16} />
                    </button>
                    {customPath && (
                      <button
                        onClick={() => setCustomPath("")}
                        className="shrink-0 rounded-lg border border-border p-2 text-muted-foreground hover:bg-accent hover:text-foreground"
                        aria-label="Clear location"
                        title="Clear location"
                      >
                        <span className="text-sm">✕</span>
                      </button>
                    )}
                  </div>
                  {customPath ? (
                    <p className="mt-1 text-left text-xs">&nbsp;</p>
                  ) : newName && !nameValid ? (
                    <p className="mt-1 text-left text-xs text-destructive">
                      Invalid directory name
                    </p>
                  ) : savePath ? (
                    <p className="mt-1 text-left text-xs text-muted-foreground">
                      {savePath}
                    </p>
                  ) : (
                    <p className="mt-1 text-left text-xs">&nbsp;</p>
                  )}
                </div>
                <div className="flex gap-2">
                  <button
                    onClick={handleCreateProject}
                    disabled={!canCreate || creating}
                    className="flex-1 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                  >
                    {creating ? "Creating..." : "Create Project"}
                  </button>
                  <button
                    onClick={() => setShowNewForm(false)}
                    className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            ) : (
              <div className="flex gap-3">
                <button
                  onClick={handleNew}
                  className="flex items-center gap-2 rounded-lg bg-primary px-6 py-2.5 text-sm font-medium text-primary-foreground shadow-sm transition-colors hover:bg-primary/90"
                >
                  <FilePlus size={16} />
                  New Project
                </button>
                <button
                  onClick={handleOpen}
                  className="flex items-center gap-2 rounded-lg border border-border bg-background px-6 py-2.5 text-sm font-medium shadow-sm transition-colors hover:bg-accent"
                  aria-label="Open a Kapi project"
                >
                  <FolderOpen size={16} />
                  Open a Kapi project
                </button>
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
                <button
                  key={item.title}
                  onClick={handleNew}
                  className="group rounded-xl border border-border p-4 text-left transition-colors hover:border-primary/30 hover:bg-accent/30"
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
                </button>
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
                  <button
                    key={file.path}
                    onClick={() => handleOpenRecent(file.path)}
                    className="flex w-full items-center gap-3 rounded-lg border border-border p-3 text-left transition-colors hover:bg-accent/30"
                    aria-label={`Open ${file.name}`}
                  >
                    <FolderOpen size={16} className="shrink-0 text-muted-foreground" />
                    <div className="flex-1 truncate">
                      <div className="text-sm font-medium">{file.name}</div>
                      <div className="truncate text-xs text-muted-foreground">{file.path}</div>
                    </div>
                  </button>
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
      </div>
    </div>
  );
}
