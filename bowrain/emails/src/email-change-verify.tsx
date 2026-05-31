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

interface EmailChangeVerifyProps {
  newEmail: string;
  confirmURL: string;
  expiresIn: string;
}

/**
 * Bowrain email-change verification mail. Sent to the *new* address the user
 * proposed; clicking the button confirms ownership and finalizes the change.
 */
export const EmailChangeVerifyEmail = ({
  newEmail,
  confirmURL,
  expiresIn,
}: EmailChangeVerifyProps) => (
  <Html lang="en" dir="ltr">
    <Head />
    <Preview>
      {"Confirm your new Bowrain email "}
      {newEmail}
    </Preview>
    <Body style={main}>
      <Container style={container}>
        <Section style={header}>
          <Text style={logoText}>Bowrain</Text>
          <Text style={tagline}>Localization platform</Text>
        </Section>

        <Section style={card}>
          <Heading as="h1" style={h1}>
            {"Confirm your new email"}
          </Heading>

          <Text style={paragraph}>
            {"Someone — hopefully you — asked to change a Bowrain account's email to "}
            <strong>{newEmail}</strong>
            {". Click the button below to confirm and finish the switch."}
          </Text>

          <Section style={btnWrapper}>
            <Button href={confirmURL} style={btn}>
              Confirm email change
            </Button>
          </Section>

          <Hr style={hr} />

          <Text style={fallback}>
            {"Button not working? Copy and paste this link into your browser:"}
          </Text>
          <Link href={confirmURL} style={link}>
            {confirmURL}
          </Link>

          <Text style={paragraph}>
            {"This link expires in "}
            {expiresIn}
            {". After confirmation, you'll need to sign in again with your new email."}
          </Text>
        </Section>

        <Section style={footer}>
          <Text style={footerText}>{"© Bowrain. All rights reserved."}</Text>
          <Text style={footerText}>
            {
              "If you didn't request this change, you can safely ignore this email — your account stays as-is."
            }
          </Text>
        </Section>
      </Container>
    </Body>
  </Html>
);

export default EmailChangeVerifyEmail;

const h1: React.CSSProperties = {
  color: "#0f172a",
  fontSize: "26px",
  fontWeight: "700",
  margin: "0 0 16px",
  lineHeight: "1.2",
};
