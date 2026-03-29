import { useState, useEffect, useCallback } from "react";
import {
  FolderOpen,
  FilePlus,
  Workflow,
  Languages,
  Wrench,
  Puzzle,
  ArrowRight,
} from "lucide-react";
import type { KapiProject } from "../types/api";
import { api } from "../hooks/useApi";

interface WelcomePageProps {
  onOpen: (project: KapiProject, path: string) => void;
  onNew: (project: KapiProject) => void;
}

const GETTING_STARTED_STEPS = [
  {
    icon: <FilePlus size={18} />,
    title: "Create a project",
    description:
      "Start a new .kapi project file to organize your localization workflow. Define source and target languages, pick a framework preset, and map your content files.",
  },
  {
    icon: <Workflow size={18} />,
    title: "Build a flow",
    description:
      "Compose tools into reusable pipelines. Chain AI translation, quality checks, pseudo-translation, and more into flows you can run with one click.",
  },
  {
    icon: <Wrench size={18} />,
    title: "Run tools",
    description:
      "Execute individual tools or full flows on your files. Watch progress in real time with per-file status and live block counts.",
  },
  {
    icon: <Puzzle size={18} />,
    title: "Add plugins",
    description:
      "Install the Okapi Bridge plugin for 57+ file format filters, or browse the registry for community-contributed tools and formats.",
  },
];

const QUICK_ACTIONS = [
  {
    icon: <Languages size={16} />,
    label: "AI Translate a file",
    description: "Translate a single file using Claude, GPT, or Ollama",
  },
  {
    icon: <Wrench size={16} />,
    label: "Pseudo-translate for testing",
    description: "Generate pseudo-translations to test UI layout",
  },
  {
    icon: <Wrench size={16} />,
    label: "Run a quality check",
    description: "Validate translations against rule-based QA checks",
  },
];

export function WelcomePage({ onOpen, onNew }: WelcomePageProps) {
  const [error, setError] = useState<string | null>(null);
  const [recentFiles, setRecentFiles] = useState<
    Array<{ path: string; name: string; opened_at: string }>
  >([]);

  useEffect(() => {
    api.listRecentFiles().then((files) => {
      if (files) setRecentFiles(files);
    });
  }, []);

  const handleNew = useCallback(() => {
    const proj: KapiProject = {
      version: "v1",
      name: "New Project",
      source_language: "en-US",
      target_languages: [],
      flows: {},
    };
    onNew(proj);
  }, [onNew]);

  const handleOpen = useCallback(async () => {
    setError(null);
    try {
      const result = await api.openProjectDialog();
      if (result) {
        const path = (await api.getProjectPath()) ?? "";
        onOpen(result, path);
      }
    } catch (e) {
      setError(String(e));
    }
  }, [onOpen]);

  const handleOpenRecent = useCallback(
    async (path: string) => {
      setError(null);
      try {
        const result = await api.openProject(path);
        if (result) {
          onOpen(result, path);
        }
      } catch (e) {
        setError(`Failed to open ${path}: ${e}`);
      }
    },
    [onOpen],
  );

  return (
    <div className="flex h-screen flex-col bg-background">
      <div
        className="fixed inset-x-0 top-0 z-10 h-12"
        style={{ WebkitAppRegion: "drag" } as React.CSSProperties}
      />

      <div className="flex-1 overflow-auto">
        <div className="mx-auto max-w-3xl px-8 pb-12 pt-20">
          {/* Hero */}
          <div className="mb-12 flex flex-col items-center text-center">
            <img
              src="/neokapi-logo.png"
              alt="neokapi"
              className="mb-4 h-24 w-24 drop-shadow-lg"
            />
            <h1 className="mb-1 text-3xl font-bold tracking-tight">
              Kapi Desktop
            </h1>
            <p className="mb-6 max-w-md text-base text-muted-foreground">
              The open-source localization toolkit for developers.
              Format-aware parsing, AI translation, quality checks, and
              pluggable pipelines &mdash; all from your desktop.
            </p>

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
                aria-label="Open .kapi project file"
              >
                <FolderOpen size={16} />
                Open .kapi File
              </button>
            </div>

            {error && (
              <p className="mt-3 text-sm text-destructive" role="alert">
                {error}
              </p>
            )}
          </div>

          {/* Getting started */}
          <section className="mb-12">
            <h2 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
              Getting Started
            </h2>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              {GETTING_STARTED_STEPS.map((step) => (
                <div
                  key={step.title}
                  className="group rounded-xl border border-border p-4 transition-colors hover:border-primary/30 hover:bg-accent/30"
                >
                  <div className="mb-2 flex items-center gap-2.5">
                    <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10 text-primary">
                      {step.icon}
                    </div>
                    <h3 className="text-sm font-medium">{step.title}</h3>
                  </div>
                  <p className="text-xs leading-relaxed text-muted-foreground">
                    {step.description}
                  </p>
                </div>
              ))}
            </div>
          </section>

          {/* Quick actions */}
          <section className="mb-12">
            <h2 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
              Quick Actions
            </h2>
            <div className="space-y-2">
              {QUICK_ACTIONS.map((action) => (
                <button
                  key={action.label}
                  onClick={handleNew}
                  className="group flex w-full items-center gap-3 rounded-lg border border-border p-3 text-left transition-colors hover:border-primary/30 hover:bg-accent/30"
                  aria-label={action.label}
                >
                  <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                    {action.icon}
                  </div>
                  <div className="flex-1">
                    <div className="text-sm font-medium">{action.label}</div>
                    <div className="text-xs text-muted-foreground">
                      {action.description}
                    </div>
                  </div>
                  <ArrowRight
                    size={14}
                    className="text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100"
                  />
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
                      <div className="truncate text-xs text-muted-foreground">
                        {file.path}
                      </div>
                    </div>
                  </button>
                ))}
              </div>
            ) : (
              <div className="rounded-xl border border-dashed border-border p-8 text-center">
                <FolderOpen
                  size={24}
                  className="mx-auto mb-2 text-muted-foreground/50"
                />
                <p className="text-sm text-muted-foreground">
                  No recent projects yet. Create a new project or open an
                  existing .kapi file to get started.
                </p>
              </div>
            )}
          </section>

          <footer className="mt-12 border-t border-border pt-4 text-center">
            <p className="text-xs text-muted-foreground">
              Kapi Desktop is part of{" "}
              <span className="font-medium text-foreground">neokapi</span>, the
              open-source localization framework.
            </p>
          </footer>
        </div>
      </div>
    </div>
  );
}
