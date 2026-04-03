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

interface PaymentFailedEmailProps {
  workspaceName: string;
  invoiceAmount: string;
  currency: string;
  updatePaymentURL: string;
}

/**
 * Branded payment-failed email for Bowrain.
 *
 * Props are populated at build time with Go text/template tokens
 * (e.g. workspaceName = "{{.WorkspaceName}}") so the rendered HTML
 * doubles as a Go template. The mailer package fills in real values at
 * send time using text/template.Execute().
 */
export const PaymentFailedEmail = ({
  workspaceName,
  invoiceAmount,
  currency,
  updatePaymentURL,
}: PaymentFailedEmailProps) => (
  <Html lang="en" dir="ltr">
    <Head />
    <Preview>
      {"Payment failed for "}
      {workspaceName}
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
            {"Payment failed for your subscription"}
          </Heading>

          <Text style={paragraph}>
            {"We were unable to process the payment of "}
            <strong>
              {invoiceAmount} {currency}
            </strong>
            {" for the workspace "}
            <strong>{workspaceName}</strong>.
          </Text>

          <Text style={paragraph}>
            {"Your subscription is still active, but you have a "}
            <strong>{"7-day grace period"}</strong>
            {
              " to update your payment method. If the payment is not resolved within this period, your subscription will be downgraded to the free plan."
            }
          </Text>

          <Section style={btnWrapper}>
            <Button href={updatePaymentURL} style={btn}>
              Update Payment Method
            </Button>
          </Section>

          <Hr style={hr} />

          <Text style={fallback}>
            {"Button not working? Copy and paste this link into your browser:"}
          </Text>
          <Link href={updatePaymentURL} style={link}>
            {updatePaymentURL}
          </Link>
        </Section>

        {/* ── Footer ─────────────────────────────────── */}
        <Section style={footer}>
          <Text style={footerText}>{"© Bowrain. All rights reserved."}</Text>
          <Text style={footerText}>
            {"You received this email because you are an admin of this workspace."}
          </Text>
        </Section>
      </Container>
    </Body>
  </Html>
);

export default PaymentFailedEmail;

// ── Local styles (payment-failed-specific) ───────────────────────────────────

const h1: React.CSSProperties = {
  color: "#0f172a",
  fontSize: "26px",
  fontWeight: "700",
  margin: "0 0 16px",
  lineHeight: "1.2",
};
