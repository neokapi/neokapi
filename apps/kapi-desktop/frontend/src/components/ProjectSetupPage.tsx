import { useState } from "react";
import { FolderInput, FolderOutput, FileBox } from "lucide-react";
import { ActionCard } from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";

interface ProjectSetupPageProps {
  tabID: string;
  onDone: () => void;
}

const templates = [
  {
    id: "input-output",
    icon: (
      <div className="flex items-center gap-1.5">
        <FolderInput size={20} />
        <span className="text-xs text-muted-foreground">&rarr;</span>
        <FolderOutput size={20} />
      </div>
    ),
    title: "Input \u2192 Output",
    description:
      "Source files in ./input/, translations written to ./output/{lang}/. Great for batch processing.",
  },
  {
    id: "empty",
    icon: <FileBox size={20} />,
    title: "Start empty",
    description: "Blank project \u2014 configure everything yourself.",
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
            <ActionCard
              key={t.id}
              icon={t.icon}
              title={t.title}
              description={t.description}
              loading={applying === t.id}
              disabled={applying !== null}
              onClick={() => handleSelect(t.id)}
            />
          ))}
        </div>
      </div>
    </div>
  );
}
