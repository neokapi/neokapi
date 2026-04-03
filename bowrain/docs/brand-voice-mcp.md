# Brand Voice MCP Server

The Bowrain MCP server exposes brand voice capabilities to AI assistants via the Model Context Protocol (MCP) over Streamable HTTP.

## Endpoint

```
POST /mcp/     # JSON-RPC 2.0 (Streamable HTTP)
GET  /mcp/     # SSE for server-initiated messages
```

## Authentication

OAuth 2.1 bearer token via the `Authorization` header. Tokens are standard Bowrain JWTs issued by Keycloak.

```
Authorization: Bearer <jwt-token>
```

Protected resource metadata:

```
GET /.well-known/oauth-protected-resource
```

## Resources

| URI Template                       | Description                                            |
| ---------------------------------- | ------------------------------------------------------ |
| `brand://profiles/{id}`            | Full voice profile (tone, style, vocabulary, examples) |
| `brand://profiles/{id}/vocabulary` | Preferred/forbidden/competitor term rules              |
| `brand://profiles/{id}/examples`   | Before/after transformation examples                   |
| `brand://terminology/{workspace}`  | Workspace terminology index                            |

## Tools

### Phase 1

| Tool               | Description                                         |
| ------------------ | --------------------------------------------------- |
| `check_vocabulary` | Validate text against brand vocabulary rules        |
| `list_profiles`    | List available brand voice profiles                 |
| `get_voice_guide`  | Formatted voice guide optimized for LLM consumption |

### Phase 2

| Tool                     | Description                                         |
| ------------------------ | --------------------------------------------------- |
| `score_brand_compliance` | Full compliance check with MQM-inspired 0-100 score |
| `suggest_corrections`    | Generate specific text corrections for findings     |
| `rewrite_in_voice`       | Rewrite text to match brand voice with diff         |

## Prompts

| Prompt             | Description                                  |
| ------------------ | -------------------------------------------- |
| `write_in_voice`   | Write new content in a brand voice           |
| `rewrite_in_voice` | Rewrite existing text to match a brand voice |
| `check_draft`      | Check a draft against brand voice guidelines |

## Client Configuration

### Claude Desktop

```json
{
  "mcpServers": {
    "bowrain": {
      "url": "https://your-server.com/mcp/",
      "transport": "streamable-http"
    }
  }
}
```

### Cursor / VS Code

```json
{
  "mcp": {
    "servers": {
      "bowrain": {
        "url": "https://your-server.com/mcp/",
        "transport": "streamable-http"
      }
    }
  }
}
```

## Implementation

The MCP server is implemented in `platform/server/mcp/` using the official MCP Go SDK (`github.com/modelcontextprotocol/go-sdk`). It mounts on the existing bowrain-server Echo instance and shares the same authentication infrastructure.
