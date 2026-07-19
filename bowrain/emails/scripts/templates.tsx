/**
 * templates.tsx — renders every React Email template to HTML with Go
 * text/template tokens as prop values (e.g. "{{.WorkspaceName}}"). React
 * Email outputs these literally because { and } are not HTML-special
 * characters; at runtime the Go mailer calls text/template.Execute() to
 * fill in the real values.
 *
 * This module is the single source of truth for the template list. It is
 * consumed two ways by scripts/render.ts:
 *
 *  - imported directly (via tsx) for the English render, and
 *  - bundled per target locale with esbuild + the @neokapi/i18n-react
 *    esbuild plugin in inline mode, which bakes the locale's compiled
 *    catalog (translations/<locale>.json) into the JSX before rendering.
 */

import { render } from "@react-email/render";
import InviteEmail from "../src/invite.js";
import CreditsWarningEmail from "../src/credits-warning.js";
import CreditsExhaustedEmail from "../src/credits-exhausted.js";
import PaymentFailedEmail from "../src/payment-failed.js";
import SubscriptionChangedEmail from "../src/subscription-changed.js";
import NotificationEmail from "../src/notification.js";
import DigestEmail from "../src/digest.js";
import EmailChangeVerifyEmail from "../src/email-change-verify.js";

/** Renders every template; returns file name (sans .html) → HTML. */
export async function renderTemplates(): Promise<Record<string, string>> {
  const out: Record<string, string> = {};

  out["invite"] = await render(
    InviteEmail({
      workspaceName: "{{.WorkspaceName}}",
      role: "{{.Role}}",
      joinURL: "{{.JoinURL}}",
    }),
    { pretty: false },
  );

  out["credits-warning"] = await render(
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

  out["credits-exhausted"] = await render(
    CreditsExhaustedEmail({
      workspaceName: "{{.WorkspaceName}}",
      resetDate: "{{.ResetDate}}",
      upgradeURL: "{{.UpgradeURL}}",
      buyCreditsURL: "{{.BuyCreditsURL}}",
    }),
    { pretty: false },
  );

  out["payment-failed"] = await render(
    PaymentFailedEmail({
      workspaceName: "{{.WorkspaceName}}",
      invoiceAmount: "{{.InvoiceAmount}}",
      currency: "{{.Currency}}",
      updatePaymentURL: "{{.UpdatePaymentURL}}",
    }),
    { pretty: false },
  );

  out["subscription-changed"] = await render(
    SubscriptionChangedEmail({
      workspaceName: "{{.WorkspaceName}}",
      planName: "{{.PlanName}}",
      status: "{{.Status}}",
      billingURL: "{{.BillingURL}}",
    }),
    { pretty: false },
  );

  out["notification"] = await render(
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

  // The digest template uses Go range/if blocks for dynamic content.
  // We render a single-item placeholder; the Go mailer replaces the body
  // section with range-generated HTML at send time.
  out["digest"] = await render(
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

  out["email-change-verify"] = await render(
    EmailChangeVerifyEmail({
      newEmail: "{{.NewEmail}}",
      confirmURL: "{{.ConfirmURL}}",
      expiresIn: "{{.ExpiresIn}}",
    }),
    { pretty: false },
  );

  return out;
}
