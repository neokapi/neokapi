import {
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
import { useState, useCallback, useEffect } from "react";
import { ErrorNotice } from "../errors";
import type { ProjectInfo, TranslationStats } from "../types/api";
import { useEditorApi } from "../hooks/useEditorApi";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import { useLocales } from "../hooks/useLocales";
import { Languages, Wand2, Sparkles, Loader2 } from "./icons";

interface PreProcessSurfaceProps {
  project: ProjectInfo;
  fileName: string;
  onBack: () => void;
  /** Optional slot for the cross-surface switcher (Pre-process/Translate/Review). */
  surfaceTabs?: React.ReactNode;
}

type OpKey = "pseudo" | "memory" | "ai";

/**
 * PreProcessSurface is the pre-flight route — bulk source-prep operations run
 * before per-block translation: pseudo-translate (layout/expansion testing),
 * bulk content-memory leverage (apply exact + fuzzy matches across the file), and AI bulk
 * draft. These were removed from the per-block Translate toolbar so the editor
 * stays focused on editing one block at a time.
 */
export function PreProcessSurface({
  project,
  fileName,
  onBack: _onBack,
  surfaceTabs,
}: PreProcessSurfaceProps) {
  const [targetLocale, setTargetLocale] = useState(project.target_languages[0] || "");
  const [running, setRunning] = useState<OpKey | null>(null);
  const [error, setError] = useState<{ title: string; cause?: unknown } | null>(null);
  const [results, setResults] = useState<Partial<Record<OpKey, TranslationStats>>>({});
  const [blockTotal, setBlockTotal] = useState<number | null>(null);

  const { getDisplayName } = useLocales();
  const api = useEditorApi();
  const fullApi = useApi();
  const { activeWorkspace } = useWorkspace();
  const wsSlug = activeWorkspace?.slug ?? "";

  // The file's translatable size is a count query — the surface reports the
  // number, so it asks for the number.
  useEffect(() => {
    let cancelled = false;
    api
      .getBlockCounts(project.id, fileName)
      .then((counts) => {
        if (!cancelled) setBlockTotal(counts.translatable);
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
      setError({ title: "Couldn't pseudo-translate the file", cause: e });
    } finally {
      setRunning(null);
    }
  }, [fullApi, wsSlug, project.id, fileName, targetLocale]);

  const runMemory = useCallback(async () => {
    setRunning("memory");
    setError(null);
    try {
      const stats = await api.memoryTranslateFile(project.id, fileName, targetLocale);
      setResults((prev) => ({ ...prev, memory: stats }));
    } catch (e) {
      setError({ title: "Couldn't recycle from the content memory", cause: e });
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
      key: "memory",
      title: "Bulk recycle",
      desc: "Pre-fill targets from the content memory across the whole file: exact and high-confidence fuzzy matches land as drafts you can review.",
      icon: <Languages className="w-4 h-4" />,
      run: runMemory,
      cta: "Recycle from memory",
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
        <ErrorNotice
          title={error.title}
          error={error.cause}
          variant="inline"
          className="mb-3 max-w-2xl"
        />
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
