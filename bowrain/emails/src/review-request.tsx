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

interface ReviewRequestEmailProps {
  /** Human-readable workspace name. */
  workspaceName: string;
  /** The change-set's name, as its author titled it. */
  changeSetName: string;
  /** Display name of the person who proposed the change-set. */
  authorName: string;
  /** How many changes the change-set carries, already formatted. */
  changeCount: string;
  /** Deep link to the change-set's review page. */
  reviewURL: string;
}

/**
 * Branded review-request email for Bowrain.
 *
 * Sent when a governed change-set is submitted for review, to every workspace
 * member who may approve it. The change-set's author is never a recipient:
 * separation of duties bars them from reviewing their own work, so a summons
 * that reached only them would be a summons to nobody.
 *
 * Props are populated at build time with Go text/template tokens
 * (e.g. workspaceName = "{{.WorkspaceName}}") so the rendered HTML doubles as a
 * Go template. Every sentence stays static English JSX so the i18n pipeline can
 * extract it; only names, counts, and URLs arrive as tokens.
 */
export const ReviewRequestEmail = ({
  workspaceName,
  changeSetName,
  authorName,
  changeCount,
  reviewURL,
}: ReviewRequestEmailProps) => (
  <Html lang="en" dir="ltr">
    <Head />
    <Preview>A change to your workspace terms is waiting for review</Preview>
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
            <Text style={categoryBadge}>Review</Text>
          </Section>

          <Heading as="h1" style={h1}>
            A change is waiting for your review
          </Heading>

          <Text style={paragraph}>
            <strong>{authorName}</strong> proposed <strong>{changeSetName}</strong> in{" "}
            <strong>{workspaceName}</strong>.
          </Text>

          <Text style={paragraph}>
            It carries <strong>{changeCount}</strong> to the workspace terms. Governed changes take
            effect only once someone other than their author approves them, so this one is waiting
            on you.
          </Text>

          <Section style={btnWrapper}>
            <Button href={reviewURL} style={btn}>
              Review the change
            </Button>
          </Section>

          <Hr style={hr} />

          <Text style={fallback}>
            Button not working? Copy and paste this link into your browser:
          </Text>
          <Link href={reviewURL} style={link}>
            {reviewURL}
          </Link>
        </Section>

        {/* ── Footer ─────────────────────────────────── */}
        <Section style={footer}>
          <Text style={footerText}>© Bowrain. All rights reserved.</Text>
          <Text style={footerText}>
            You received this because you can approve changes in this workspace. Turn it off in
            notification preferences.
          </Text>
        </Section>
      </Container>
    </Body>
  </Html>
);

export default ReviewRequestEmail;

// ── Local styles (review-request-specific) ───────────────────────────────────

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
  backgroundColor: "#f1f5f9",
  color: "#475569",
  fontSize: "11px",
  fontWeight: "600",
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  padding: "4px 10px",
  borderRadius: "4px",
  margin: "0",
};
