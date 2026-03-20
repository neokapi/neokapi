import type { Meta, StoryObj } from "@storybook/react-vite";
import { PlanComparisonTable } from "../../components/billing/PlanComparisonTable";
import type { ComparisonFeature } from "../../components/billing/PlanComparisonTable";

const meta: Meta<typeof PlanComparisonTable> = {
  title: "Billing/PlanComparisonTable",
  component: PlanComparisonTable,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof PlanComparisonTable>;

const features: ComparisonFeature[] = [
  {
    label: "Weekly AI Credits",
    values: { free: "50K", pro: "500K", team: "2M", enterprise: "Custom" },
  },
  {
    label: "@bravo Messages",
    values: { free: "5/day", pro: "Unlimited", team: "Unlimited", enterprise: "Unlimited" },
  },
  {
    label: "@bravo Code Execution",
    values: { free: false, pro: false, team: true, enterprise: true },
  },
  {
    label: "Projects",
    values: { free: "1", pro: "10", team: "Unlimited", enterprise: "Unlimited" },
  },
  {
    label: "Seats",
    values: { free: "1", pro: "3", team: "Unlimited", enterprise: "Unlimited" },
  },
  {
    label: "Git Connectors",
    values: { free: false, pro: true, team: true, enterprise: true },
  },
  {
    label: "Custom Connectors",
    values: { free: false, pro: false, team: true, enterprise: true },
  },
  {
    label: "API Access",
    values: { free: false, pro: true, team: true, enterprise: true },
  },
  {
    label: "Custom MT Providers",
    values: { free: false, pro: true, team: true, enterprise: true },
  },
  {
    label: "SSO/SAML",
    values: { free: false, pro: false, team: false, enterprise: true },
  },
];

export const Default: Story = {
  args: {
    features,
    recommendedPlan: "pro",
  },
};

export const TeamRecommended: Story = {
  args: {
    features,
    recommendedPlan: "team",
  },
};
