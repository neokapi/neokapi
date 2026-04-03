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
import {
  main,
  container,
  header,
  logoText,
  tagline,
  card,
  paragraph,
  btn,
  btnWrapper,
  hr,
  fallback,
  link,
  footer,
  footerText,
} from "./theme";

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
            {
              "You can view your full billing details and manage your subscription from the billing page."
            }
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

// ── Local styles (subscription-changed-specific) ─────────────────────────────

const h1: React.CSSProperties = {
  color: "#0f172a",
  fontSize: "26px",
  fontWeight: "700",
  margin: "0 0 16px",
  lineHeight: "1.2",
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
