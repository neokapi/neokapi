import { useEffect } from "react";
import { useWorkspace, WorkspaceLanguageSettings, PageContainer } from "@neokapi/ui";

export function SettingsLanguagesRoute() {
  const { activeWorkspace } = useWorkspace();

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Languages · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  if (!activeWorkspace) return null;

  return (
    <PageContainer width="standard">
      <WorkspaceLanguageSettings workspace={activeWorkspace} />
    </PageContainer>
  );
}
