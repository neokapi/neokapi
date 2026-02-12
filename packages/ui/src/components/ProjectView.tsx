import { useState, useRef, useCallback } from "react";
import type { ProjectInfo } from "../types/api";
import { useLocales } from "../hooks/useLocales";
import { Button } from "./ui/button";
import { Badge } from "./ui/badge";
import { Card, CardContent } from "./ui/card";
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
  onSave?: () => void;
}

export function ProjectView({
  project,
  onBack,
  onOpenFile,
  onUploadFiles,
  onRemoveFile,
  onOpenTM,
  onOpenTerms,
  onSave,
}: ProjectViewProps) {
  const { getDisplayName } = useLocales();
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragOver, setDragOver] = useState(false);

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
    <div>
      <div className="flex items-center gap-3 mb-6">
        <Button variant="outline" size="sm" onClick={onBack} data-testid="back-to-projects">
          <ArrowLeft className="w-3.5 h-3.5 mr-1" />
          Projects
        </Button>
        <h2 className="flex-1 text-xl font-semibold">{project.name}</h2>
        {onOpenTerms && (
          <Button variant="outline" onClick={onOpenTerms} data-testid="open-terms-btn">
            Terminology
          </Button>
        )}
        {onOpenTM && (
          <Button variant="outline" onClick={onOpenTM} data-testid="open-tm-btn">
            Translation Memory
          </Button>
        )}
        {onSave && (
          <Button onClick={onSave} data-testid="save-project-btn">
            Save
          </Button>
        )}
      </div>

      <div className="flex gap-4 mb-6">
        <Card className="flex-1 text-center">
          <CardContent className="py-3">
            <div className="text-2xl font-bold">{items.length}</div>
            <div className="text-xs text-muted-foreground">Files</div>
          </CardContent>
        </Card>
        <Card className="flex-1 text-center">
          <CardContent className="py-3">
            <div className="text-2xl font-bold">{totalBlocks}</div>
            <div className="text-xs text-muted-foreground">Blocks</div>
          </CardContent>
        </Card>
        <Card className="flex-1 text-center">
          <CardContent className="py-3">
            <div className="text-2xl font-bold">{totalWords}</div>
            <div className="text-xs text-muted-foreground">Words</div>
          </CardContent>
        </Card>
        <Card className="flex-1 text-center">
          <CardContent className="py-3">
            <div className="text-sm font-semibold">
              {getDisplayName(project.source_locale)} <ArrowRight className="w-3.5 h-3.5 inline-block" /> {project.target_locales.map(l => getDisplayName(l)).join(", ")}
            </div>
            <div className="text-xs text-muted-foreground">Languages</div>
          </CardContent>
        </Card>
      </div>

      {/* File drop zone */}
      <div
        className={`flex flex-col items-center justify-center gap-2 p-8 border-2 border-dashed rounded-lg bg-card ${dragOver ? "border-primary" : "border-border"}`}
        onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
        onDragLeave={() => setDragOver(false)}
        onDrop={handleDrop}
        data-testid="file-drop-zone"
      >
        <Package className="w-8 h-8 text-muted-foreground opacity-30" />
        <span className="text-muted-foreground text-[13px]">
          Drag and drop files here to add them to the project
        </span>
        <input ref={inputRef} type="file" multiple onChange={handleFileInputChange} className="hidden" />
        <Button size="sm" className="mt-2" onClick={() => inputRef.current?.click()} data-testid="add-files-btn">
          Add Files
        </Button>
      </div>

      {/* File list */}
      {items.length > 0 && (
        <div className="mt-4">
          <table className="w-full border-collapse bg-card rounded-lg overflow-hidden">
            <thead>
              <tr>
                <th className="px-4 py-2.5 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">File</th>
                <th className="px-4 py-2.5 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Format</th>
                <th className="px-4 py-2.5 text-right text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Blocks</th>
                <th className="px-4 py-2.5 text-right text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Words</th>
                <th className="px-4 py-2.5 text-xs font-semibold text-muted-foreground border-b border-border w-20"></th>
              </tr>
            </thead>
            <tbody>
              {items.map((f) => (
                <tr key={f.name} className="transition-colors hover:bg-accent/50" data-testid={`file-row-${f.name}`}>
                  <td className="px-4 py-2.5 text-sm border-b border-border">
                    <button
                      onClick={() => onOpenFile(f.name)}
                      className="bg-transparent border-none text-primary cursor-pointer text-sm p-0 hover:underline inline-flex items-center gap-1.5"
                      data-testid={`open-file-${f.name}`}
                    >
                      {formatIcon(f.format)} {f.name}
                    </button>
                  </td>
                  <td className="px-4 py-2.5 text-sm border-b border-border">
                    <Badge variant="secondary">{f.format}</Badge>
                  </td>
                  <td className="px-4 py-2.5 text-sm border-b border-border text-right">{f.block_count}</td>
                  <td className="px-4 py-2.5 text-sm border-b border-border text-right">{f.word_count}</td>
                  <td className="px-4 py-2.5 text-sm border-b border-border text-right">
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
    </div>
  );
}
