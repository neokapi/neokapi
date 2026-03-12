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

interface InviteEmailProps {
  workspaceName: string;
  role: string;
  joinURL: string;
}

/**
 * Branded invitation email for Bowrain.
 *
 * Props are populated at build time with Go text/template tokens
 * (e.g. workspaceName = "{{.WorkspaceName}}") so the rendered HTML
 * doubles as a Go template. The mailer package fills in real values at
 * send time using text/template.Execute().
 */
export const InviteEmail = ({
  workspaceName,
  role,
  joinURL,
}: InviteEmailProps) => (
  <Html lang="en" dir="ltr">
    <Head />
    <Preview>
      {"You've been invited to join "}
      {workspaceName}
      {" on Bowrain"}
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
            {"You're Invited 🎉"}
          </Heading>

          <Text style={paragraph}>
            {"You've been invited to join "}
            <strong>{workspaceName}</strong>
            {" on Bowrain as "}
            <strong>{role}</strong>.
          </Text>

          <Text style={paragraph}>
            Click the button below to accept the invitation and get started.
          </Text>

          <Section style={btnWrapper}>
            <Button href={joinURL} style={btn}>
              Accept Invitation
            </Button>
          </Section>

          <Hr style={hr} />

          <Text style={fallback}>
            {"Button not working? Copy and paste this link into your browser:"}
          </Text>
          <Link href={joinURL} style={link}>
            {joinURL}
          </Link>
        </Section>

        {/* ── Footer ─────────────────────────────────── */}
        <Section style={footer}>
          <Text style={footerText}>{"© Bowrain. All rights reserved."}</Text>
          <Text style={footerText}>
            {
              "If you didn't request this invitation, you can safely ignore this email."
            }
          </Text>
        </Section>
      </Container>
    </Body>
  </Html>
);

export default InviteEmail;

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
