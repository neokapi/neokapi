import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import { createServer } from "node:http";
import { createTransport } from "nodemailer";
import { z } from "zod";
import { resolveRecipient } from "./roles.js";

// ---------------------------------------------------------------------------
// Configuration from environment
// ---------------------------------------------------------------------------

const SMTP_HOST = process.env.SMTP_HOST ?? "mailpit";
const SMTP_PORT = Number(process.env.SMTP_PORT ?? "1025");
const MAILPIT_API_HOST = process.env.MAILPIT_API_HOST ?? "mailpit";
const MAILPIT_API_PORT = Number(process.env.MAILPIT_API_PORT ?? "8025");
const AGENT_NAME = process.env.AGENT_NAME ?? "Agent";
const AGENT_EMAIL = process.env.AGENT_EMAIL ?? "agent@bowrain.test";
const PORT = Number(process.env.PORT ?? "3001");

// ---------------------------------------------------------------------------
// SMTP transporter (nodemailer)
// ---------------------------------------------------------------------------

const smtp = createTransport({
  host: SMTP_HOST,
  port: SMTP_PORT,
  secure: false, // Mailpit does not use TLS
  tls: { rejectUnauthorized: false },
});

// ---------------------------------------------------------------------------
// MCP server
// ---------------------------------------------------------------------------

const server = new McpServer({
  name: "email-mcp",
  version: "0.1.0",
});

// -- email.send -------------------------------------------------------------

server.tool(
  "email.send",
  "Send an email via SMTP. The `to` field accepts a role name (pm, brand-manager, developer, translator-fr, translator-de, translator-ja, qa, all) or a raw email address.",
  {
    to: z.string().describe("Recipient role name or email address"),
    subject: z.string().describe("Email subject line"),
    body: z.string().describe("Email body (plain text)"),
  },
  async ({ to, subject, body }) => {
    const recipients = resolveRecipient(to);

    await smtp.sendMail({
      from: `"${AGENT_NAME}" <${AGENT_EMAIL}>`,
      to: recipients.join(", "),
      subject,
      text: body,
    });

    return {
      content: [
        {
          type: "text" as const,
          text: `Email sent to ${recipients.join(", ")} with subject "${subject}"`,
        },
      ],
    };
  },
);

// -- email.listInbox --------------------------------------------------------

server.tool(
  "email.listInbox",
  "List emails received by this agent. Queries the Mailpit HTTP API.",
  {
    since: z
      .string()
      .optional()
      .describe("ISO-8601 timestamp — only return messages after this time"),
  },
  async ({ since }) => {
    const searchQuery = `to:${AGENT_EMAIL}`;
    const url = new URL(`http://${MAILPIT_API_HOST}:${MAILPIT_API_PORT}/api/v1/search`);
    url.searchParams.set("query", searchQuery);
    if (since) {
      url.searchParams.set("since", since);
    }

    const res = await fetch(url.toString());
    if (!res.ok) {
      return {
        content: [
          {
            type: "text" as const,
            text: `Mailpit API error: ${res.status} ${res.statusText}`,
          },
        ],
        isError: true,
      };
    }

    const data = (await res.json()) as {
      messages?: Array<{
        From: { Address: string; Name: string };
        Subject: string;
        Date: string;
        Snippet: string;
      }>;
    };

    const messages = (data.messages ?? []).map((m) => ({
      from: m.From?.Name ? `${m.From.Name} <${m.From.Address}>` : m.From?.Address,
      subject: m.Subject,
      date: m.Date,
      snippet: m.Snippet,
    }));

    return {
      content: [
        {
          type: "text" as const,
          text: JSON.stringify(messages, null, 2),
        },
      ],
    };
  },
);

// ---------------------------------------------------------------------------
// HTTP transport — Streamable HTTP on /mcp
// ---------------------------------------------------------------------------

const httpServer = createServer(async (req, res) => {
  if (req.url === "/health") {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ status: "ok" }));
    return;
  }

  if (req.url !== "/mcp") {
    res.writeHead(404);
    res.end("Not found");
    return;
  }

  const transport = new StreamableHTTPServerTransport({ sessionIdGenerator: undefined });
  await server.connect(transport);
  await transport.handleRequest(req, res);
});

httpServer.listen(PORT, () => {
  console.log(`email-mcp listening on :${PORT}/mcp`);
});
