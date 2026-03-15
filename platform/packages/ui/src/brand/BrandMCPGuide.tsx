import { useState, useCallback } from "react";
import { Card, CardHeader, CardTitle, CardContent } from "../components/ui/card";
import { Button } from "../components/ui/button";
import { Badge } from "../components/ui/badge";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "../components/ui/tabs";
import { Copy, Check } from "../components/icons";

interface BrandMCPGuideProps {
  serverUrl?: string;
  apiToken?: string;
}

function ConfigBlock({ title, config }: { title: string; config: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(config).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
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

export function BrandMCPGuide({ serverUrl = "http://localhost:8080", apiToken = "<your-api-token>" }: BrandMCPGuideProps) {
  const claudeDesktopConfig = JSON.stringify(
    {
      mcpServers: {
        bowrain: {
          command: "bowrain",
          args: ["mcp", "serve"],
          env: {
            BOWRAIN_SERVER_URL: serverUrl,
            BOWRAIN_API_TOKEN: apiToken,
          },
        },
      },
    },
    null,
    2,
  );

  const cursorConfig = JSON.stringify(
    {
      "mcp.servers": {
        bowrain: {
          command: "bowrain",
          args: ["mcp", "serve"],
          env: {
            BOWRAIN_SERVER_URL: serverUrl,
            BOWRAIN_API_TOKEN: apiToken,
          },
        },
      },
    },
    null,
    2,
  );

  const vscodeConfig = JSON.stringify(
    {
      "mcp": {
        "servers": {
          "bowrain": {
            "command": "bowrain",
            "args": ["mcp", "serve"],
            "env": {
              "BOWRAIN_SERVER_URL": serverUrl,
              "BOWRAIN_API_TOKEN": apiToken,
            },
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
          checking via the Model Context Protocol (MCP).
        </p>
      </div>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">Prerequisites</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <div className="flex items-center gap-2">
            <Badge variant="outline" className="text-[10px]">1</Badge>
            <span>Install the <code className="text-xs bg-muted px-1 rounded">bowrain</code> CLI</span>
          </div>
          <div className="flex items-center gap-2">
            <Badge variant="outline" className="text-[10px]">2</Badge>
            <span>Generate an API token from Settings</span>
          </div>
          <div className="flex items-center gap-2">
            <Badge variant="outline" className="text-[10px]">3</Badge>
            <span>Add the MCP server configuration to your editor</span>
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
              Add the following to your Claude Desktop configuration file
              (<code className="text-xs bg-muted px-1 rounded">~/Library/Application Support/Claude/claude_desktop_config.json</code>):
            </p>
            <ConfigBlock title="claude_desktop_config.json" config={claudeDesktopConfig} />
          </Card>
        </TabsContent>

        <TabsContent value="cursor">
          <Card className="p-5 space-y-4">
            <p className="text-sm text-muted-foreground">
              Add the following to your Cursor settings
              (<code className="text-xs bg-muted px-1 rounded">.cursor/mcp.json</code>):
            </p>
            <ConfigBlock title=".cursor/mcp.json" config={cursorConfig} />
          </Card>
        </TabsContent>

        <TabsContent value="vscode">
          <Card className="p-5 space-y-4">
            <p className="text-sm text-muted-foreground">
              Add the following to your VS Code settings
              (<code className="text-xs bg-muted px-1 rounded">.vscode/settings.json</code>):
            </p>
            <ConfigBlock title=".vscode/settings.json" config={vscodeConfig} />
          </Card>
        </TabsContent>
      </Tabs>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">Available MCP Tools</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <div className="grid gap-2">
            {[
              { name: "brand_check", desc: "Check content against your brand voice profile" },
              { name: "brand_profiles", desc: "List and manage brand voice profiles" },
              { name: "brand_suggest", desc: "Get brand-compliant text suggestions" },
              { name: "translate", desc: "Translate content with brand voice awareness" },
              { name: "terminology", desc: "Look up approved terminology" },
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
