import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@neokapi/ui-primitives";
import { useState, useCallback } from "react";
import { Copy, Check } from "../components/icons";

interface VoiceMCPGuideProps {
  /** Origin of the Bowrain server this workspace lives on (shown in the sign-in + CI steps). */
  serverUrl?: string;
  /** API token for headless/CI auth (BOWRAIN_AUTH_TOKEN). */
  apiToken?: string;
}

function ConfigBlock({ title, config }: { title: string; config: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    navigator.clipboard
      .writeText(config)
      .then(() => {
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      })
      .catch(() => {
        // The clipboard can refuse — no permission, an insecure origin, a
        // browser that only allows it inside a user gesture. Saying "Copied"
        // over an empty clipboard is worse than not saying it.
        setCopied(false);
      });
  }, [config]);

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium">{title}</span>
        <Button variant="ghost" size="sm" className="h-6 gap-1 text-xs" onClick={handleCopy}>
          {copied ? <Check className="w-3 h-3" /> : <Copy className="w-3 h-3" />}
          {copied ? "Copied" : "Copy"}
        </Button>
      </div>
      <pre className="text-xs bg-muted/50 rounded-md p-3 overflow-x-auto font-mono whitespace-pre">
        {config}
      </pre>
    </div>
  );
}

export function VoiceMCPGuide({
  serverUrl = "https://app.bowrain.cloud",
  apiToken = "<your-api-token>",
}: VoiceMCPGuideProps) {
  // The MCP server is `kapi mcp` (stdio) with the kapi-bowrain plugin installed.
  // The editor launches it as a subprocess; it discovers the workspace's .kapi
  // recipe (bowrain: block) by upward walk and authenticates from the keychain
  // written by `kapi auth login`. No env vars are needed for the interactive path.
  const mcpServer = { command: "kapi", args: ["mcp"] };

  const claudeDesktopConfig = JSON.stringify({ mcpServers: { kapi: mcpServer } }, null, 2);
  const cursorConfig = JSON.stringify({ mcpServers: { kapi: mcpServer } }, null, 2);
  const vscodeConfig = JSON.stringify({ mcp: { servers: { kapi: mcpServer } } }, null, 2);

  // Headless/CI: skip interactive login by passing the server + token through env.
  const ciConfig = JSON.stringify(
    {
      mcpServers: {
        kapi: {
          command: "kapi",
          args: ["mcp"],
          env: {
            BOWRAIN_SERVER_URL: serverUrl,
            BOWRAIN_AUTH_TOKEN: apiToken,
          },
        },
      },
    },
    null,
    2,
  );

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold">MCP Connection Guide</h1>
        <p className="text-sm text-muted-foreground mt-1">
          Connect your AI coding assistant to Bowrain for brand-aware translations and compliance
          checking via the Model Context Protocol (MCP). The kapi CLI serves the MCP endpoint; the
          kapi-bowrain plugin adds the project-sync and brand knowledge tools.
        </p>
      </div>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">Prerequisites</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <div className="flex items-center gap-2">
            <Badge variant="outline" className="text-[10px]">
              1
            </Badge>
            <span>
              Install the CLI and Bowrain plugin:{" "}
              <code className="text-xs bg-muted px-1 rounded">
                brew install neokapi/tap/bowrain-cli
              </code>{" "}
              (brings the <code className="text-xs bg-muted px-1 rounded">kapi</code> CLI and the{" "}
              <code className="text-xs bg-muted px-1 rounded">kapi-bowrain</code> plugin)
            </span>
          </div>
          <div className="flex items-center gap-2">
            <Badge variant="outline" className="text-[10px]">
              2
            </Badge>
            <span>
              Sign in from your project directory:{" "}
              <code className="text-xs bg-muted px-1 rounded">kapi auth login</code> (authenticates
              against <code className="text-xs bg-muted px-1 rounded">{serverUrl}</code>)
            </span>
          </div>
          <div className="flex items-center gap-2">
            <Badge variant="outline" className="text-[10px]">
              3
            </Badge>
            <span>Add the MCP server configuration below to your editor</span>
          </div>
        </CardContent>
      </Card>

      <Tabs defaultValue="claude-desktop">
        <TabsList className="w-full grid grid-cols-3">
          <TabsTrigger value="claude-desktop">Claude Desktop</TabsTrigger>
          <TabsTrigger value="cursor">Cursor</TabsTrigger>
          <TabsTrigger value="vscode">VS Code</TabsTrigger>
        </TabsList>

        <TabsContent value="claude-desktop">
          <Card className="p-5 space-y-4">
            <p className="text-sm text-muted-foreground">
              Add the following to your Claude Desktop configuration file (
              <code className="text-xs bg-muted px-1 rounded">
                ~/Library/Application Support/Claude/claude_desktop_config.json
              </code>
              ):
            </p>
            <ConfigBlock title="claude_desktop_config.json" config={claudeDesktopConfig} />
          </Card>
        </TabsContent>

        <TabsContent value="cursor">
          <Card className="p-5 space-y-4">
            <p className="text-sm text-muted-foreground">
              Add the following to your Cursor MCP config (
              <code className="text-xs bg-muted px-1 rounded">~/.cursor/mcp.json</code>):
            </p>
            <ConfigBlock title=".cursor/mcp.json" config={cursorConfig} />
          </Card>
        </TabsContent>

        <TabsContent value="vscode">
          <Card className="p-5 space-y-4">
            <p className="text-sm text-muted-foreground">
              Add the following to your VS Code settings (
              <code className="text-xs bg-muted px-1 rounded">.vscode/settings.json</code>):
            </p>
            <ConfigBlock title=".vscode/settings.json" config={vscodeConfig} />
          </Card>
        </TabsContent>
      </Tabs>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">Headless / CI</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <p className="text-muted-foreground">
            Where interactive <code className="text-xs bg-muted px-1 rounded">kapi auth login</code>{" "}
            isn&rsquo;t possible, generate an API token from Settings and pass it, with the server
            URL, through the process environment instead:
          </p>
          <ConfigBlock title="token auth (CI)" config={ciConfig} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">Available MCP tools</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <p className="text-xs text-muted-foreground">
            Exposed by <code className="text-xs bg-muted px-1 rounded">kapi mcp</code> with the
            kapi-bowrain plugin installed.
          </p>
          <div className="grid gap-2">
            {[
              {
                name: "project_status",
                desc: "Show sync status: pending push/pull and server connection",
              },
              { name: "project_push", desc: "Upload local changes to the Bowrain server" },
              { name: "project_pull", desc: "Download translations from the Bowrain server" },
              {
                name: "concept_search",
                desc: "Search the workspace brand knowledge graph for governed terms",
              },
              {
                name: "concept_story",
                desc: "Show a concept's timeline: revisions, observations, change-sets",
              },
              {
                name: "experiment_status",
                desc: "Report brand knowledge-graph change-sets and blast radius",
              },
              {
                name: "voice_check",
                desc: "Score text against a voice profile (0–100 + findings)",
              },
              {
                name: "voice_rewrite",
                desc: "Rewrite off-brand copy by substituting forbidden/competitor terms",
              },
              { name: "term_lookup", desc: "Look up approved terminology in the terms store" },
            ].map((tool) => (
              <div key={tool.name} className="flex items-start gap-2 border rounded px-3 py-2">
                <code className="text-xs bg-muted px-1 rounded shrink-0">{tool.name}</code>
                <span className="text-xs text-muted-foreground">{tool.desc}</span>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
