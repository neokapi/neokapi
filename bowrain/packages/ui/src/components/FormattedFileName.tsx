import type { ReactNode } from "react";
import {
  Globe,
  FileCode,
  FileJson,
  FileText,
  FileType,
  MessageSquare,
  FileSpreadsheet,
  ArrowRight,
  Lock,
} from "./icons";

const FORMAT_ICONS: Record<string, (cls: string) => ReactNode> = {
  html: (cls) => <Globe className={cls} />,
  xml: (cls) => <FileCode className={cls} />,
  json: (cls) => <FileJson className={cls} />,
  yaml: (cls) => <FileText className={cls} />,
  plaintext: (cls) => <FileType className={cls} />,
  po: (cls) => <MessageSquare className={cls} />,
  properties: (cls) => <Lock className={cls} />,
  markdown: (cls) => <FileText className={cls} />,
  csv: (cls) => <FileSpreadsheet className={cls} />,
  xliff: (cls) => <ArrowRight className={cls} />,
  xliff2: (cls) => <ArrowRight className={cls} />,
};

export function formatIcon(format: string, className = "w-4 h-4 inline-block align-text-bottom") {
  const factory = FORMAT_ICONS[format];
  return factory ? factory(className) : <FileCode className={className} />;
}

interface FormattedFileNameProps {
  name: string;
  format?: string;
  iconClassName?: string;
}

/**
 * Renders a filename with a format icon and the extension in faded monospace.
 * e.g. icon + "myfile" + ".json" (faded)
 */
export function FormattedFileName({
  name,
  format,
  iconClassName = "w-4 h-4 shrink-0",
}: FormattedFileNameProps) {
  const dotIndex = name.lastIndexOf(".");
  const hasExtension = dotIndex > 0;
  const baseName = hasExtension ? name.slice(0, dotIndex) : name;
  const extension = hasExtension ? name.slice(dotIndex) : null;

  return (
    <span className="inline-flex items-center gap-1.5">
      {format && formatIcon(format, iconClassName)}
      <span>
        <span>{baseName}</span>
        {extension && (
          <span className="text-muted-foreground/70 font-mono text-[0.9em]">{extension}</span>
        )}
      </span>
    </span>
  );
}
