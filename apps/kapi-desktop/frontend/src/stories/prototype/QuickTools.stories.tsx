import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import {
  ArrowRight,
  FileSearch,
  FileText,
  Languages,
  Play,
  Repeat,
  ShieldCheck,
  Upload,
  Wand2,
  Workflow,
  X,
} from "lucide-react";
import { Badge, Button } from "@neokapi/ui-primitives";
import { DesktopFrame } from "./_shared";

/**
 * Prototype: the ad-hoc Quick Tools toolbox.
 *
 * A self-contained "drop a file → run one operation" scratchpad. No project
 * machinery — just a file, a grid of one-shot operations, and a small "run a
 * saved flow" affordance. Translate appears here as the localization operation.
 */
const meta = {
  title: "Prototype/QuickTools",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj;

interface Op {
  icon: ReactNode;
  title: string;
  description: string;
  l10n?: boolean;
}

const ops: Op[] = [
  {
    icon: <ShieldCheck size={18} />,
    title: "Check",
    description: "Brand, terminology, and placeholder checks",
  },
  { icon: <Wand2 size={18} />, title: "Rewrite", description: "Bring text back on voice" },
  { icon: <FileText size={18} />, title: "Stats", description: "Word and segment counts" },
  { icon: <FileSearch size={18} />, title: "Inspect", description: "Read the content model" },
  { icon: <Repeat size={18} />, title: "Convert", description: "Translate between formats" },
  {
    icon: <Languages size={18} />,
    title: "Translate",
    description: "AI translation to a target language",
    l10n: true,
  },
];

function Toolbox() {
  return (
    <div className="flex min-h-screen items-start justify-center bg-background p-8 text-foreground">
      <div className="w-full max-w-2xl">
        <DesktopFrame title="Quick Tools">
          <div className="p-6">
            <div className="mb-1 flex items-center gap-2">
              <h1 className="text-base font-semibold">Toolbox</h1>
              <Badge variant="outline" className="text-[10px]">
                Ad-hoc
              </Badge>
            </div>
            <p className="mb-5 text-xs text-muted-foreground">
              Run a single operation on a file. Nothing here is saved to a project.
            </p>

            {/* Dropzone with a selected file. */}
            <div className="mb-6 rounded-xl border-2 border-dashed border-border bg-muted/20 p-5">
              <div className="flex items-center justify-center gap-2 text-sm text-muted-foreground">
                <Upload size={16} />
                <span>Drop a file here, or</span>
                <button type="button" className="font-medium text-primary hover:underline">
                  browse
                </button>
              </div>
              <div className="mx-auto mt-4 flex max-w-sm items-center gap-2 rounded-lg border border-border bg-background px-3 py-2">
                <FileText size={15} className="shrink-0 text-muted-foreground" />
                <span className="flex-1 truncate text-sm">homepage.fr.json</span>
                <Badge variant="secondary" className="text-[10px]">
                  json
                </Badge>
                <Button variant="ghost" size="icon-xs" aria-label="Remove file">
                  <X size={13} />
                </Button>
              </div>
            </div>

            {/* Operation grid. */}
            <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-3">
              {ops.map((op) => (
                <button
                  key={op.title}
                  type="button"
                  className="group relative flex flex-col items-start gap-2 rounded-xl border border-border p-3 text-left transition-colors hover:border-primary/40 hover:bg-accent/30"
                >
                  <span className="flex size-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
                    {op.icon}
                  </span>
                  <span className="flex items-center gap-1.5 text-sm font-medium">
                    {op.title}
                    {op.l10n && (
                      <Badge variant="secondary" className="px-1 py-0 text-[9px] leading-tight">
                        L10N
                      </Badge>
                    )}
                  </span>
                  <span className="text-xs leading-snug text-muted-foreground">
                    {op.description}
                  </span>
                </button>
              ))}
            </div>

            {/* Run a saved flow. */}
            <div className="mt-6 flex items-center gap-3 rounded-xl border border-border bg-muted/20 p-3">
              <Workflow size={18} className="shrink-0 text-muted-foreground" />
              <div className="flex-1">
                <div className="text-sm font-medium">Run a flow</div>
                <div className="text-xs text-muted-foreground">
                  Chain operations with a saved flow, ad-hoc on this file.
                </div>
              </div>
              <Button variant="outline" size="sm">
                Pick a flow
                <ArrowRight size={14} />
              </Button>
            </div>

            <div className="mt-5 flex justify-end">
              <Button>
                <Play size={14} />
                Run on homepage.fr.json
              </Button>
            </div>
          </div>
        </DesktopFrame>
      </div>
    </div>
  );
}

export const Default: Story = {
  render: () => <Toolbox />,
};
