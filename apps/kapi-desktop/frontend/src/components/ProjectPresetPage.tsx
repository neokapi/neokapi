import { useState, useEffect } from "react";
import { Sparkles, SkipForward, Loader2 } from "lucide-react";
import { Button, Badge } from "@neokapi/ui-primitives";
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
          {/* Detected preset — highlighted */}
          {detected && (
            <>
              <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Recommended
              </p>
              <Button
                variant="outline"
                onClick={() => handleApply(detected.name)}
                disabled={applying}
                className="group flex h-auto w-full whitespace-normal items-start gap-4 rounded-xl border-primary/30 bg-primary/5 p-5 text-left hover:border-primary/50 hover:bg-primary/10"
              >
                <div className="shrink-0 pt-0.5 text-primary">
                  <Sparkles size={20} />
                </div>
                <div className="flex-1">
                  <div className="flex items-center gap-2 text-sm font-medium">
                    {detected.name}
                    <Badge variant="secondary" className="text-[10px]">
                      detected
                    </Badge>
                    {applying && <Loader2 size={14} className="animate-spin" />}
                  </div>
                  <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                    {detected.description}
                  </p>
                </div>
              </Button>
            </>
          )}

          {/* Other presets */}
          {others.length > 0 && (
            <>
              <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Other presets
              </p>
              {others.map((p) => (
                <Button
                  key={p.name}
                  variant="outline"
                  onClick={() => handleApply(p.name)}
                  disabled={applying}
                  className="group flex h-auto w-full whitespace-normal items-start gap-4 rounded-xl p-5 text-left hover:border-primary/30 hover:bg-accent/30"
                >
                  <div className="shrink-0 pt-0.5 text-primary">
                    <Sparkles size={20} />
                  </div>
                  <div className="flex-1">
                    <div className="text-sm font-medium">{p.name}</div>
                    <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                      {p.description}
                    </p>
                  </div>
                </Button>
              ))}
            </>
          )}

          {/* Skip */}
          <Button
            variant="outline"
            onClick={onSkip}
            disabled={applying}
            className="group flex h-auto w-full whitespace-normal items-start gap-4 rounded-xl p-5 text-left hover:border-primary/30 hover:bg-accent/30"
          >
            <div className="shrink-0 pt-0.5 text-muted-foreground">
              <SkipForward size={20} />
            </div>
            <div className="flex-1">
              <div className="text-sm font-medium">Skip</div>
              <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                Configure everything manually in Settings.
              </p>
            </div>
          </Button>
        </div>
      </div>
    </div>
  );
}
