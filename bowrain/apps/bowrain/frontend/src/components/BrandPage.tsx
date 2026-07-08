import { useCallback, useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  useApi,
  useWorkspace,
  CandidateRulesList,
  BlastRadiusSummary,
  DriftAlert,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  Alert,
  AlertDescription,
} from "@neokapi/ui";
import type { ProjectInfo, CandidateRule, BlastRadius } from "@neokapi/ui";
import { ShieldCheck } from "lucide-react";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";

interface BrandPageProps {
  /** Projects available for blast-radius / drift evaluation. */
  projects: ProjectInfo[];
}

/**
 * BrandPage is the desktop's brand-governance review surface — the
 * correction-learning loop (AD-019) at parity with the web brand-review route.
 * It reuses the shared, prop-driven CandidateRulesList / BlastRadiusSummary /
 * DriftAlert components and drives them through the WailsApiAdapter, which
 * proxies the server's REST brand endpoints. Unlike web's route (which gets a
 * profileId from the URL), the desktop page lets the user pick a profile and an
 * evaluation project up front.
 */
export function BrandPage({ projects }: BrandPageProps) {
  const api = useApi();
  const qc = useQueryClient();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  // The brand review surface is a server/team feature; the personal
  // (disconnected) workspace has none, so the queries stay disabled there.
  const isTeam = !!ws && activeWorkspace?.type !== "personal";

  const [profileId, setProfileId] = useState("");
  const [projectId, setProjectId] = useState("");
  const [showHistory, setShowHistory] = useState(false);
  const [preview, setPreview] = useState<{ term: string; radius: BlastRadius } | null>(null);
  const [busyTerm, setBusyTerm] = useState<string | undefined>(undefined);
  const [actionError, setActionError] = useState<string | null>(null);

  // Profiles for the workspace.
  const profilesQuery = useQuery({
    queryKey: qk.brandProfiles(ws),
    enabled: isTeam,
    queryFn: () => api.listBrandProfiles(ws),
  });
  const profiles = profilesQuery.data ?? [];

  // Auto-select the first profile once loaded (UI selection, not a fetch).
  useEffect(() => {
    if (profiles.length > 0) setProfileId((cur) => cur || profiles[0].id);
  }, [profiles]);

  // Candidate rules for the selected profile.
  const candidatesQuery = useQuery({
    queryKey: qk.brandCandidates(ws, profileId, showHistory),
    enabled: isTeam && !!profileId,
    queryFn: () => api.listBrandCandidates(ws, profileId, { all: showHistory }),
  });
  const candidates = candidatesQuery.data ?? [];
  const loading = isTeam && !!profileId && candidatesQuery.isLoading;

  // Drift for the selected project. Errors degrade silently (no drift shown),
  // preserving the original behaviour.
  const driftQuery = useQuery({
    queryKey: qk.brandDrift(ws, projectId),
    enabled: isTeam && !!projectId,
    queryFn: () => api.getBrandDrift(ws, projectId),
  });
  const drift = driftQuery.data ?? null;

  // Cross-client freshness: brand-voice changes pushed from the server refresh
  // the workspace profiles + candidate rules.
  useInvalidateOnEvent("brand-voice-changed", [qk.brandProfiles(ws), ["brand-candidates", ws]]);

  const error =
    actionError ??
    (profilesQuery.error
      ? profilesQuery.error instanceof Error
        ? profilesQuery.error.message
        : "Failed to load brand profiles"
      : null) ??
    (candidatesQuery.error
      ? candidatesQuery.error instanceof Error
        ? candidatesQuery.error.message
        : "Failed to load candidate rules"
      : null);

  const refetchCandidates = useCallback(() => {
    void qc.invalidateQueries({ queryKey: ["brand-candidates", ws, profileId] });
  }, [qc, ws, profileId]);

  const onPromote = useCallback(
    async (c: CandidateRule) => {
      setBusyTerm(c.term);
      setActionError(null);
      try {
        await api.promoteBrandRule(ws, profileId, {
          term: c.term,
          replacement: c.replacement,
          correction_count: c.correction_count,
        });
        refetchCandidates();
      } catch (e) {
        setActionError(e instanceof Error ? e.message : "Failed to promote rule");
      } finally {
        setBusyTerm(undefined);
      }
    },
    [api, ws, profileId, refetchCandidates],
  );

  const onReject = useCallback(
    async (c: CandidateRule) => {
      setBusyTerm(c.term);
      setActionError(null);
      try {
        await api.rejectBrandRule(ws, profileId, { term: c.term, replacement: c.replacement });
        refetchCandidates();
      } catch (e) {
        setActionError(e instanceof Error ? e.message : "Failed to reject rule");
      } finally {
        setBusyTerm(undefined);
      }
    },
    [api, ws, profileId, refetchCandidates],
  );

  const onEvaluate = useCallback(
    async (c: CandidateRule) => {
      if (!projectId) return;
      setBusyTerm(c.term);
      setActionError(null);
      try {
        const radius = await api.evaluateBrandRule(ws, profileId, {
          term: c.term,
          replacement: c.replacement,
          project_id: projectId,
        });
        setPreview({ term: c.term, radius });
      } catch (e) {
        setActionError(e instanceof Error ? e.message : "Failed to evaluate rule");
      } finally {
        setBusyTerm(undefined);
      }
    },
    [api, ws, profileId, projectId],
  );

  if (!activeWorkspace || activeWorkspace.type === "personal") {
    return (
      <div className="p-6 text-sm text-muted-foreground" data-testid="brand-empty">
        Connect to a Bowrain server and select a team workspace to review brand rules.
      </div>
    );
  }

  return (
    <div className="mx-auto w-full max-w-3xl space-y-4 p-6" data-testid="brand-page">
      <header className="space-y-1">
        <h1 className="flex items-center gap-2 text-xl font-semibold">
          <ShieldCheck className="h-5 w-5" /> Review suggested rules
        </h1>
        <p className="text-sm text-muted-foreground">
          Brand checks act like tests for AI output. Repeated corrections become candidate rules —
          promote one to harden it into a check on every future generation, or reject it to stop it
          re-surfacing.
        </p>
      </header>

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <div className="flex flex-wrap items-center gap-3">
        <label className="flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">Profile</span>
          <select
            value={profileId}
            onChange={(e) => setProfileId(e.target.value)}
            className="rounded-md border bg-background px-2 py-1 text-sm"
            data-testid="brand-profile-select"
          >
            {profiles.length === 0 && <option value="">No profiles</option>}
            {profiles.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
        <label className="flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">Evaluate against</span>
          <select
            value={projectId}
            onChange={(e) => setProjectId(e.target.value)}
            className="rounded-md border bg-background px-2 py-1 text-sm"
            data-testid="brand-project-select"
          >
            <option value="">a project…</option>
            {projects.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
        <label className="ml-auto flex items-center gap-2 text-xs text-muted-foreground">
          <input
            type="checkbox"
            checked={showHistory}
            onChange={(e) => setShowHistory(e.target.checked)}
          />
          Show decided
        </label>
      </div>

      {projectId && drift ? <DriftAlert drift={drift} /> : null}

      {loading ? (
        <div className="py-8 text-center text-sm text-muted-foreground">Loading candidates…</div>
      ) : (
        <CandidateRulesList
          candidates={candidates}
          onPromote={onPromote}
          onReject={onReject}
          onEvaluate={projectId ? onEvaluate : undefined}
          busyTerm={busyTerm}
        />
      )}

      <Dialog open={preview !== null} onOpenChange={(open: boolean) => !open && setPreview(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Blast radius — promoting &ldquo;{preview?.term}&rdquo;</DialogTitle>
          </DialogHeader>
          {preview ? <BlastRadiusSummary radius={preview.radius} /> : null}
        </DialogContent>
      </Dialog>
    </div>
  );
}
