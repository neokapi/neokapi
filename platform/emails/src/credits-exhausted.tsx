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

interface CreditsExhaustedEmailProps {
  workspaceName: string;
  resetDate: string;
  upgradeURL: string;
  buyCreditsURL: string;
}

/**
 * Branded credits-exhausted email for Bowrain.
 *
 * Props are populated at build time with Go text/template tokens
 * (e.g. workspaceName = "{{.WorkspaceName}}") so the rendered HTML
 * doubles as a Go template. The mailer package fills in real values at
 * send time using text/template.Execute().
 */
export const CreditsExhaustedEmail = ({
  workspaceName,
  resetDate,
  upgradeURL,
  buyCreditsURL,
}: CreditsExhaustedEmailProps) => (
  <Html lang="en" dir="ltr">
    <Head />
    <Preview>
      {"Your AI credits are exhausted in "}
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
            {"Your AI credits are exhausted"}
          </Heading>

          <Text style={paragraph}>
            {"The workspace "}
            <strong>{workspaceName}</strong>
            {
              " has used all of its AI credits. AI-powered features such as machine translation and quality checks are paused until your credits reset."
            }
          </Text>

          <Text style={paragraph}>
            {"Your credits will automatically reset on "}
            <strong>{resetDate}</strong>
            {
              ". If you need credits sooner, you can upgrade your plan or purchase additional credits."
            }
          </Text>

          <Section style={btnWrapper}>
            <Button href={upgradeURL} style={btn}>
              Upgrade Plan
            </Button>
          </Section>

          <Section style={btnWrapper}>
            <Button href={buyCreditsURL} style={btnSecondary}>
              Buy Additional Credits
            </Button>
          </Section>

          <Hr style={hr} />

          <Text style={fallback}>{"Upgrade: "}</Text>
          <Link href={upgradeURL} style={link}>
            {upgradeURL}
          </Link>

          <Text style={fallback}>{"Buy credits: "}</Text>
          <Link href={buyCreditsURL} style={link}>
            {buyCreditsURL}
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

export default CreditsExhaustedEmail;

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

const btnSecondary: React.CSSProperties = {
  backgroundColor: "#ffffff",
  borderRadius: "8px",
  border: "1px solid #2563eb",
  color: "#2563eb",
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
