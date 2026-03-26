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

interface NotificationEmailProps {
  /** Notification title, e.g. "Quality gate failed". */
  title: string;
  /** Notification body with details. */
  body: string;
  /** Category label, e.g. "Quality", "Task", "Automation". */
  category: string;
  /** "high" or "normal" — high-priority gets visual emphasis. */
  priority: string;
  /** URL to view the notification in context. */
  actionURL: string;
  /** Label for the CTA button. */
  actionLabel: string;
}

/**
 * Branded immediate notification email for Bowrain.
 *
 * Sent for urgent/high-priority notifications that need immediate attention
 * (e.g. quality gate failures, deadline approaching, flow failures).
 */
export const NotificationEmail = ({
  title,
  body,
  category,
  priority,
  actionURL,
  actionLabel,
}: NotificationEmailProps) => {
  const isHigh = priority === "high";

  return (
    <Html lang="en" dir="ltr">
      <Head />
      <Preview>
        {title}
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
            {/* Category + priority badge */}
            <Section style={badgeRow}>
              <Text style={categoryBadge}>{category}</Text>
              {isHigh && <Text style={priorityBadge}>Urgent</Text>}
            </Section>

            <Heading as="h1" style={isHigh ? { ...h1, ...h1Urgent } : h1}>
              {title}
            </Heading>

            {isHigh && <Section style={urgentBar} />}

            <Text style={paragraph}>{body}</Text>

            <Section style={btnWrapper}>
              <Button href={actionURL} style={isHigh ? btnUrgent : btn}>
                {actionLabel}
              </Button>
            </Section>

            <Hr style={hr} />

            <Text style={fallback}>
              {"Button not working? Copy and paste this link into your browser:"}
            </Text>
            <Link href={actionURL} style={link}>
              {actionURL}
            </Link>
          </Section>

          {/* ── Footer ─────────────────────────────────── */}
          <Section style={footer}>
            <Text style={footerText}>{"© Bowrain. All rights reserved."}</Text>
            <Text style={footerText}>
              {"You received this because you have email notifications enabled for this category."}
            </Text>
          </Section>
        </Container>
      </Body>
    </Html>
  );
};

export default NotificationEmail;

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

const badgeRow: React.CSSProperties = {
  marginBottom: "16px",
};

const categoryBadge: React.CSSProperties = {
  display: "inline-block",
  backgroundColor: "#f1f5f9",
  color: "#475569",
  fontSize: "11px",
  fontWeight: "600",
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  padding: "4px 10px",
  borderRadius: "4px",
  margin: "0 8px 0 0",
};

const priorityBadge: React.CSSProperties = {
  display: "inline-block",
  backgroundColor: "#fef2f2",
  color: "#dc2626",
  fontSize: "11px",
  fontWeight: "600",
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  padding: "4px 10px",
  borderRadius: "4px",
  margin: "0",
};

const h1: React.CSSProperties = {
  color: "#0f172a",
  fontSize: "24px",
  fontWeight: "700",
  margin: "0 0 16px",
  lineHeight: "1.3",
};

const h1Urgent: React.CSSProperties = {
  color: "#991b1b",
};

const urgentBar: React.CSSProperties = {
  backgroundColor: "#ef4444",
  height: "3px",
  borderRadius: "2px",
  marginBottom: "20px",
};

const paragraph: React.CSSProperties = {
  color: "#334155",
  fontSize: "16px",
  lineHeight: "1.6",
  margin: "0 0 24px",
};

const btnWrapper: React.CSSProperties = {
  margin: "0 0 28px",
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

const btnUrgent: React.CSSProperties = {
  backgroundColor: "#dc2626",
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
  margin: "0 0 20px",
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
