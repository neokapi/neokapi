/**
 * render.ts — pre-renders React Email templates to HTML files consumed by
 * the Go mailer package (platform/mailer/).
 *
 * Each template is rendered with Go text/template tokens as prop values
 * (e.g. "{{.WorkspaceName}}"). React Email outputs these literally because
 * { and } are not HTML-special characters. At runtime the Go mailer calls
 * text/template.Execute() to fill in the real values.
 *
 * Run:  vp run build   (from bowrain/emails/)
 * Make: make email-build
 */

import { render } from "@react-email/render";
import { mkdirSync, writeFileSync } from "fs";
import { dirname, resolve } from "path";
import { fileURLToPath } from "url";
import InviteEmail from "../src/invite.js";
import CreditsWarningEmail from "../src/credits-warning.js";
import CreditsExhaustedEmail from "../src/credits-exhausted.js";
import PaymentFailedEmail from "../src/payment-failed.js";
import SubscriptionChangedEmail from "../src/subscription-changed.js";
import NotificationEmail from "../src/notification.js";
import DigestEmail from "../src/digest.js";
import EmailChangeVerifyEmail from "../src/email-change-verify.js";

const __dirname = dirname(fileURLToPath(import.meta.url));

// Output directory: platform/mailer/templates/ (relative to this script)
const outDir = resolve(__dirname, "../../mailer/templates");
mkdirSync(outDir, { recursive: true });

async function buildTemplates(): Promise<void> {
  // ── Invite email ──────────────────────────────────────────────────────────
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

  // ── Credits warning email ─────────────────────────────────────────────────
  const creditsWarningHtml = await render(
    CreditsWarningEmail({
      workspaceName: "{{.WorkspaceName}}",
      usedCredits: "{{.UsedCredits}}",
      totalCredits: "{{.TotalCredits}}",
      usagePercent: "{{.UsagePercent}}",
      resetDate: "{{.ResetDate}}",
      upgradeURL: "{{.UpgradeURL}}",
    }),
    { pretty: false },
  );
  writeFileSync(resolve(outDir, "credits-warning.html"), creditsWarningHtml, "utf-8");
  console.log("✓  Rendered credits-warning.html");

  // ── Credits exhausted email ───────────────────────────────────────────────
  const creditsExhaustedHtml = await render(
    CreditsExhaustedEmail({
      workspaceName: "{{.WorkspaceName}}",
      resetDate: "{{.ResetDate}}",
      upgradeURL: "{{.UpgradeURL}}",
      buyCreditsURL: "{{.BuyCreditsURL}}",
    }),
    { pretty: false },
  );
  writeFileSync(resolve(outDir, "credits-exhausted.html"), creditsExhaustedHtml, "utf-8");
  console.log("✓  Rendered credits-exhausted.html");

  // ── Payment failed email ──────────────────────────────────────────────────
  const paymentFailedHtml = await render(
    PaymentFailedEmail({
      workspaceName: "{{.WorkspaceName}}",
      invoiceAmount: "{{.InvoiceAmount}}",
      currency: "{{.Currency}}",
      updatePaymentURL: "{{.UpdatePaymentURL}}",
    }),
    { pretty: false },
  );
  writeFileSync(resolve(outDir, "payment-failed.html"), paymentFailedHtml, "utf-8");
  console.log("✓  Rendered payment-failed.html");

  // ── Subscription changed email ────────────────────────────────────────────
  const subscriptionChangedHtml = await render(
    SubscriptionChangedEmail({
      workspaceName: "{{.WorkspaceName}}",
      planName: "{{.PlanName}}",
      status: "{{.Status}}",
      billingURL: "{{.BillingURL}}",
    }),
    { pretty: false },
  );
  writeFileSync(resolve(outDir, "subscription-changed.html"), subscriptionChangedHtml, "utf-8");
  console.log("✓  Rendered subscription-changed.html");

  // ── Notification immediate email ───────────────────────────────────────────
  const notificationHtml = await render(
    NotificationEmail({
      title: "{{.Title}}",
      body: "{{.Body}}",
      category: "{{.Category}}",
      priority: "{{.Priority}}",
      actionURL: "{{.ActionURL}}",
      actionLabel: "{{.ActionLabel}}",
    }),
    { pretty: false },
  );
  writeFileSync(resolve(outDir, "notification.html"), notificationHtml, "utf-8");
  console.log("✓  Rendered notification.html");

  // ── Digest email (daily/weekly) ─────────────────────────────────────────────
  // The digest template uses Go range/if blocks for dynamic content.
  // We render a single-item placeholder; the Go mailer replaces the body
  // section with range-generated HTML at send time.
  const digestHtml = await render(
    DigestEmail({
      frequency: "{{.Frequency}}",
      totalUpdates: "{{.TotalUpdates}}",
      groups: [
        {
          category: "placeholder",
          label: "{{.GroupLabel}}",
          items: [{ title: "{{.ItemTitle}}", body: "{{.ItemBody}}" }],
        },
      ],
      settingsURL: "{{.SettingsURL}}",
      dashboardURL: "{{.DashboardURL}}",
    }),
    { pretty: false },
  );
  writeFileSync(resolve(outDir, "digest.html"), digestHtml, "utf-8");
  console.log("✓  Rendered digest.html");

  // ── Email change verification ─────────────────────────────────────────────
  const emailChangeHtml = await render(
    EmailChangeVerifyEmail({
      newEmail: "{{.NewEmail}}",
      confirmURL: "{{.ConfirmURL}}",
      expiresIn: "{{.ExpiresIn}}",
    }),
    { pretty: false },
  );
  writeFileSync(resolve(outDir, "email-change-verify.html"), emailChangeHtml, "utf-8");
  console.log("✓  Rendered email-change-verify.html");

  console.log(`\nAll templates written to ${outDir}`);
}

buildTemplates().catch((err: unknown) => {
  console.error("Template render failed:", err);
  process.exit(1);
});
