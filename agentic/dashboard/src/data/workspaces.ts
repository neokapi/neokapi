export interface Workspace {
  id: string;
  name: string;
  slug: string;
  upstream: string;
  description: string;
  languages: string[];
  agentCount: number;
  status: "active" | "idle" | "paused";
  lastActivity: string;
}

export const workspaces: Workspace[] = [
  {
    id: "excalidraw",
    name: "Excalidraw",
    slug: "excalidraw-l10n",
    upstream: "excalidraw/excalidraw",
    description: "AI agents localizing Excalidraw — collaborative whiteboard",
    languages: ["fr-FR", "de-DE"],
    agentCount: 4,
    status: "active",
    lastActivity: new Date(Date.now() - 12 * 60_000).toISOString(),
  },
  {
    id: "docusaurus",
    name: "Docusaurus",
    slug: "docusaurus-l10n",
    upstream: "facebook/docusaurus",
    description: "Documentation framework localization — future workspace",
    languages: ["fr-FR", "de-DE", "ja-JP"],
    agentCount: 0,
    status: "idle",
    lastActivity: "",
  },
  {
    id: "gitea",
    name: "Gitea",
    slug: "gitea-l10n",
    upstream: "go-gitea/gitea",
    description: "Self-hosted Git service localization — future workspace",
    languages: ["fr-FR", "de-DE", "zh-CN"],
    agentCount: 0,
    status: "idle",
    lastActivity: "",
  },
];
