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

interface TaskAssignedEmailProps {
  /** Human-readable workspace name. */
  workspaceName: string;
  /** The task's title, as whoever opened it wrote it. */
  taskTitle: string;
  /** The task's description, or an empty string when it has none. */
  taskDescription: string;
  /** How urgent the task is, in prose ("high", "urgent"). */
  priority: string;
  /** Display name of whoever assigned it. */
  assignerName: string;
  /** Deep link to the workspace task queue. */
  taskURL: string;
}

/**
 * Branded task-assignment email for Bowrain.
 *
 * Sent only for tasks marked high or urgent. Routine assignments stay in-app and
 * on the badge: a queue that mails on every item trains its readers to ignore
 * it, and then the urgent one arrives looking like all the others.
 *
 * Props are populated at build time with Go text/template tokens
 * (e.g. workspaceName = "{{.WorkspaceName}}") so the rendered HTML doubles as a
 * Go template. Every sentence stays static English JSX so the i18n pipeline can
 * extract it; only names, the priority, and the URL arrive as tokens.
 */
export const TaskAssignedEmail = ({
  workspaceName,
  taskTitle,
  taskDescription,
  priority,
  assignerName,
  taskURL,
}: TaskAssignedEmailProps) => (
  <Html lang="en" dir="ltr">
    <Head />
    <Preview>A task in your workspace is waiting on you</Preview>
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
            <Text style={categoryBadge}>{priority}</Text>
          </Section>

          <Heading as="h1" style={h1}>
            A task is waiting on you
          </Heading>

          <Text style={paragraph}>
            <strong>{assignerName}</strong> assigned you <strong>{taskTitle}</strong> in{" "}
            <strong>{workspaceName}</strong>.
          </Text>

          <Text style={paragraph}>{taskDescription}</Text>

          <Text style={paragraph}>
            You are hearing about this one by mail because it was marked urgent. Everything else
            waits quietly in your queue.
          </Text>

          <Section style={btnWrapper}>
            <Button href={taskURL} style={btn}>
              Open the task
            </Button>
          </Section>

          <Hr style={hr} />

          <Text style={fallback}>
            Button not working? Copy and paste this link into your browser:
          </Text>
          <Link href={taskURL} style={link}>
            {taskURL}
          </Link>
        </Section>

        {/* ── Footer ─────────────────────────────────── */}
        <Section style={footer}>
          <Text style={footerText}>© Bowrain. All rights reserved.</Text>
          <Text style={footerText}>
            You received this because an urgent task was assigned to you. Turn it off in
            notification preferences.
          </Text>
        </Section>
      </Container>
    </Body>
  </Html>
);

export default TaskAssignedEmail;

// ── Local styles (task-assigned-specific) ────────────────────────────────────

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
  backgroundColor: "#fff7ed",
  color: "#c2410c",
  fontSize: "11px",
  fontWeight: "600",
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  padding: "4px 10px",
  borderRadius: "4px",
  margin: "0",
};
