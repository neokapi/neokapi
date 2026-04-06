import { useState, useEffect } from "react";
import { Sparkles, SkipForward } from "lucide-react";
import { Badge, ActionCard } from "@neokapi/ui-primitives";
import type { KapiProject } from "../types/api";
import { api } from "../hooks/useApi";

export interface ProjectPresetPageProps {
  tabID: string;
  detectedPreset: string;
  onApplied: (project: KapiProject) => void;
  onSkip: () => void;
}

export function ProjectPresetPage({
  tabID,
  detectedPreset,
  onApplied,
  onSkip,
}: ProjectPresetPageProps) {
  const [applying, setApplying] = useState(false);
  const [presets, setPresets] = useState<Array<{ name: string; description: string }>>([]);

  useEffect(() => {
    api
      .listPresets()
      .then((p) => {
        if (p) setPresets(p);
      })
      .catch(() => {});
  }, []);

  const detected = presets.find((p) => p.name === detectedPreset);
  const others = presets.filter((p) => p.name !== detectedPreset);

  const handleApply = async (presetName: string) => {
    setApplying(true);
    try {
      const updated = await api.applyPreset(tabID, presetName);
      if (updated) onApplied(updated);
    } catch {
      setApplying(false);
    }
  };

  return (
    <div className="flex h-full items-center justify-center p-6">
      <div className="w-full max-w-lg">
        <h1 className="mb-2 text-center text-xl font-semibold">Configure Project</h1>
        <p className="mb-8 text-center text-sm text-muted-foreground">
          We detected an existing codebase. Apply a preset to configure content patterns and
          workflows automatically.
        </p>

        <div className="space-y-3">
          {detected && (
            <>
              <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Recommended
              </p>
              <ActionCard
                icon={<Sparkles size={20} />}
                title={detected.name}
                description={detected.description}
                badge={
                  <Badge variant="secondary" className="text-[10px]">
                    detected
                  </Badge>
                }
                highlighted
                loading={applying}
                disabled={applying}
                onClick={() => handleApply(detected.name)}
              />
            </>
          )}

          {others.length > 0 && (
            <>
              <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Other presets
              </p>
              {others.map((p) => (
                <ActionCard
                  key={p.name}
                  icon={<Sparkles size={20} />}
                  title={p.name}
                  description={p.description}
                  disabled={applying}
                  onClick={() => handleApply(p.name)}
                />
              ))}
            </>
          )}

          <ActionCard
            icon={<SkipForward size={20} />}
            title="Skip"
            description="Configure everything manually in Settings."
            disabled={applying}
            onClick={onSkip}
          />
        </div>
      </div>
    </div>
  );
}
