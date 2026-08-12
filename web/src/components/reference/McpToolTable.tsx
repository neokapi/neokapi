import React from "react";
import { mcpTools } from "@neokapi/reference-data";
import type { MCPSurface, MCPTool } from "@neokapi/reference-data";

/**
 * The generated MCP reference, rendered from mcp-tools.json — which gen-refs
 * produces by connecting to a real `kapi mcp` server and issuing tools/list and
 * resources/templates/list, and which a CI drift gate keeps honest.
 *
 * Nothing here is written by hand. Register, retire, or reword a tool or an
 * address and this page follows, or the build fails.
 */

function bySurface(surface: MCPSurface): MCPTool[] {
  return mcpTools.tools.filter((t) => t.surface === surface);
}

/** The number of tools on one surface, so prose never hardcodes a count. */
export function McpToolCount({ surface = "default" }: { surface?: MCPSurface }) {
  return <>{bySurface(surface).length}</>;
}

/** Every tool on one surface, as a name/description table. */
export function McpToolList({ surface = "default" }: { surface?: MCPSurface }) {
  return (
    <table>
      <thead>
        <tr>
          <th>Tool</th>
          <th>What it does</th>
        </tr>
      </thead>
      <tbody>
        {bySurface(surface).map((tool) => (
          <tr key={tool.name}>
            <td>
              <code id={`tool-${tool.name}`}>{tool.name}</code>
            </td>
            <td>{tool.description}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/** The tool names on one surface, as an inline comma-separated code list. */
export function McpToolNames({ surface }: { surface: MCPSurface }) {
  const names = bySurface(surface).map((t) => t.name);
  return (
    <>
      {names.map((name, i) => (
        <React.Fragment key={name}>
          {i > 0 ? ", " : ""}
          <code>{name}</code>
        </React.Fragment>
      ))}
    </>
  );
}

/** One tool's input schema, as a parameter table. */
export function McpToolParams({ name }: { name: string }) {
  const tool = mcpTools.tools.find((t) => t.name === name);
  if (!tool) {
    throw new Error(`McpToolParams: no MCP tool named "${name}" in the generated reference`);
  }
  const params = tool.params ?? [];
  if (params.length === 0) {
    return <p>Takes no arguments.</p>;
  }
  return (
    <table>
      <thead>
        <tr>
          <th>Parameter</th>
          <th>Type</th>
          <th>Required</th>
          <th>Description</th>
        </tr>
      </thead>
      <tbody>
        {params.map((param) => (
          <tr key={param.name}>
            <td>
              <code>{param.name}</code>
            </td>
            <td>{param.type || "—"}</td>
            <td>{param.required ? "yes" : "no"}</td>
            <td>{param.description ?? ""}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/** Every address the server answers reads at, with the rendering it carries. */
export function McpResourceList() {
  const resources = mcpTools.resources ?? [];
  return (
    <table>
      <thead>
        <tr>
          <th>Address</th>
          <th>Renders as</th>
          <th>What it answers</th>
        </tr>
      </thead>
      <tbody>
        {resources.map((resource) => (
          <tr key={resource.uriTemplate}>
            <td>
              <code id={`resource-${resource.name}`}>{resource.uriTemplate}</code>
            </td>
            <td>
              <code>{resource.mimeType}</code>
            </td>
            <td>{resource.description}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/** One tool's description, so prose can quote the registration verbatim. */
export function McpToolDescription({ name }: { name: string }) {
  const tool = mcpTools.tools.find((t) => t.name === name);
  if (!tool) {
    throw new Error(`McpToolDescription: no MCP tool named "${name}" in the generated reference`);
  }
  return <>{tool.description}</>;
}
