import { useState, useRef, useCallback, useMemo } from "react";
import type { ProjectInfo, StreamInfo } from "../types/api";
import { useLocales } from "../hooks/useLocales";
import { useIsMobile } from "../hooks/useIsMobile";
import { useSetBreadcrumb } from "../context/BreadcrumbContext";
import { useStream } from "../context/StreamContext";
import { Button } from "./ui/button";
import { Badge } from "./ui/badge";
import { GlassCard } from "./ui/card";
import { OpenInDesktop } from "./OpenInDesktop";
import { StreamSelector } from "./StreamSelector";
import { StreamBadge } from "./StreamBadge";
import {
  ArrowLeft, ArrowRight, Globe, FileCode, FileJson, FileText,
  FileType, MessageSquare, FileSpreadsheet, Upload, X, Lock, Package,
} from "./icons";

interface ProjectViewProps {
  project: ProjectInfo;
  onBack: () => void;
  onOpenFile: (fileName: string) => void;
  /** Upload files via adapter. Web apps pass File objects; desktop passes file paths. */
  onUploadFiles: (files: File[]) => void;
  onRemoveFile: (fileName: string) => void;
  onOpenTM?: () => void;
  onOpenTerms?: () => void;
  /** When set, shows "Open in Bowrain Desktop" banner with deep link. */
  serverMode?: { serverURL: string; workspaceSlug: string };
}

