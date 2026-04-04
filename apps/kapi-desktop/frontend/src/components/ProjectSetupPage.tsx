import { useState } from "react";
import { FolderInput, FolderOutput, FileBox, Loader2 } from "lucide-react";
import { Button } from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";

interface ProjectSetupPageProps {
  tabID: string;
  onDone: () => void;
}

const templates = [
  {
    id: "input-output",
    icon: <FolderInput size={20} />,
    secondIcon: <FolderOutput size={20} />,
    title: "Input \u2192 Output",
    description:
      "A folder for input files and a folder for output files. Source files go in ./input/, translations are written to ./output/{lang}/.",
  },
  {
    id: "empty",
    icon: <FileBox size={20} />,
    title: "Empty Project",
    description: "Start with a blank project and configure everything yourself.",
  },
];

export function ProjectSetupPage({ tabID, onDone }: ProjectSetupPageProps) {
  const [applying, setApplying] = useState<string | null>(null);

  const handleSelect = async (templateID: string) => {
    setApplying(templateID);
    try {
      await api.applyTemplate(tabID, templateID);
      onDone();
    } catch {
      setApplying(null);
    }
  };

  return (
    <div className="flex h-full items-center justify-center p-6">
      <div className="w-full max-w-lg">
        <h1 className="mb-2 text-center text-xl font-semibold">Get Started</h1>
        <p className="mb-8 text-center text-sm text-muted-foreground">
          Choose a template to set up your project structure.
        </p>
        <div className="space-y-3">
          {templates.map((t) => (
            <Button
              key={t.id}
              variant="outline"
              onClick={() => handleSelect(t.id)}
              disabled={applying !== null}
              className="group flex w-full h-auto whitespace-normal items-start gap-4 rounded-xl p-5 text-left hover:border-primary/30 hover:bg-accent/30"
            >
              <div className="flex shrink-0 items-center gap-1.5 pt-0.5 text-primary">
                {t.icon}
                {t.secondIcon && (
                  <>
                    <span className="text-xs text-muted-foreground">&rarr;</span>
                    {t.secondIcon}
                  </>
                )}
              </div>
              <div className="flex-1">
                <div className="flex items-center gap-2 text-sm font-medium">
                  {t.title}
                  {applying === t.id && <Loader2 size={14} className="animate-spin" />}
                </div>
                <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                  {t.description}
                </p>
              </div>
            </Button>
          ))}
        </div>
      </div>
    </div>
  );
}
