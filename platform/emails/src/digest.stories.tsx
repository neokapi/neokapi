import type { Meta, StoryObj } from "@storybook/react-vite";
import { DigestEmail } from "./digest";
import { EmailPreview } from "./storybook-decorator";

const meta: Meta<typeof DigestEmail> = {
  title: "Emails/Digest",
  component: DigestEmail,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
  decorators: [
    (_, { args }) => (
      <EmailPreview>
        <DigestEmail {...args} />
      </EmailPreview>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof DigestEmail>;

export const DailyDigest: Story = {
  args: {
    frequency: "daily",
    totalUpdates: "7",
    dashboardURL: "https://app.bowrain.com/ws/acme/notifications",
    settingsURL: "https://app.bowrain.com/ws/acme/settings/notifications",
    groups: [
      {
        category: "task",
        label: "Tasks",
        items: [
          {
            title: "New task assigned: Review French translations",
            body: "Alice assigned you to review 24 strings in fr-FR for the Mobile App project.",
          },
          {
            title: "Task completed: Translate homepage",
            body: "Bob completed translating homepage content to de-DE.",
          },
        ],
      },
      {
        category: "quality",
        label: "Quality",
        items: [
          {
            title: "Quality gate failed: Terminology check",
            body: "3 terminology violations found in fr-FR for the Website project.",
            priority: "high",
          },
        ],
      },
      {
        category: "project",
        label: "Project",
        items: [
          {
            title: "Stream merged: feature/onboarding",
            body: "The feature/onboarding stream was merged into main.",
          },
          {
            title: "New content available",
            body: "12 new strings pushed to the Mobile App project for translation.",
          },
        ],
      },
      {
        category: "mention",
        label: "Mentions",
        items: [
          {
            title: "Alice mentioned you",
            body: "\"@charlie can you review the updated glossary terms?\"",
          },
        ],
      },
      {
        category: "automation",
        label: "Automation",
        items: [
          {
            title: "Flow failed: Auto-translate (ja-JP)",
            body: "The auto-translate flow failed for ja-JP in the Website project.",
            priority: "high",
          },
        ],
      },
    ],
  },
};

export const WeeklySummary: Story = {
  args: {
    frequency: "weekly",
    totalUpdates: "23",
    dashboardURL: "https://app.bowrain.com/ws/acme/notifications",
    settingsURL: "https://app.bowrain.com/ws/acme/settings/notifications",
    groups: [
      {
        category: "project",
        label: "Project",
        items: [
          {
            title: "Version v2.1 ready",
            body: "All locales passed quality gates. Version v2.1 is ready for release.",
          },
          {
            title: "Stream merged: feature/payments",
            body: "The feature/payments stream was merged into main.",
          },
          {
            title: "Progress milestone: fr-FR reached 100%",
            body: "French translations are now complete for the Mobile App project.",
          },
          {
            title: "Progress milestone: ja-JP reached 75%",
            body: "Japanese translations reached 75% completion for the Website project.",
          },
          {
            title: "New content available",
            body: "48 new strings pushed across 3 projects this week.",
          },
        ],
      },
      {
        category: "task",
        label: "Tasks",
        items: [
          {
            title: "Deadline approaching: Translate settings page",
            body: "Due in 24 hours. 8 strings remaining in de-DE.",
            priority: "high",
          },
          {
            title: "5 tasks completed this week",
            body: "Translation and review tasks completed across Mobile App and Website.",
          },
          {
            title: "2 new tasks assigned",
            body: "Review Korean translations, Translate checkout flow to pt-BR.",
          },
        ],
      },
      {
        category: "quality",
        label: "Quality",
        items: [
          {
            title: "Quality gate resolved: Terminology check (fr-FR)",
            body: "All terminology violations have been fixed.",
          },
          {
            title: "Quality gate failed: Length check (de-DE)",
            body: "12 strings exceed maximum length in de-DE for the Mobile App project.",
            priority: "high",
          },
        ],
      },
      {
        category: "mention",
        label: "Mentions",
        items: [
          {
            title: "Alice mentioned you (3 times)",
            body: "Latest: \"@charlie the Korean reviewer feedback is in\"",
          },
          {
            title: "Bob mentioned you",
            body: "\"@charlie can you check the German plurals?\"",
          },
        ],
      },
      {
        category: "automation",
        label: "Automation",
        items: [
          {
            title: "6 automation runs completed",
            body: "Auto-translate and quality check flows ran successfully.",
          },
          {
            title: "Flow failed: Quality check (ko-KR)",
            body: "The quality check flow timed out for ko-KR. Manual retry needed.",
            priority: "high",
          },
        ],
      },
      {
        category: "system",
        label: "System",
        items: [
          {
            title: "New team member joined",
            body: "Diana joined the workspace as a Translator.",
          },
        ],
      },
    ],
  },
};

export const MinimalDaily: Story = {
  args: {
    frequency: "daily",
    totalUpdates: "1",
    dashboardURL: "https://app.bowrain.com/ws/startup/notifications",
    settingsURL: "https://app.bowrain.com/ws/startup/settings/notifications",
    groups: [
      {
        category: "task",
        label: "Tasks",
        items: [
          {
            title: "New content available for translation",
            body: "3 new strings added to the Landing Page project.",
          },
        ],
      },
    ],
  },
};

export const HighPriorityOnly: Story = {
  args: {
    frequency: "daily",
    totalUpdates: "3",
    dashboardURL: "https://app.bowrain.com/ws/acme/notifications",
    settingsURL: "https://app.bowrain.com/ws/acme/settings/notifications",
    groups: [
      {
        category: "quality",
        label: "Quality",
        items: [
          {
            title: "Quality gate failed: Spelling check (es-ES)",
            body: "18 spelling errors detected in the latest push.",
            priority: "high",
          },
        ],
      },
      {
        category: "task",
        label: "Tasks",
        items: [
          {
            title: "Deadline approaching: Review mobile strings",
            body: "Due tomorrow. 42 strings awaiting review in ja-JP.",
            priority: "high",
          },
        ],
      },
      {
        category: "automation",
        label: "Automation",
        items: [
          {
            title: "Flow failed: Sync connector (GitHub)",
            body: "GitHub connector sync failed. Authentication token may have expired.",
            priority: "high",
          },
        ],
      },
    ],
  },
};
