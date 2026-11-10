import { useEffect } from "react";
import { useWorkspace, RoleTemplateManager, PageContainer } from "@neokapi/ui";

export function SettingsRolesRoute() {
  const { activeWorkspace } = useWorkspace();

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Roles · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  if (!activeWorkspace) return null;

  return (
    <PageContainer width="standard">
      <RoleTemplateManager workspace={activeWorkspace} />
    </PageContainer>
  );
}
