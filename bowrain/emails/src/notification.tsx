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
  btn as btnBase,
  hr,
  fallback,
  link,
  footer,
  footerText,
} from "./theme";

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
      <Preview>{title}</Preview>
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

            <Section style={btnWrapperLocal}>
              <Button href={actionURL} style={isHigh ? btnUrgent : btnBase}>
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

// ── Local styles (notification-specific) ─────────────────────────────────────

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

const btnWrapperLocal: React.CSSProperties = {
  margin: "0 0 28px",
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
