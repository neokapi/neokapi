import { useState } from "react";
import type { ProjectInfo } from "../types/api";
import { useLocales } from "../hooks/useLocales";
import { LocaleSelect, MultiLocaleSelect } from "./LocaleSelect";
import { Button } from "./ui/button";
import { CardContent, GlassCard } from "./ui/card";
import { Badge } from "./ui/badge";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "./ui/dialog";
import {
  FolderOpen,
  ArrowRight,
  Upload,
  Globe,
  FileText,
  Plus,
  Clock,
  Sparkles,
} from "./icons";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Compute relative time string from an ISO timestamp. */
function relativeTime(iso: string): string {
  const now = Date.now();
  const then = new Date(iso).getTime();
  const diff = now - then;
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  const months = Math.floor(days / 30);
  if (months < 12) return `${months}mo ago`;
  return `${Math.floor(months / 12)}y ago`;
}

/** Sum a numeric field across project items. */
function sumItems(project: ProjectInfo, field: "word_count" | "block_count"): number {
  return project.items?.reduce((acc, item) => acc + (item[field] ?? 0), 0) ?? 0;
}

/** Format a number with compact notation (e.g. 1.2k). */
function compactNumber(n: number): string {
  if (n < 1000) return String(n);
  if (n < 10_000) return `${(n / 1000).toFixed(1)}k`;
  if (n < 1_000_000) return `${Math.round(n / 1000)}k`;
  return `${(n / 1_000_000).toFixed(1)}M`;
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface ProjectDashboardProps {
  projects: ProjectInfo[];
  onCreateProject: (name: string, sourceLang: string, targetLangs: string[]) => void;
  onOpenProject: (project: ProjectInfo) => void;
  /** Optional callback to create a sample project for first-time users. */
  onCreateSampleProject?: () => void;
  /** Workspace name shown in the greeting. */
  workspaceName?: string;
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

/** Summary statistics bar shown above the project grid. */
function DashboardStats({ projects }: { projects: ProjectInfo[] }) {
  const totalWords = projects.reduce((acc, p) => acc + sumItems(p, "word_count"), 0);
  const uniqueLocales = new Set(projects.flatMap((p) => p.target_locales));
  const totalFiles = projects.reduce((acc, p) => acc + (p.items?.length ?? 0), 0);

  const stats = [
    { label: "Projects", value: String(projects.length), icon: FolderOpen },
    { label: "Words", value: compactNumber(totalWords), icon: FileText },
    { label: "Languages", value: String(uniqueLocales.size), icon: Globe },
    { label: "Files", value: String(totalFiles), icon: Upload },
  ];

  return (
    <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6">
      {stats.map((s) => (
        <GlassCard key={s.label} intensity="subtle" hover={false} padding="compact">
          <div className="flex items-center gap-3 px-4 py-3">
            <div className="flex items-center justify-center w-9 h-9 rounded-lg bg-primary/10">
              <s.icon className="w-4 h-4 text-primary" />
            </div>
            <div>
              <div className="text-lg font-semibold leading-tight">{s.value}</div>
              <div className="text-xs text-muted-foreground">{s.label}</div>
            </div>
          </div>
        </GlassCard>
      ))}
    </div>
  );
}

/** Single project card in the grid. */
function ProjectCard({
  project,
  onOpen,
  getDisplayName,
}: {
  project: ProjectInfo;
  onOpen: () => void;
  getDisplayName: (code: string) => string;
}) {
  const wordCount = sumItems(project, "word_count");
  const fileCount = project.items?.length ?? 0;
  const streamCount = project.streams?.length ?? 0;

  return (
    <GlassCard
      intensity="medium"
      hover
      glow="primary"
      onClick={onOpen}
      className="cursor-pointer transition-all group"
      data-testid={`project-card-${project.id}`}
    >
      <CardContent className="pt-4 pb-4">
        {/* Header: name + language count */}
        <div className="flex items-start justify-between mb-3">
          <h3 className="font-semibold text-base leading-snug pr-2">{project.name}</h3>
          <Badge variant="secondary" className="shrink-0 text-[11px]">
            {project.target_locales.length} lang{project.target_locales.length !== 1 ? "s" : ""}
          </Badge>
        </div>

        {/* Locale mapping */}
        <div className="text-[13px] text-muted-foreground mb-3 flex items-center gap-1.5 flex-wrap">
          <span className="font-medium text-foreground/80">{getDisplayName(project.source_locale)}</span>
          <ArrowRight className="w-3 h-3 shrink-0 opacity-50" />
          <span className="truncate">
            {project.target_locales.map((l) => getDisplayName(l)).join(", ")}
          </span>
        </div>

        {/* Stats row */}
        <div className="flex items-center gap-4 text-xs text-muted-foreground">
          <span className="flex items-center gap-1">
            <FileText className="w-3 h-3" />
            {fileCount} file{fileCount !== 1 ? "s" : ""}
          </span>
          <span className="flex items-center gap-1">
            <Globe className="w-3 h-3" />
            {compactNumber(wordCount)} words
          </span>
          {streamCount > 1 && (
            <span className="flex items-center gap-1">
              <Sparkles className="w-3 h-3" />
              {streamCount} streams
            </span>
          )}
        </div>

        {/* Last modified */}
        {project.modified_at && (
          <div className="flex items-center gap-1 mt-3 pt-3 border-t border-border/30 text-[11px] text-muted-foreground">
            <Clock className="w-3 h-3" />
            <span>Updated {relativeTime(project.modified_at)}</span>
          </div>
        )}
      </CardContent>
    </GlassCard>
  );
}

// ---------------------------------------------------------------------------
// Onboarding: Getting started pathways
// ---------------------------------------------------------------------------

interface PathwayCardProps {
  icon: React.ReactNode;
  title: string;
  description: string;
  action: string;
  onClick: () => void;
  glow?: "blue" | "violet" | "cyan";
}

function PathwayCard({ icon, title, description, action, onClick, glow }: PathwayCardProps) {
  return (
    <GlassCard
      intensity="medium"
      hover
      glow={glow}
      className="cursor-pointer transition-all group flex flex-col"
      onClick={onClick}
    >
      <CardContent className="pt-5 pb-5 flex flex-col flex-1">
        <div className="flex items-center justify-center w-11 h-11 rounded-xl bg-primary/10 mb-4">
          {icon}
        </div>
        <h3 className="font-semibold text-sm mb-1.5">{title}</h3>
        <p className="text-xs text-muted-foreground leading-relaxed mb-4 flex-1">
          {description}
        </p>
        <span className="text-xs font-medium text-primary flex items-center gap-1 group-hover:gap-2 transition-all">
          {action}
          <ArrowRight className="w-3 h-3" />
        </span>
      </CardContent>
    </GlassCard>
  );
}

function OnboardingView({
  onStartCreate,
  onCreateSampleProject,
}: {
  onStartCreate: () => void;
  onCreateSampleProject?: () => void;
}) {
  return (
    <div className="flex flex-col items-center" data-testid="empty-projects">
      {/* Hero */}
      <div className="text-center mb-8 max-w-lg">
        <div className="flex items-center justify-center w-16 h-16 rounded-2xl bg-primary/10 mx-auto mb-5">
          <Sparkles className="w-8 h-8 text-primary" />
        </div>
        <h2 className="text-2xl font-bold mb-2 tracking-tight">
          Get started with your first project
        </h2>
        <p className="text-sm text-muted-foreground leading-relaxed">
          Bowrain helps you localize content into any language. Choose how you
          want to bring your content in.
        </p>
      </div>

      {/* Pathway cards */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 w-full max-w-2xl mb-8">
        <PathwayCard
          icon={<FolderOpen className="w-5 h-5 text-primary" />}
          title="From your repo"
          description="Track translation files in your codebase with bowrain init. Sync changes automatically."
          action="Create project"
          onClick={onStartCreate}
          glow="violet"
        />
        <PathwayCard
          icon={<Upload className="w-5 h-5 text-primary" />}
          title="Upload files"
          description="Drop in JSON, XLIFF, PO, HTML, or any supported format. Start translating immediately."
          action="Create project"
          onClick={onStartCreate}
          glow="blue"
        />
        <PathwayCard
          icon={<Globe className="w-5 h-5 text-primary" />}
          title="Connect a CMS"
          description="Pull content from WordPress, Contentful, or other platforms via connectors."
          action="Create project"
          onClick={onStartCreate}
          glow="cyan"
        />
      </div>

      {/* Sample project CTA */}
      {onCreateSampleProject && (
        <button
          type="button"
          onClick={onCreateSampleProject}
          className="text-xs text-muted-foreground hover:text-foreground transition-colors flex items-center gap-1.5"
        >
          <Sparkles className="w-3 h-3" />
          Or try a sample project to explore Bowrain
        </button>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function ProjectDashboard({
  projects,
  onCreateProject,
  onOpenProject,
  onCreateSampleProject,
  workspaceName,
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

  const isEmpty = projects.length === 0;

  return (
    <>
      {isEmpty ? (
        <OnboardingView
          onStartCreate={() => setShowCreate(true)}
          onCreateSampleProject={onCreateSampleProject}
        />
      ) : (
        <div>
          {/* Header */}
          <div className="flex justify-between items-center mb-5">
            <div>
              <h2 className="text-xl font-semibold tracking-tight">
                {workspaceName ? `${workspaceName}` : "Projects"}
              </h2>
              <p className="text-[13px] text-muted-foreground mt-0.5">
                {projects.length} project{projects.length !== 1 ? "s" : ""} in
                this workspace
              </p>
            </div>
            <Button onClick={() => setShowCreate(true)} data-testid="new-project-btn">
              <Plus className="w-4 h-4 mr-1.5" />
              New Project
            </Button>
          </div>

          {/* Stats */}
          <DashboardStats projects={projects} />

          {/* Project grid */}
          <div className="grid grid-cols-[repeat(auto-fill,minmax(min(300px,100%),1fr))] gap-4">
            {projects.map((p) => (
              <ProjectCard
                key={p.id}
                project={p}
                onOpen={() => onOpenProject(p)}
                getDisplayName={getDisplayName}
              />
            ))}
          </div>
        </div>
      )}

      {/* Create project dialog */}
      <Dialog open={showCreate} onOpenChange={handleOpenChange}>
        <DialogContent
          size="md"
          data-testid="create-project-dialog"
          onInteractOutside={(e: Event) => e.preventDefault()}
        >
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
            <Button
              onClick={handleCreate}
              disabled={!name.trim() || targetLangsList.length === 0}
              data-testid="create-project-submit"
            >
              Create
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
