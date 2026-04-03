import type { Meta, StoryObj } from "@storybook/react-vite";
import { PaymentFailedEmail } from "./payment-failed";
import { EmailPreview } from "./storybook-decorator";

const meta: Meta<typeof PaymentFailedEmail> = {
  title: "Emails/Payment Failed",
  component: PaymentFailedEmail,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
  decorators: [
    (_, { args }) => (
      <EmailPreview>
        <PaymentFailedEmail {...args} />
      </EmailPreview>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof PaymentFailedEmail>;

export const MonthlySubscription: Story = {
  args: {
    workspaceName: "Acme Translations",
    invoiceAmount: "$25.00",
    currency: "USD",
    updatePaymentURL: "https://app.bowrain.com/acme/settings/billing",
  },
};

export const TeamPlan: Story = {
  args: {
    workspaceName: "Globex Engineering",
    invoiceAmount: "$100.00",
    currency: "USD",
    updatePaymentURL: "https://app.bowrain.com/globex/settings/billing",
  },
};

export const EuroCurrency: Story = {
  args: {
    workspaceName: "Berlin Localization GmbH",
    invoiceAmount: "€45.00",
    currency: "EUR",
    updatePaymentURL: "https://app.bowrain.com/berlin-loc/settings/billing",
  },
};
