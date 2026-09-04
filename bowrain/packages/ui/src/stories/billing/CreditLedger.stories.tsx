import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { CreditLedger } from "../../components/billing/CreditLedger";
import type { CreditLedgerEntry } from "../../types/api";

const meta: Meta<typeof CreditLedger> = {
  title: "Billing/CreditLedger",
  component: CreditLedger,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof CreditLedger>;

const now = Date.now();
const hour = 60 * 60 * 1000;

const sampleEntries: CreditLedgerEntry[] = [
  {
    id: "1",
    amount: 500_000,
    balanceAfter: 500_000,
    operation: "plan_reset",
    createdAt: new Date(now - 48 * hour).toISOString(),
  },
  {
    id: "2",
    amount: -12_400,
    balanceAfter: 487_600,
    operation: "ai_translation",
    referenceId: "job-abc12345",
    createdAt: new Date(now - 36 * hour).toISOString(),
  },
  {
    id: "3",
    amount: -3_200,
    balanceAfter: 484_400,
    operation: "bravo_message",
    referenceId: "conv-def67890",
    createdAt: new Date(now - 24 * hour).toISOString(),
  },
  {
    id: "4",
    amount: -800,
    balanceAfter: 483_600,
    operation: "bravo_container",
    referenceId: "conv-def67890",
    createdAt: new Date(now - 23 * hour).toISOString(),
  },
  {
    id: "5",
    amount: -5_100,
    balanceAfter: 478_500,
    operation: "ai_quality_check",
    referenceId: "chk-ghi11223",
    createdAt: new Date(now - 12 * hour).toISOString(),
  },
  {
    id: "6",
    amount: 200_000,
    balanceAfter: 678_500,
    operation: "purchase",
    referenceId: "pi-jkl44556",
    createdAt: new Date(now - 6 * hour).toISOString(),
  },
  {
    id: "7",
    amount: 50_000,
    balanceAfter: 728_500,
    operation: "grant",
    createdAt: new Date(now - 2 * hour).toISOString(),
  },
  {
    id: "8",
    amount: -18_300,
    balanceAfter: 710_200,
    operation: "ai_translation",
    referenceId: "job-mno77889",
    createdAt: new Date(now - 1 * hour).toISOString(),
  },
];

/** The operations the server reports for the window, whatever the page holds. */
const windowOperations = [
  "plan_reset",
  "ai_translation",
  "bravo_message",
  "bravo_container",
  "ai_quality_check",
  "purchase",
  "grant",
];

export const Default: Story = {
  args: {
    entries: sampleEntries,
    operations: windowOperations,
    operation: "all",
    onOperationChange: fn(),
    total: sampleEntries.length,
  },
};

export const Empty: Story = {
  args: {
    entries: [],
    operations: [],
    total: 0,
  },
};

/** One operation chosen: the server returns its rows, and the window's chips stay. */
export const FilteredToOneOperation: Story = {
  args: {
    entries: sampleEntries.filter((e) => e.operation === "ai_translation"),
    operations: windowOperations,
    operation: "ai_translation",
    onOperationChange: fn(),
    total: 2,
  },
};

/** A window larger than one page — the pager counts the server's total. */
export const Paged: Story = {
  args: {
    entries: sampleEntries.slice(0, 4),
    operations: windowOperations,
    operation: "all",
    onOperationChange: fn(),
    total: 128,
    limit: 4,
    offset: 4,
    onOffsetChange: fn(),
  },
};
