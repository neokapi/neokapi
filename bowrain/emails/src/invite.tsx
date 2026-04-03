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
export const InviteEmail = ({ workspaceName, role, joinURL }: InviteEmailProps) => (
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
            {"If you didn't request this invitation, you can safely ignore this email."}
          </Text>
        </Section>
      </Container>
    </Body>
  </Html>
);

export default InviteEmail;

// ── Local styles (invite-specific) ───────────────────────────────────────────

const h1: React.CSSProperties = {
  color: "#0f172a",
  fontSize: "26px",
  fontWeight: "700",
  margin: "0 0 16px",
  lineHeight: "1.2",
};
