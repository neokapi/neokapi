/**
 * render.ts — pre-renders React Email templates to HTML files consumed by
 * the Go mailer package (bowrain/mailer/).
 *
 * Each template is rendered with Go text/template tokens as prop values
 * (e.g. "{{.WorkspaceName}}"). React Email outputs these literally because
 * { and } are not HTML-special characters. At runtime the Go mailer calls
 * text/template.Execute() to fill in the real values.
 *
 * Run:  npm run build   (from bowrain/emails/)
 * Make: make email-build
 */

import { render } from "@react-email/render";
import { mkdirSync, writeFileSync } from "fs";
import { dirname, resolve } from "path";
import { fileURLToPath } from "url";
import InviteEmail from "../src/invite.js";

const __dirname = dirname(fileURLToPath(import.meta.url));

// Output directory: bowrain/mailer/templates/ (relative to this script)
const outDir = resolve(__dirname, "../../mailer/templates");
mkdirSync(outDir, { recursive: true });

async function buildTemplates(): Promise<void> {
  // ── Invite email ──────────────────────────────────────────────────────────
  //
  // Props are the Go text/template tokens that will be substituted at
  // send time. React renders them as literal strings in the HTML output.
  const inviteHtml = await render(
    InviteEmail({
      workspaceName: "{{.WorkspaceName}}",
      role: "{{.Role}}",
      joinURL: "{{.JoinURL}}",
    }),
    { pretty: false },
  );

  writeFileSync(resolve(outDir, "invite.html"), inviteHtml, "utf-8");
  console.log("✓  Rendered invite.html");

  console.log(`\nAll templates written to ${outDir}`);
}

buildTemplates().catch((err: unknown) => {
  console.error("Template render failed:", err);
  process.exit(1);
});