export function ProjectView({
  project,
  onBack,
  onOpenFile,
  onUploadFiles,
  onRemoveFile,
  onOpenTM,
  onOpenTerms,
  serverMode,
}: ProjectViewProps) {
  const { getDisplayName } = useLocales();
  const isMobile = useIsMobile();
  const { activeStream, setActiveStream } = useStream();
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragOver, setDragOver] = useState(false);

  // Register breadcrumb in the top bar area
  const breadcrumbNode = useMemo(() => (
    <button onClick={onBack} data-testid="back-to-projects" className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer bg-transparent border-none p-0">
      <ArrowLeft className="w-3.5 h-3.5" /> Projects
    </button>
  ), [onBack]);
  useSetBreadcrumb(breadcrumbNode);

  const items = project.items ?? [];
  const totalBlocks = items.reduce((sum, f) => sum + f.block_count, 0);
  const totalWords = items.reduce((sum, f) => sum + f.word_count, 0);

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    if (e.dataTransfer.files.length > 0) {
      onUploadFiles(Array.from(e.dataTransfer.files));
    }
  }, [onUploadFiles]);

  const handleFileInputChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files.length > 0) {
      onUploadFiles(Array.from(e.target.files));
      e.target.value = "";
    }
  }, [onUploadFiles]);

  const formatIcon = (format: string) => {
    const cls = "w-4 h-4 inline-block align-text-bottom";
    const icons: Record<string, React.ReactNode> = {
      html: <Globe className={cls} />,
      xml: <FileCode className={cls} />,
      json: <FileJson className={cls} />,
      yaml: <FileText className={cls} />,
      plaintext: <FileType className={cls} />,
      po: <MessageSquare className={cls} />,
      properties: <Lock className={cls} />,
      markdown: <FileText className={cls} />,
      csv: <FileSpreadsheet className={cls} />,
      xliff: <ArrowRight className={cls} />,
      xliff2: <ArrowRight className={cls} />,
    };
    return icons[format] || <FileCode className={cls} />;
  };

  return (
    <div className="flex-1 min-h-0 overflow-auto">
      {serverMode && (
        <OpenInDesktop
          projectId={project.id}
          serverURL={serverMode.serverURL}
          workspaceSlug={serverMode.workspaceSlug}
        />
      )}

      {/* Project overview card */}
      <GlassCard intensity="subtle" className={isMobile ? "p-4 mb-3" : "p-6 mb-4"}>
        <div className={isMobile ? "flex flex-col gap-3 mb-4" : "flex items-center justify-between mb-6"}>
          <div>
            <h2 className={isMobile ? "text-lg font-semibold" : "text-xl font-semibold"}>{project.name}</h2>
            <p className="text-[13px] text-muted-foreground mt-1">
              {getDisplayName(project.source_locale)} <ArrowRight className="w-3.5 h-3.5 inline-block" /> {project.target_locales.map(l => getDisplayName(l)).join(", ")}
            </p>
          </div>
          <div className="flex gap-2">
            {onOpenTerms && (
              <Button variant="ghost" size="sm" onClick={onOpenTerms} data-testid="open-terms-btn">
                Terminology
              </Button>
            )}
            {onOpenTM && (
              <Button variant="ghost" size="sm" onClick={onOpenTM} data-testid="open-tm-btn">
                Translation Memory
              </Button>
            )}
            {project.streams && project.streams.length > 0 && (
              <StreamSelector
                streams={project.streams}
                activeStream={project.streams.find((s) => s.name === activeStream) ?? null}
                onStreamChange={(stream: StreamInfo) => setActiveStream(stream.name)}
              />
            )}
          </div>
        </div>

        <div className="flex gap-3">
          <div className="flex-1 text-center rounded-lg border border-border/50 py-3">
            <div className={isMobile ? "text-xl font-bold" : "text-2xl font-bold"}>{items.length}</div>
            <div className="text-xs text-muted-foreground">Files</div>
          </div>
          <div className="flex-1 text-center rounded-lg border border-border/50 py-3">
            <div className={isMobile ? "text-xl font-bold" : "text-2xl font-bold"}>{totalBlocks}</div>
            <div className="text-xs text-muted-foreground">Blocks</div>
          </div>
          <div className="flex-1 text-center rounded-lg border border-border/50 py-3">
            <div className={isMobile ? "text-xl font-bold" : "text-2xl font-bold"}>{totalWords}</div>
            <div className="text-xs text-muted-foreground">Words</div>
          </div>
        </div>
      </GlassCard>

      {/* Files card */}
      <GlassCard intensity="subtle" className={isMobile ? "p-4" : "p-6"}>
        <div className="flex items-center justify-between mb-4">
          <div>
            <h3 className={isMobile ? "text-base font-semibold" : "text-lg font-semibold"}>Files</h3>
            <p className="text-[13px] text-muted-foreground mt-1">{items.length} file{items.length !== 1 ? "s" : ""} in project</p>
          </div>
          <div>
            <input ref={inputRef} type="file" multiple onChange={handleFileInputChange} className="hidden" />
            <Button size="sm" onClick={() => inputRef.current?.click()} data-testid="add-files-btn">
              Add Files
            </Button>
          </div>
        </div>

        {/* Drop zone */}
        {!isMobile && (
          <div
            className={`flex flex-col items-center justify-center gap-2 p-8 mb-6 rounded-lg border border-dashed border-border transition-all ${dragOver ? "ring-2 ring-primary bg-accent/30" : "bg-accent/10"}`}
            onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
            onDragLeave={() => setDragOver(false)}
            onDrop={handleDrop}
            data-testid="file-drop-zone"
          >
            <Package className="w-8 h-8 text-muted-foreground opacity-30" />
            <span className="text-muted-foreground text-[13px]">
              Drag and drop files here to add them to the project
            </span>
          </div>
        )}

        {/* File table */}
        {items.length > 0 && (
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead>
                <tr className="border-b border-border">
                  <th className={`${isMobile ? "px-2" : "px-4"} py-2.5 text-left text-sm font-medium text-muted-foreground`}>File</th>
                  {!isMobile && <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">Format</th>}
                  <th className={`${isMobile ? "px-2" : "px-4"} py-2.5 text-right text-sm font-medium text-muted-foreground`}>Blocks</th>
                  {!isMobile && <th className="px-4 py-2.5 text-right text-sm font-medium text-muted-foreground">Words</th>}
                  <th className={`${isMobile ? "px-1 w-10" : "px-4 w-20"} py-2.5 text-sm font-medium text-muted-foreground`}></th>
                </tr>
              </thead>
              <tbody>
                {items.map((f) => (
                  <tr key={f.name} className="border-b border-border/50 transition-colors hover:bg-accent/50" data-testid={`file-row-${f.name}`}>
                    <td className={`${isMobile ? "px-2" : "px-4"} py-2.5 text-sm`}>
                      <button
                        onClick={() => onOpenFile(f.name)}
                        className="bg-transparent border-none text-primary cursor-pointer text-sm p-0 hover:underline inline-flex items-center gap-1.5 text-left break-all"
                        data-testid={`open-file-${f.name}`}
                      >
                        {formatIcon(f.format)} {f.name}
                      </button>
                    </td>
                    {!isMobile && (
                      <td className="px-4 py-2.5 text-sm">
                        <Badge variant="secondary">{f.format}</Badge>
                      </td>
                    )}
                    <td className={`${isMobile ? "px-2" : "px-4"} py-2.5 text-sm text-muted-foreground text-right`}>{f.block_count}</td>
                    {!isMobile && (
                      <td className="px-4 py-2.5 text-sm text-muted-foreground text-right">{f.word_count}</td>
                    )}
                    <td className={`${isMobile ? "px-1" : "px-4"} py-2.5 text-sm text-right`}>
                      <button
                        onClick={(e) => { e.stopPropagation(); onRemoveFile(f.name); }}
                        className="bg-transparent border-none text-muted-foreground cursor-pointer px-2 py-1 rounded hover:text-destructive transition-colors"
                        data-testid={`remove-file-${f.name}`}
                      >
                        <X className="w-3.5 h-3.5" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </GlassCard>
    </div>
  );
}
