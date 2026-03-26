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

interface CreditsWarningEmailProps {
  workspaceName: string;
  usedCredits: string;
  totalCredits: string;
  usagePercent: string;
  resetDate: string;
  upgradeURL: string;
}

/**
 * Branded credits-warning email for Bowrain.
 *
 * Props are populated at build time with Go text/template tokens
 * (e.g. workspaceName = "{{.WorkspaceName}}") so the rendered HTML
 * doubles as a Go template. The mailer package fills in real values at
 * send time using text/template.Execute().
 */
export const CreditsWarningEmail = ({
  workspaceName,
  usedCredits,
  totalCredits,
  usagePercent,
  resetDate,
  upgradeURL,
}: CreditsWarningEmailProps) => (
  <Html lang="en" dir="ltr">
    <Head />
    <Preview>
      {"Your AI credits are running low in "}
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
            {"Your AI credits are running low"}
          </Heading>

          <Text style={paragraph}>
            {"The workspace "}
            <strong>{workspaceName}</strong>
            {" has used "}
            <strong>
              {usedCredits}
              {" of "}
              {totalCredits}
            </strong>
            {" AI credits ("}
            {usagePercent}
            {"%)."}
          </Text>

          {/* ── Usage bar ────────────────────────────── */}
          <Section style={barOuter}>
            <Section style={barInner} />
          </Section>

          <Text style={paragraph}>
            {"Your credits will reset on "}
            <strong>{resetDate}</strong>
            {". To avoid interruption, consider upgrading your plan for a higher credit allowance."}
          </Text>

          <Section style={btnWrapper}>
            <Button href={upgradeURL} style={btn}>
              Upgrade Plan
            </Button>
          </Section>

          <Hr style={hr} />

          <Text style={fallback}>
            {"Button not working? Copy and paste this link into your browser:"}
          </Text>
          <Link href={upgradeURL} style={link}>
            {upgradeURL}
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

export default CreditsWarningEmail;

// ── Local styles (credits-warning-specific) ──────────────────────────────────

const h1: React.CSSProperties = {
  color: "#0f172a",
  fontSize: "26px",
  fontWeight: "700",
  margin: "0 0 16px",
  lineHeight: "1.2",
};

const barOuter: React.CSSProperties = {
  backgroundColor: "#e2e8f0",
  borderRadius: "6px",
  height: "12px",
  margin: "0 0 16px",
  overflow: "hidden",
};

const barInner: React.CSSProperties = {
  backgroundColor: "#f59e0b",
  borderRadius: "6px",
  height: "12px",
  width: "80%",
};
