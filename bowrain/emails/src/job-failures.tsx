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

interface JobFailuresEmailProps {
  /** Human-readable workspace name. */
  workspaceName: string;
  /** What kind of work this was, in prose (e.g. "translation", "push"). */
  jobKind: string;
  /** How many jobs had failed for this reason when the mail was written. */
  count: string;
  /** The one-line reason the jobs share, as the worker recorded it. */
  reason: string;
  /** Deep link to where the failures can be inspected and re-run. */
  jobURL: string;
}

/**
 * Branded grouped job-failure email for Bowrain.
 *
 * Sent once for many jobs in a workspace that stopped for the same reason,
 * such as a workspace that reached its AI usage limit while a run was fanning
 * out. The single-job case uses job-failed.tsx.
 *
 * Props are populated at build time with Go text/template tokens
 * (e.g. workspaceName = "{{.WorkspaceName}}") so the rendered HTML doubles as a
 * Go template. Every sentence stays static English JSX so the i18n pipeline can
 * extract it; only names, the count, the reason, and the URL arrive as tokens.
 */
export const JobFailuresEmail = ({
  workspaceName,
  jobKind,
  count,
  reason,
  jobURL,
}: JobFailuresEmailProps) => (
  <Html lang="en" dir="ltr">
    <Head />
    <Preview>Several jobs in your workspace stopped for the same reason</Preview>
    <Body style={main}>
      <Container style={container}>
        {/* ── Header ─────────────────────────────────── */}
        <Section style={header}>
          <Text style={logoText}>Bowrain</Text>
          <Text style={tagline}>The context graph for your content</Text>
        </Section>

        {/* ── Body ───────────────────────────────────── */}
        <Section style={card}>
          <Section style={badgeRow}>
            <Text style={categoryBadge}>Failed</Text>
          </Section>

          <Heading as="h1" style={h1}>
            Several jobs stopped for the same reason
          </Heading>

          <Text style={paragraph}>
            <strong>{count}</strong> <strong>{jobKind}</strong> jobs in{" "}
            <strong>{workspaceName}</strong> did not complete. Each one stopped with this reason:
          </Text>

          <Text style={reasonBlock}>{reason}</Text>

          <Text style={paragraph}>
            This is the only email about these failures. Jobs that fail for the same reason in the
            next hour are added to one notification in the app, which keeps the running count.
          </Text>

          <Section style={btnWrapper}>
            <Button href={jobURL} style={btn}>
              Open the run history
            </Button>
          </Section>

          <Hr style={hr} />

          <Text style={fallback}>
            Button not working? Copy and paste this link into your browser:
          </Text>
          <Link href={jobURL} style={link}>
            {jobURL}
          </Link>
        </Section>

        {/* ── Footer ─────────────────────────────────── */}
        <Section style={footer}>
          <Text style={footerText}>© Bowrain. All rights reserved.</Text>
          <Text style={footerText}>
            You received this because work you are responsible for did not finish. Turn it off in
            notification preferences.
          </Text>
        </Section>
      </Container>
    </Body>
  </Html>
);

export default JobFailuresEmail;

// ── Local styles (job-failures-specific) ─────────────────────────────────────

const h1: React.CSSProperties = {
  color: "#0f172a",
  fontSize: "26px",
  fontWeight: "700",
  margin: "0 0 16px",
  lineHeight: "1.2",
};

const badgeRow: React.CSSProperties = {
  marginBottom: "16px",
};

const categoryBadge: React.CSSProperties = {
  display: "inline-block",
  backgroundColor: "#fef2f2",
  color: "#b91c1c",
  fontSize: "11px",
  fontWeight: "600",
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  padding: "4px 10px",
  borderRadius: "4px",
  margin: "0",
};

// The recorded reason is machine text, not prose: it is set apart so a reader
// can tell what the system said from what we said about it.
const reasonBlock: React.CSSProperties = {
  color: "#334155",
  fontSize: "14px",
  fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
  lineHeight: "1.5",
  backgroundColor: "#f8fafc",
  borderLeft: "3px solid #e2e8f0",
  padding: "12px 14px",
  borderRadius: "4px",
  margin: "0 0 16px",
};
