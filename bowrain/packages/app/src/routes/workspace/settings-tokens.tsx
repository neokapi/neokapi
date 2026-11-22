import { useEffect } from "react";
import { useWorkspace, ApiTokenManager, PageContainer } from "@neokapi/ui";

export function SettingsTokensRoute() {
  const { activeWorkspace } = useWorkspace();

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `API Tokens · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  if (!activeWorkspace) return null;

  return (
    <PageContainer width="standard">
      <ApiTokenManager workspace={activeWorkspace} />
    </PageContainer>
  );
}
