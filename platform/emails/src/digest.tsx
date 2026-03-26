import {
  Body,
  Button,
  Container,
  Head,
  Heading,
  Hr,
  Html,
  Preview,
  Section,
  Text,
} from "@react-email/components";
import * as React from "react";

/** A single notification item within a digest category group. */
interface DigestItem {
  title: string;
  body: string;
  priority?: "high" | "normal";
}

/** A group of notifications under a single category. */
interface DigestGroup {
  category: string;
  label: string;
  items: DigestItem[];
}

interface DigestEmailProps {
  /** "daily" or "weekly" */
  frequency: string;
  /** Total number of notifications in this digest. */
  totalUpdates: string;
  /** Grouped notification items, rendered as JSON in Go template mode. */
  groups: DigestGroup[];
  /** URL to notification settings. */
  settingsURL: string;
  /** URL to open the dashboard. */
  dashboardURL: string;
}

/**
 * Branded digest email for Bowrain.
 *
 * In Storybook mode, groups are passed as a real array of DigestGroup.
 * In Go template mode, the entire body section is generated server-side
 * (the Go mailer iterates over notification groups). This component
 * serves as the visual reference and Storybook preview.
 */
export const DigestEmail = ({
  frequency,
  totalUpdates,
  groups,
  settingsURL,
  dashboardURL,
}: DigestEmailProps) => {
  const isWeekly = frequency === "weekly";
  const title = isWeekly ? "Weekly Summary" : "Daily Digest";

  return (
    <Html lang="en" dir="ltr">
      <Head />
      <Preview>
        {title}
        {" — "}
        {totalUpdates}
        {" updates"}
      </Preview>
      <Body style={main}>
        <Container style={container}>
          {/* ── Header ─────────────────────────────────── */}
          <Section style={header}>
            <Text style={logoText}>Bowrain</Text>
            <Text style={tagline}>Localization platform</Text>
          </Section>

          {/* ── Title bar ──────────────────────────────── */}
          <Section style={titleBar}>
            <Heading as="h1" style={titleH1}>
              {title}
            </Heading>
            <Text style={subtitle}>
              {totalUpdates}
              {" new updates"}
            </Text>
          </Section>

          {/* ── Body ───────────────────────────────────── */}
          <Section style={card}>
            {groups.map((group) => (
              <Section key={group.category} style={categorySection}>
                <Text style={categoryHeader}>
                  {group.label}
                  {" ("}
                  {group.items.length}
                  {")"}
                </Text>
                {group.items.map((item, idx) => (
                  <Section
                    key={idx}
                    style={
                      item.priority === "high"
                        ? { ...itemRow, ...itemHighPriority }
                        : itemRow
                    }
                  >
                    <Text style={itemTitle}>{item.title}</Text>
                    <Text style={itemBody}>{item.body}</Text>
                  </Section>
                ))}
              </Section>
            ))}

            <Hr style={hr} />

            <Section style={btnWrapper}>
              <Button href={dashboardURL} style={btn}>
                Open Dashboard
              </Button>
            </Section>
          </Section>

          {/* ── Footer ─────────────────────────────────── */}
          <Section style={footer}>
            <Text style={footerText}>
              {"You can change your digest frequency in "}
              <a href={settingsURL} style={footerLink}>
                notification settings
              </a>
              .
            </Text>
            <Text style={footerText}>{"© Bowrain. All rights reserved."}</Text>
          </Section>
        </Container>
      </Body>
    </Html>
  );
};

export default DigestEmail;

// ── Styles ────────────────────────────────────────────────────────────────────

const main: React.CSSProperties = {
  backgroundColor: "#f1f5f9",
  fontFamily:
    '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif',
};

const container: React.CSSProperties = {
  maxWidth: "600px",
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

const titleBar: React.CSSProperties = {
  backgroundColor: "#1e293b",
  padding: "20px 32px",
};

const titleH1: React.CSSProperties = {
  color: "#f8fafc",
  fontSize: "20px",
  fontWeight: "600",
  margin: "0",
};

const subtitle: React.CSSProperties = {
  color: "#94a3b8",
  fontSize: "14px",
  margin: "4px 0 0",
};

const card: React.CSSProperties = {
  backgroundColor: "#ffffff",
  padding: "24px 32px 32px",
};

const categorySection: React.CSSProperties = {
  marginBottom: "20px",
};

const categoryHeader: React.CSSProperties = {
  fontSize: "13px",
  fontWeight: "600",
  color: "#64748b",
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  margin: "0 0 12px",
  paddingBottom: "8px",
  borderBottom: "1px solid #f1f5f9",
};

const itemRow: React.CSSProperties = {
  marginBottom: "12px",
};

const itemHighPriority: React.CSSProperties = {
  borderLeft: "3px solid #ef4444",
  paddingLeft: "12px",
};

const itemTitle: React.CSSProperties = {
  margin: "0",
  fontSize: "14px",
  fontWeight: "500",
  color: "#0f172a",
};

const itemBody: React.CSSProperties = {
  margin: "2px 0 0",
  fontSize: "13px",
  color: "#64748b",
};

const hr: React.CSSProperties = {
  borderColor: "#e2e8f0",
  borderTopWidth: "1px",
  margin: "24px 0 20px",
};

const btnWrapper: React.CSSProperties = {
  textAlign: "center" as const,
  margin: "0",
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

const footerLink: React.CSSProperties = {
  color: "#2563eb",
  textDecoration: "underline",
};
