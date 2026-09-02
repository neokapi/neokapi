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

interface JobFailedEmailProps {
  /** Human-readable workspace name. */
  workspaceName: string;
  /** What kind of work this was, in prose (e.g. "translation", "push"). */
  jobKind: string;
  /** What the job was working on — an item name, or the project's name. */
  subject: string;
  /** The one-line reason the job stopped, as the worker recorded it. */
  reason: string;
  /** Deep link to where the failure can be inspected and re-run. */
  jobURL: string;
}

/**
 * Branded job-failure email for Bowrain.
 *
 * Sent once per failed job — after retries, not per attempt — to the person who
 * asked for the work, or to the workspace's owners when the platform started it
 * itself. Work that stops is the one outcome nobody discovers on their own: a
 * job that silently failed looks exactly like a job that is still running.
 *
 * Props are populated at build time with Go text/template tokens
 * (e.g. workspaceName = "{{.WorkspaceName}}") so the rendered HTML doubles as a
 * Go template. Every sentence stays static English JSX so the i18n pipeline can
 * extract it; only names, the reason, and the URL arrive as tokens.
 */
export const JobFailedEmail = ({
  workspaceName,
  jobKind,
  subject,
  reason,
  jobURL,
}: JobFailedEmailProps) => (
  <Html lang="en" dir="ltr">
    <Head />
    <Preview>Work in your workspace stopped before it finished</Preview>
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
            A job stopped before it finished
          </Heading>

          <Text style={paragraph}>
            A <strong>{jobKind}</strong> job for <strong>{subject}</strong> in{" "}
            <strong>{workspaceName}</strong> did not complete.
          </Text>

          <Text style={reasonBlock}>{reason}</Text>

          <Text style={paragraph}>
            The work was retried where retrying could help, and stopped when it could not. Nothing
            was left half-applied, and the content it was working on is unchanged.
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

export default JobFailedEmail;

// ── Local styles (job-failed-specific) ───────────────────────────────────────

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
