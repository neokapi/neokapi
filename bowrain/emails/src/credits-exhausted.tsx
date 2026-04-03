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

// ── Local styles (credits-exhausted-specific) ────────────────────────────────

const h1: React.CSSProperties = {
  color: "#0f172a",
  fontSize: "26px",
  fontWeight: "700",
  margin: "0 0 16px",
  lineHeight: "1.2",
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
