import {
  Body,
  Button,
  Container,
  Head,
  Heading,
  Hr,
  Html,
  Link,
  Preview,
  Section,
  Text,
} from "@react-email/components";
import * as React from "react";

interface SubscriptionChangedEmailProps {
  workspaceName: string;
  planName: string;
  status: string;
  billingURL: string;
}

/**
 * Branded subscription-changed email for Bowrain.
 *
 * Props are populated at build time with Go text/template tokens
 * (e.g. workspaceName = "{{.WorkspaceName}}") so the rendered HTML
 * doubles as a Go template. The mailer package fills in real values at
 * send time using text/template.Execute().
 */
export const SubscriptionChangedEmail = ({
  workspaceName,
  planName,
  status,
  billingURL,
}: SubscriptionChangedEmailProps) => (
  <Html lang="en" dir="ltr">
    <Head />
    <Preview>
      {"Your subscription has been updated for "}
      {workspaceName}
    </Preview>
    <Body style={main}>
      <Container style={container}>
        {/* ── Header ─────────────────────────────────── */}
        <Section style={header}>
          <Text style={logoText}>Bowrain</Text>
          <Text style={tagline}>Localization platform</Text>
        </Section>

        {/* ── Body ───────────────────────────────────── */}
        <Section style={card}>
          <Heading as="h1" style={h1}>
            {"Your subscription has been updated"}
          </Heading>

          <Text style={paragraph}>
            {"The subscription for "}
            <strong>{workspaceName}</strong>
            {" has been updated. Here are the details:"}
          </Text>

          {/* ── Plan details ─────────────────────────── */}
          <Section style={detailsBox}>
            <Text style={detailLabel}>Plan</Text>
            <Text style={detailValue}>{planName}</Text>
            <Text style={detailLabel}>Status</Text>
            <Text style={detailValue}>{status}</Text>
          </Section>

          <Text style={paragraph}>
            {"You can view your full billing details and manage your subscription from the billing page."}
          </Text>

          <Section style={btnWrapper}>
            <Button href={billingURL} style={btn}>
              View Billing
            </Button>
          </Section>

          <Hr style={hr} />

          <Text style={fallback}>
            {"Button not working? Copy and paste this link into your browser:"}
          </Text>
          <Link href={billingURL} style={link}>
            {billingURL}
          </Link>
        </Section>

        {/* ── Footer ─────────────────────────────────── */}
        <Section style={footer}>
          <Text style={footerText}>{"© Bowrain. All rights reserved."}</Text>
          <Text style={footerText}>
            {"You received this email because you are an admin of this workspace."}
          </Text>
        </Section>
      </Container>
    </Body>
  </Html>
);

export default SubscriptionChangedEmail;

// ── Styles ────────────────────────────────────────────────────────────────────

const main: React.CSSProperties = {
  backgroundColor: "#f1f5f9",
  fontFamily:
    '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif',
};

const container: React.CSSProperties = {
  maxWidth: "560px",
  margin: "40px auto",
  padding: "0",
};

const header: React.CSSProperties = {
  backgroundColor: "#0f172a",
  borderRadius: "12px 12px 0 0",
  padding: "28px 32px 24px",
};

const logoText: React.CSSProperties = {
  color: "#f8fafc",
  fontSize: "22px",
  fontWeight: "700",
  margin: "0 0 2px",
  letterSpacing: "-0.3px",
};

const tagline: React.CSSProperties = {
  color: "#94a3b8",
  fontSize: "13px",
  margin: "0",
};

const card: React.CSSProperties = {
  backgroundColor: "#ffffff",
  padding: "40px 32px 32px",
};

const h1: React.CSSProperties = {
  color: "#0f172a",
  fontSize: "26px",
  fontWeight: "700",
  margin: "0 0 16px",
  lineHeight: "1.2",
};

const paragraph: React.CSSProperties = {
  color: "#334155",
  fontSize: "16px",
  lineHeight: "1.6",
  margin: "0 0 16px",
};

const detailsBox: React.CSSProperties = {
  backgroundColor: "#f8fafc",
  borderRadius: "8px",
  border: "1px solid #e2e8f0",
  padding: "20px 24px",
  margin: "0 0 16px",
};

const detailLabel: React.CSSProperties = {
  color: "#64748b",
  fontSize: "13px",
  fontWeight: "600",
  margin: "0 0 2px",
  textTransform: "uppercase",
  letterSpacing: "0.5px",
};

const detailValue: React.CSSProperties = {
  color: "#0f172a",
  fontSize: "16px",
  fontWeight: "600",
  margin: "0 0 12px",
};

const btnWrapper: React.CSSProperties = {
  margin: "28px 0",
};

const btn: React.CSSProperties = {
  backgroundColor: "#2563eb",
  borderRadius: "8px",
  color: "#ffffff",
  display: "inline-block",
  fontSize: "15px",
  fontWeight: "600",
  padding: "14px 28px",
  textDecoration: "none",
  lineHeight: "1",
};

const hr: React.CSSProperties = {
  borderColor: "#e2e8f0",
  borderTopWidth: "1px",
  margin: "28px 0 20px",
};

const fallback: React.CSSProperties = {
  color: "#64748b",
  fontSize: "13px",
  margin: "0 0 6px",
};

const link: React.CSSProperties = {
  color: "#2563eb",
  fontSize: "13px",
  wordBreak: "break-all",
};

const footer: React.CSSProperties = {
  backgroundColor: "#f8fafc",
  borderRadius: "0 0 12px 12px",
  borderTop: "1px solid #e2e8f0",
  padding: "20px 32px",
};

const footerText: React.CSSProperties = {
  color: "#94a3b8",
  fontSize: "12px",
  lineHeight: "1.5",
  margin: "0 0 4px",
};
