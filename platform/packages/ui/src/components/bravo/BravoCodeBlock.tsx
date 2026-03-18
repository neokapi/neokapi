import { useState } from "react";
import { cn } from "../../lib/utils";

export interface BravoCodeBlockProps {
  language: string;
  code: string;
  result?: {
    stdout?: string;
    stderr?: string;
    exit_code?: number;
  };
}

export function BravoCodeBlock({ language, code, result }: BravoCodeBlockProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    navigator.clipboard.writeText(code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  };

  return (
    <div className="my-2 rounded-md border bg-card overflow-hidden text-xs">
      <div className="flex items-center justify-between bg-muted/50 px-3 py-1.5">
        <span className="font-mono text-muted-foreground">{language || "code"}</span>
        <button
          onClick={handleCopy}
          className="text-muted-foreground hover:text-foreground transition-colors"
        >
          {copied ? "Copied!" : "Copy"}
        </button>
      </div>

      <pre className="overflow-x-auto p-3">
        <code>{code}</code>
      </pre>

      {result && (
        <div className="border-t">
          <div className="bg-muted/30 px-3 py-1.5 flex items-center gap-2">
            <span className="text-muted-foreground font-medium">Output</span>
            {result.exit_code !== undefined && (
              <span
                className={cn(
                  "text-[10px] font-mono px-1.5 py-0.5 rounded",
                  result.exit_code === 0
                    ? "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400"
                    : "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
                )}
              >
                exit {result.exit_code}
              </span>
            )}
          </div>
          {result.stdout && (
            <pre className="overflow-x-auto p-3 text-green-700 dark:text-green-400">
              {result.stdout}
            </pre>
          )}
          {result.stderr && (
            <pre className="overflow-x-auto p-3 text-destructive">{result.stderr}</pre>
          )}
        </div>
      )}
    </div>
  );
}
