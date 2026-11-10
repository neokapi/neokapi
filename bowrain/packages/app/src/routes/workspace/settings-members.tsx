import { useEffect } from "react";
import { useWorkspace, InviteManager, PageContainer } from "@neokapi/ui";

export function SettingsMembersRoute() {
  const { activeWorkspace } = useWorkspace();

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Members · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  if (!activeWorkspace) return null;

  return (
    <PageContainer width="standard">
      <InviteManager workspace={activeWorkspace} />
    </PageContainer>
  );
}
