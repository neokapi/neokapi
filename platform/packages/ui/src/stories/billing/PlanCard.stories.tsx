import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { PlanCard } from "../../components/billing/PlanCard";

const meta: Meta<typeof PlanCard> = {
  title: "Billing/PlanCard",
  component: PlanCard,
  tags: ["autodocs"],
  args: { onSelect: fn() },
};

export default meta;
type Story = StoryObj<typeof PlanCard>;

export const Free: Story = {
  args: {
    plan: "free",
    name: "Free",
    price: "$0",
    description: "Get started with AI-powered localization",
    credits: "50K credits / week",
    features: [
      { label: "@bravo chat (5 messages/day)", included: true },
      { label: "1 project", included: true },
      { label: "Git connectors", included: false },
      { label: "API access", included: false },
      { label: "Custom MT providers", included: false },
    ],
  },
};

export const ProRecommended: Story = {
  args: {
    plan: "pro",
    name: "Pro",
    price: "$25",
    period: "mo",
    description: "For professionals and small teams",
    credits: "500K credits / week",
    recommended: true,
    features: [
      { label: "@bravo unlimited messages", included: true },
      { label: "Up to 10 projects", included: true },
      { label: "3 seats", included: true },
      { label: "Git connectors", included: true },
      { label: "API access", included: true },
      { label: "Custom MT providers", included: true },
      { label: "@bravo code execution", included: false },
    ],
  },
};

export const Team: Story = {
  args: {
    plan: "team",
    name: "Team",
    price: "$20",
    period: "seat/mo",
    description: "For growing teams",
    credits: "2M credits / week",
    features: [
      { label: "Everything in Pro", included: true },
      { label: "Unlimited projects", included: true },
      { label: "Unlimited seats", included: true },
      { label: "@bravo code execution", included: true },
      { label: "Custom connectors", included: true },
      { label: "SSO/SAML", included: false },
    ],
  },
};

export const Enterprise: Story = {
  args: {
    plan: "enterprise",
    name: "Enterprise",
    price: "Custom",
    description: "For large organizations",
    credits: "Custom credit allocation",
    ctaLabel: "Contact Sales",
    features: [
      { label: "Everything in Team", included: true },
      { label: "SSO/SAML", included: true },
      { label: "Dedicated support", included: true },
      { label: "Custom agreements", included: true },
      { label: "SLA guarantees", included: true },
    ],
  },
};

export const CurrentPlan: Story = {
  args: {
    plan: "pro",
    name: "Pro",
    price: "$25",
    period: "mo",
    credits: "500K credits / week",
    current: true,
    features: [
      { label: "@bravo unlimited messages", included: true },
      { label: "Up to 10 projects", included: true },
      { label: "3 seats", included: true },
    ],
  },
};

export const AllPlans: Story = {
  render: () => (
    <div
      style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 16, maxWidth: 1200 }}
    >
      <PlanCard
        plan="free"
        name="Free"
        price="$0"
        credits="50K credits / week"
        features={[
          { label: "@bravo chat", included: true },
          { label: "1 project", included: true },
          { label: "Git connectors", included: false },
        ]}
      />
      <PlanCard
        plan="pro"
        name="Pro"
        price="$25"
        period="mo"
        credits="500K credits / week"
        recommended
        features={[
          { label: "Unlimited @bravo", included: true },
          { label: "10 projects", included: true },
          { label: "Git connectors", included: true },
          { label: "API access", included: true },
        ]}
      />
      <PlanCard
        plan="team"
        name="Team"
        price="$20"
        period="seat/mo"
        credits="2M credits / week"
        features={[
          { label: "Everything in Pro", included: true },
          { label: "Unlimited seats", included: true },
          { label: "Code execution", included: true },
        ]}
      />
      <PlanCard
        plan="enterprise"
        name="Enterprise"
        price="Custom"
        credits="Custom"
        ctaLabel="Contact Sales"
        features={[
          { label: "Everything in Team", included: true },
          { label: "SSO/SAML", included: true },
          { label: "Dedicated support", included: true },
        ]}
      />
    </div>
  ),
};
