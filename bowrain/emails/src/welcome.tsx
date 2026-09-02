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

interface WelcomeEmailProps {
  /** Human-readable name of the workspace that was just created. */
  workspaceName: string;
  /** Deep link to that workspace's home. */
  workspaceURL: string;
}

/**
 * Branded welcome email for Bowrain.
 *
 * Sent once per account, when its first workspace is provisioned. The three
 * steps are the three ways into an empty workspace the dashboard itself offers,
 * in the same order, so the mail and the screen agree about what to do first.
 *
 * Props are populated at build time with Go text/template tokens
 * (e.g. workspaceName = "{{.WorkspaceName}}") so the rendered HTML doubles as a
 * Go template. Every sentence stays static English JSX so the i18n pipeline can
 * extract it; only the workspace name and the URL arrive as tokens.
 */
export const WelcomeEmail = ({ workspaceName, workspaceURL }: WelcomeEmailProps) => (
  <Html lang="en" dir="ltr">
    <Head />
    <Preview>Your Bowrain workspace is ready</Preview>
    <Body style={main}>
      <Container style={container}>
        {/* ── Header ─────────────────────────────────── */}
        <Section style={header}>
          <Text style={logoText}>Bowrain</Text>
          <Text style={tagline}>The context graph for your content</Text>
        </Section>

        {/* ── Body ───────────────────────────────────── */}
        <Section style={card}>
          <Heading as="h1" style={h1}>
            Welcome to Bowrain
          </Heading>

          <Text style={paragraph}>
            Your workspace <strong>{workspaceName}</strong> is ready.
          </Text>

          <Text style={paragraph}>
            Bowrain is the context graph people and agents plug into: the coordinates (voice,
            audience, surface, register, market, validity) that fix how a piece of content should
            read, held at workspace scope so every project draws on the same memory and adds to it.
          </Text>

          <Text style={leadIn}>Three things to do first.</Text>

          <Text style={paragraph}>
            <strong>Point an AI assistant at your material.</strong> Install the kapi skill in
            Claude, or in any assistant that reads a SKILL.md, and ask it to build a starter pack
            for this workspace. It reads your repository or your published site and proposes a voice
            profile and candidate terms for you to correct rather than author. The workspace home
            carries the prompt to copy.
          </Text>

          <Text style={paragraph}>
            <strong>Create a project.</strong> A project is one body of content, such as a
            repository, a site or a set of documents, bound to the coordinates that govern it.
            Create one from the workspace home, or connect a checkout from the command line with
            kapi init.
          </Text>

          <Text style={paragraph}>
            <strong>Run the loop and read what it proposes.</strong> kapi up converges the project
            and kapi status says where you stand. Whatever needs a person waits in this workspace's
            review queue, and what is approved there is carried back into your repository on the
            next pass.
          </Text>

          <Section style={btnWrapper}>
            <Button href={workspaceURL} style={btn}>
              Open your workspace
            </Button>
          </Section>

          <Hr style={hr} />

          <Text style={fallback}>
            Button not working? Copy and paste this link into your browser:
          </Text>
          <Link href={workspaceURL} style={link}>
            {workspaceURL}
          </Link>
        </Section>

        {/* ── Footer ─────────────────────────────────── */}
        <Section style={footer}>
          <Text style={footerText}>© Bowrain. All rights reserved.</Text>
          <Text style={footerText}>
            You received this once, because a Bowrain workspace was created for this address.
          </Text>
        </Section>
      </Container>
    </Body>
  </Html>
);

export default WelcomeEmail;

// ── Local styles (welcome-specific) ──────────────────────────────────────────

const h1: React.CSSProperties = {
  color: "#0f172a",
  fontSize: "26px",
  fontWeight: "700",
  margin: "0 0 16px",
  lineHeight: "1.2",
};

// Introduces the three steps: same size as the body, set apart by weight and by
// the space above it, so the list reads as a list without becoming a heading.
const leadIn: React.CSSProperties = {
  color: "#0f172a",
  fontSize: "16px",
  fontWeight: "600",
  lineHeight: "1.6",
  margin: "28px 0 16px",
};
