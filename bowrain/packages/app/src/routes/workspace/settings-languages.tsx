import { useEffect } from "react";
import { useWorkspace, WorkspaceLanguageSettings } from "@neokapi/ui";

export function SettingsLanguagesRoute() {
  const { activeWorkspace } = useWorkspace();

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Languages — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  if (!activeWorkspace) return null;

  return (
    <div className="mx-auto w-full max-w-3xl py-4">
      <WorkspaceLanguageSettings workspace={activeWorkspace} />
    </div>
  );
}
