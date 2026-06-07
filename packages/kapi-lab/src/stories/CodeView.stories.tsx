import type { Meta, StoryObj } from "@storybook/react-vite";
import { CodeView } from "@neokapi/ui-primitives/preview";

const meta: Meta<typeof CodeView> = {
  title: "Lab/Code View",
  component: CodeView,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof CodeView>;

const json = `{
  "greeting": "Hello, {name}!",
  "count": 42,
  "enabled": true,
  "cart": { "empty": "Your cart is empty" }
}
`;

const xliff = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2">
  <file source-language="en" target-language="fr">
    <!-- one unit -->
    <trans-unit id="greeting">
      <source>Hello, World!</source>
      <target>Bonjour le monde !</target>
    </trans-unit>
  </file>
</xliff>
`;

const properties = `# Application strings
app.title = Welcome aboard
app.greeting = Hello, World!
cart.empty = Your cart is empty
`;

export const Json: Story = { args: { text: json, filename: "messages.json" } };
export const Xliff: Story = { args: { text: xliff, filename: "app.xliff" } };
export const Properties: Story = { args: { text: properties, filename: "app.properties" } };

export const ChangedLines: Story = {
  name: "With changed lines",
  args: { text: json, filename: "messages.json", changedLines: new Set([1, 4]) },
};
