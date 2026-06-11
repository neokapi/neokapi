// ScriptStepPanel — the flow workspace's config panel for a `script` step,
// mounted via FlowEditor's renderStepConfigPanel. Instead of the schema form's
// plain textarea it gives the full ScriptLab experience in place: Monaco with
// the script API .d.ts (typed completions for part/emit/skip/log), an example
// library, and the same code config the engine runs — the step IS the script.

import React, { useEffect, useRef, useState } from "react";
import { Code2 } from "lucide-react";
import { Button, PanelHeader } from "@neokapi/ui-primitives";
import ScriptCodeEditor from "./ScriptCodeEditor";
import { DEFAULT_SCRIPT, SCRIPT_EXAMPLES } from "./scriptApi";
import shared from "./styles.module.css";

export interface ScriptStepPanelProps {
  config: Record<string, unknown>;
  onConfigChange: (config: Record<string, unknown>) => void;
  onClose: () => void;
  onRemove?: () => void;
}

export default function ScriptStepPanel({
  config,
  onConfigChange,
  onClose,
  onRemove,
}: ScriptStepPanelProps): React.ReactElement {
  // Local code state, synced to the step config debounced (300ms) and flushed
  // on unmount — same contract as the default StepConfigPanel, so a pending
  // edit is never dropped when the panel closes.
  const [code, setCode] = useState(() =>
    typeof config.code === "string" && config.code ? config.code : DEFAULT_SCRIPT,
  );
  const codeRef = useRef(code);
  const configRef = useRef(config);
  configRef.current = config;
  const onConfigChangeRef = useRef(onConfigChange);
  onConfigChangeRef.current = onConfigChange;

  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const dirty = useRef(false);

  const emit = () => {
    dirty.current = false;
    onConfigChangeRef.current({ ...configRef.current, code: codeRef.current });
  };

  const handleChange = (next: string) => {
    setCode(next);
    codeRef.current = next;
    dirty.current = true;
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(emit, 300);
  };

  // Flush the pending edit on unmount (panel closed / selection switched).
  useEffect(() => {
    return () => {
      if (timer.current) clearTimeout(timer.current);
      if (dirty.current) emit();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div
      className="flex h-full flex-col overflow-hidden border-l border-border bg-background"
      style={{ width: "min(480px, calc(100vw - 2rem))" }}
    >
      <PanelHeader className="flex-col items-start gap-0.5 py-2.5">
        <div className="flex w-full items-center justify-between">
          <div className="flex items-center gap-1.5 text-[11px] font-semibold text-foreground">
            <Code2 size={12} />
            Script
          </div>
          <div className="flex items-center gap-1">
            {onRemove && (
              <Button
                variant="ghost"
                size="xs"
                className="h-5 px-1.5 text-[9px] text-destructive"
                onClick={onRemove}
              >
                Remove
              </Button>
            )}
            <Button variant="ghost" size="xs" className="h-5 px-1.5 text-[9px]" onClick={onClose}>
              Close
            </Button>
          </div>
        </div>
        <div className="text-[10px] text-muted-foreground">
          A JavaScript transform run over each Part — edit with completions, or load an example.
        </div>
      </PanelHeader>

      <div className="flex items-center gap-2 border-b border-border px-3 py-1.5">
        <label className="text-[10px] font-semibold text-muted-foreground">Example</label>
        <select
          className={shared.select}
          defaultValue=""
          onChange={(e) => {
            const ex = SCRIPT_EXAMPLES.find((x) => x.id === e.target.value);
            if (ex) handleChange(ex.code);
            e.target.value = "";
          }}
        >
          <option value="" disabled>
            Load an example…
          </option>
          {SCRIPT_EXAMPLES.map((ex) => (
            <option key={ex.id} value={ex.id} title={ex.blurb}>
              {ex.label}
            </option>
          ))}
        </select>
      </div>

      <div className="min-h-0 flex-1">
        <ScriptCodeEditor code={code} onChange={handleChange} height="100%" />
      </div>

      <div className="border-t border-border px-3 py-1.5 text-[10px] leading-snug text-muted-foreground">
        The code lands on the step as <code>config.code</code> — press Run in the toolbar and this
        exact script processes every part.
      </div>
    </div>
  );
}
