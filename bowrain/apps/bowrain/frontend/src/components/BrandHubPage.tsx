import { QueryClient, QueryClientProvider, useQueryClient } from "@tanstack/react-query";
import {
  useWorkspace,
  ConceptsSection,
  ConceptStorySection,
  ExperimentsView,
  ExperimentDetailView,
  ActivityView,
  BrandDashboardView,
} from "@neokapi/ui";
import type { ProjectInfo } from "@neokapi/ui";
import { BrandPage } from "./BrandPage";
import { useBackendEvents } from "../hooks/useBackendEvents";

/**
 * The Brand-hub sections (AD-021), mirroring the shared AppShell sub-nav
 * (`subNavConfig.brand`): Concepts · Voice · Experiments · Activity · Dashboard.
 */
export type BrandSection = "concepts" | "voice" | "experiments" | "activity" | "dashboard";

/**
 * One React Query client for the desktop Brand hub. The shared brand-hub views
 * (ConceptsSection, ExperimentsView, the dashboard, …) and their hooks drive
 * everything through React Query; the rest of the desktop app uses the
 * backend-events refetch layer instead, so the client is scoped to the hub
 * rather than the whole app. A module-level singleton keeps the cache across
 * section switches (the views unmount/remount as the user navigates).
 */
const brandQueryClient = new QueryClient();

export interface BrandHubPageProps {
  /** Projects available to the Voice review surface for blast-radius / drift. */
  projects: ProjectInfo[];
  /** Active sub-nav section, owned by App so the AppShell sub-nav stays in sync. */
  section: BrandSection;
  /** Drill-down: the open concept's id within the Concepts section ("" = list). */
  conceptId: string;
  /** Drill-down: the open change-set's id within the Experiments section ("" = list). */
  changesetId: string;
  /** Open a concept's story (switches to the Concepts section). */
  onOpenConcept: (conceptId: string) => void;
  /** Return from a concept's story to the Concepts list. */
  onCloseConcept: () => void;
  /** Open a change-set's detail (switches to the Experiments section). */
  onOpenExperiment: (changesetId: string) => void;
  /** Return from a change-set's detail to the Experiments list. */
  onCloseExperiment: () => void;
  /** Jump to a top-level section (the dashboard's quick links). */
  onGotoSection: (section: BrandSection) => void;
}

/**
 * BrandHubFreshness keeps the working-copy brand queries fresh.
 *
 * The desktop's only server push channel is the project-scoped gRPC
 * WatchProject stream (watcher.go), which carries brand-voice / termbase
 * change events but no workspace-scoped concept/change-set events — and it is
 * closed while the Brand hub is open, since the hub has no active project. So
 * the lightest correct freshness model for the workspace-scoped knowledge graph
 * is React Query's own refetch (per-hook staleTime + refetch-on-focus) plus the
 * mutation-driven invalidation the brand hooks already do on every write. On
 * top of that, when a project *is* being watched, a brand-voice / termbase
 * event invalidates the hub's query keys here for cross-client freshness. No new
 * Wails events or App methods are needed (so no binding regen).
 */
function BrandHubFreshness() {
  const qc = useQueryClient();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  useBackendEvents(["brand-voice-changed", "termbase-changed"], () => {
    if (!ws) return;
    void qc.invalidateQueries({ queryKey: ["concepts", ws] });
    void qc.invalidateQueries({ queryKey: ["concept", ws] });
    void qc.invalidateQueries({ queryKey: ["changesets", ws] });
    void qc.invalidateQueries({ queryKey: ["changeset", ws] });
    void qc.invalidateQueries({ queryKey: ["markets", ws] });
    void qc.invalidateQueries({ queryKey: ["brand-profiles", ws] });
  });

  return null;
}

function BrandHubContent({
  projects,
  section,
  conceptId,
  changesetId,
  onOpenConcept,
  onCloseConcept,
  onOpenExperiment,
  onCloseExperiment,
  onGotoSection,
}: BrandHubPageProps) {
  const { activeWorkspace } = useWorkspace();

  // The brand knowledge graph is a server/team capability; the personal
  // (disconnected) workspace has none, mirroring BrandPage's empty state.
  if (!activeWorkspace || activeWorkspace.type === "personal") {
    return (
      <div className="p-6 text-sm text-muted-foreground" data-testid="brand-hub-empty">
        Connect to a Bowrain server and select a team workspace to manage your brand knowledge
        graph.
      </div>
    );
  }

  switch (section) {
    case "concepts":
      return conceptId ? (
        <ConceptStorySection
          conceptId={conceptId}
          onBack={onCloseConcept}
          onOpenConcept={onOpenConcept}
          onOpenExperiments={() => onGotoSection("experiments")}
        />
      ) : (
        <ConceptsSection onOpenConcept={onOpenConcept} />
      );
    case "voice":
      // The desktop Voice surface is the correction-learning review loop
      // (AD-019): profile authoring is a web/MCP workflow, so the desktop
      // re-homes the existing BrandPage review surface under Voice.
      return <BrandPage projects={projects} />;
    case "experiments":
      return changesetId ? (
        <ExperimentDetailView changesetId={changesetId} onBack={onCloseExperiment} />
      ) : (
        <ExperimentsView onOpenExperiment={onOpenExperiment} />
      );
    case "activity":
      return <ActivityView onOpenConcept={onOpenConcept} onOpenExperiment={onOpenExperiment} />;
    case "dashboard":
      return (
        <BrandDashboardView
          onOpenExperiment={onOpenExperiment}
          onOpenConcept={onOpenConcept}
          onViewExperiments={() => onGotoSection("experiments")}
          onViewConcepts={() => onGotoSection("concepts")}
          onViewVoice={() => onGotoSection("voice")}
        />
      );
    default:
      return <ConceptsSection onOpenConcept={onOpenConcept} />;
  }
}

/**
 * BrandHubPage is the desktop's unified Brand hub (AD-021) — full parity with
 * the web brand routes, built on the same shared, router-agnostic @neokapi/ui
 * brand-hub views. The desktop has no router, so section + drill-down selection
 * are React state owned by App (so the AppShell brand sub-nav stays in sync) and
 * passed down here; this component owns only the React Query provider the views
 * need and maps (section, selection) to the right view.
 */
export function BrandHubPage(props: BrandHubPageProps) {
  return (
    <QueryClientProvider client={brandQueryClient}>
      <BrandHubFreshness />
      <BrandHubContent {...props} />
    </QueryClientProvider>
  );
}
