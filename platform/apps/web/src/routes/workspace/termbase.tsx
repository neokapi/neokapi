import { useEffect, useMemo } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { ConceptExplorer, useWorkspace, useApi, Card } from "@neokapi/ui";
import { projectsQueryOptions } from "../../queries";

export function TermbaseRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const { activeWorkspace } = useWorkspace();
  const adapter = useApi();
  const ws = activeWorkspace?.slug ?? "";

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Concepts — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  // Fetch projects to compute aggregate locales
  const { data: projects } = useQuery({
    ...projectsQueryOptions(adapter, ws),
    enabled: !!ws,
  });

  // Union of all source and target locales across projects
  const { sourceLocale, targetLocales } = useMemo(() => {
    if (!projects || projects.length === 0)
      return { sourceLocale: "", targetLocales: [] as string[] };
    const sources = new Set<string>();
    const targets = new Set<string>();
    for (const p of projects) {
      sources.add(p.default_source_language);
      for (const t of p.target_languages) {
        targets.add(t);
      }
    }
    const srcArr = [...sources];
    return {
      sourceLocale: srcArr[0] ?? "",
      targetLocales: [...targets],
    };
  }, [projects]);

  if (!activeWorkspace) {
    return (
      <Card className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm">
        Select a workspace
      </Card>
    );
  }

  return (
    <div className="h-[calc(100vh-var(--topbar-height,56px))] p-4 md:p-6">
      <ConceptExplorer
        sourceLocale={sourceLocale}
        targetLocales={targetLocales}
        projects={projects}
        onBack={() => navigate({ to: "/$workspace", params: { workspace: workspace ?? "" } })}
      />
    </div>
  );
}
