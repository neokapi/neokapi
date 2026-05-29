import {
  Alert,
  AlertDescription,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@neokapi/ui-primitives";
import { useState, useCallback, useEffect, useMemo } from "react";
import type { ProjectInfo, BlockInfo, TranslationStats } from "../types/api";
import { useEditorApi } from "../hooks/useEditorApi";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import { useLocales } from "../hooks/useLocales";
import { useSetBreadcrumb } from "../context/BreadcrumbContext";
import { ArrowLeft, Languages, Wand2, Sparkles, Loader2 } from "./icons";

interface PreProcessSurfaceProps {
  project: ProjectInfo;
  fileName: string;
  onBack: () => void;
  /** Optional slot for the cross-surface switcher (Pre-process/Translate/Review). */
  surfaceTabs?: React.ReactNode;
}

type OpKey = "pseudo" | "tm" | "ai";

/**
 * PreProcessSurface is the pre-flight route — bulk source-prep operations run
 * before per-block translation: pseudo-translate (layout/expansion testing),
 * bulk TM leverage (apply exact + fuzzy matches across the file), and AI bulk
 * draft. These were removed from the per-block Translate toolbar so the editor
 * stays focused on editing one block at a time.
 */
export function PreProcessSurface({
  project,
  fileName,
  onBack,
  surfaceTabs,
}: PreProcessSurfaceProps) {
  const [targetLocale, setTargetLocale] = useState(project.target_languages[0] || "");
  const [running, setRunning] = useState<OpKey | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [results, setResults] = useState<Partial<Record<OpKey, TranslationStats>>>({});
  const [blockTotal, setBlockTotal] = useState<number | null>(null);

  const { getDisplayName } = useLocales();
  const api = useEditorApi();
  const fullApi = useApi();
  const { activeWorkspace } = useWorkspace();
  const wsSlug = activeWorkspace?.slug ?? "";

  const breadcrumbNode = useMemo(
    () => (
      <button
        onClick={onBack}
        data-testid="back-to-project"
        className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer bg-transparent border-none p-0"
      >
        <ArrowLeft className="w-3.5 h-3.5" /> {project.name}
      </button>
    ),
    [onBack, project.name],
  );
  useSetBreadcrumb(breadcrumbNode);

  // Load a block count so the surface can report the file's size (a proxy for
  // extraction/source-prep state) without a dedicated endpoint.
  useEffect(() => {
    let cancelled = false;
    api
      .getFileBlocks(project.id, fileName)
      .then((b: BlockInfo[]) => {
        if (!cancelled) setBlockTotal((b || []).filter((x) => x.translatable).length);
      })
      .catch(() => {
        if (!cancelled) setBlockTotal(null);
      });
    return () => {
      cancelled = true;
    };
  }, [api, project.id, fileName]);

  const runPseudo = useCallback(async () => {
    setRunning("pseudo");
    setError(null);
    try {
      const stats = await fullApi.pseudoTranslateFile(wsSlug, project.id, fileName, targetLocale);
      setResults((prev) => ({ ...prev, pseudo: stats }));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Pseudo-translate failed");
    } finally {
      setRunning(null);
    }
  }, [fullApi, wsSlug, project.id, fileName, targetLocale]);

  const runTM = useCallback(async () => {
    setRunning("tm");
    setError(null);
    try {
      const stats = await api.tmTranslateFile(project.id, fileName, targetLocale);
      setResults((prev) => ({ ...prev, tm: stats }));
    } catch (e) {
      setError(e instanceof Error ? e.message : "TM leverage failed");
    } finally {
      setRunning(null);
    }
  }, [api, project.id, fileName, targetLocale]);

  const resultLine = (stats?: TranslationStats) =>
    stats ? (
      <span className="text-xs text-success">
        Filled {stats.translated_blocks} of {stats.total_blocks} block(s)
      </span>
    ) : null;

  const ops: {
    key: OpKey;
    title: string;
    desc: string;
    icon: React.ReactNode;
    run?: () => void;
    cta: string;
    disabled?: boolean;
    note?: string;
  }[] = [
    {
      key: "pseudo",
      title: "Pseudo-translate",
      desc: "Generate accented, length-expanded placeholder translations to surface truncation, layout, and encoding problems before real translation begins.",
      icon: <Wand2 className="w-4 h-4" />,
      run: runPseudo,
      cta: "Run pseudo-translate",
    },
    {
      key: "tm",
      title: "Bulk TM leverage",
      desc: "Pre-fill targets from the translation memory across the whole file — exact and high-confidence fuzzy matches land as drafts you can review.",
      icon: <Languages className="w-4 h-4" />,
      run: runTM,
      cta: "Leverage TM",
    },
    {
      key: "ai",
      title: "AI bulk draft",
      desc: "Draft every untranslated block with the configured AI provider. Configure a provider in project settings, then start the draft from the Translate editor's AI actions.",
      icon: <Sparkles className="w-4 h-4" />,
      cta: "Configure in settings",
      disabled: true,
      note: "Provider required",
    },
  ];

  return (
    <div className="flex flex-col flex-1 min-h-0 overflow-auto" data-testid="preprocess-surface">
      {/* Header */}
      <div className="flex items-center gap-3 mb-4">
        {surfaceTabs}
        <span className="text-base font-semibold flex-1 truncate">Pre-process · {fileName}</span>
        <Select value={targetLocale} onValueChange={setTargetLocale}>
          <SelectTrigger className="w-[180px]" data-testid="locale-selector">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {project.target_languages.map((l) => (
              <SelectItem key={l} value={l}>
                {getDisplayName(l)} ({l})
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <p className="text-sm text-muted-foreground mb-4 max-w-2xl">
        Run file-wide source-prep here before editing block by block.
        {blockTotal !== null && <> This file has {blockTotal} translatable block(s).</>}
      </p>

      {error && (
        <Alert variant="destructive" className="mb-3 max-w-2xl">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <div className="grid gap-3 max-w-2xl">
        {ops.map((op) => (
          <Card key={op.key} data-testid={`preprocess-${op.key}`}>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                {op.icon}
                {op.title}
              </CardTitle>
              <CardDescription>{op.desc}</CardDescription>
            </CardHeader>
            <CardContent className="flex items-center gap-3">
              <Button
                size="sm"
                onClick={op.run}
                disabled={op.disabled || running !== null}
                data-testid={`preprocess-run-${op.key}`}
              >
                {running === op.key ? (
                  <>
                    <Loader2 className="w-3.5 h-3.5 mr-1 animate-spin" /> Running…
                  </>
                ) : (
                  op.cta
                )}
              </Button>
              {op.note && <span className="text-xs text-muted-foreground">{op.note}</span>}
              {resultLine(results[op.key])}
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}
