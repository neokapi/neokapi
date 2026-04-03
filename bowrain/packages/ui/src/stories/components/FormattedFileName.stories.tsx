import type { Meta, StoryObj } from "@storybook/react-vite";
import { FormattedFileName } from "../../components/FormattedFileName";

const meta: Meta<typeof FormattedFileName> = {
  title: "Components/FormattedFileName",
  component: FormattedFileName,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 400, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FormattedFileName>;

export const Json: Story = {
  args: { name: "messages.json", format: "json" },
};

export const Xliff: Story = {
  args: { name: "ui-strings.xliff", format: "xliff" },
};

export const Html: Story = {
  args: { name: "landing-page.html", format: "html" },
};

export const Yaml: Story = {
  args: { name: "config.yaml", format: "yaml" },
};

export const Csv: Story = {
  args: { name: "translations.csv", format: "csv" },
};

export const Markdown: Story = {
  args: { name: "api-reference.md", format: "markdown" },
};

export const Properties: Story = {
  args: { name: "strings.properties", format: "properties" },
};

export const Xml: Story = {
  args: { name: "resources.xml", format: "xml" },
};

export const PlainText: Story = {
  args: { name: "readme.txt", format: "plaintext" },
};

export const Po: Story = {
  args: { name: "locale.po", format: "po" },
};

export const NoExtension: Story = {
  args: { name: "Makefile", format: "plaintext" },
};

export const NoFormat: Story = {
  args: { name: "document.docx" },
};

/** All supported formats side by side */
export const AllFormats: Story = {
  render: () => (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      <FormattedFileName name="messages.json" format="json" />
      <FormattedFileName name="ui-strings.xliff" format="xliff" />
      <FormattedFileName name="landing-page.html" format="html" />
      <FormattedFileName name="config.yaml" format="yaml" />
      <FormattedFileName name="translations.csv" format="csv" />
      <FormattedFileName name="api-reference.md" format="markdown" />
      <FormattedFileName name="strings.properties" format="properties" />
      <FormattedFileName name="resources.xml" format="xml" />
      <FormattedFileName name="readme.txt" format="plaintext" />
      <FormattedFileName name="locale.po" format="po" />
      <FormattedFileName name="document.xliff" format="xliff2" />
      <FormattedFileName name="Makefile" format="plaintext" />
    </div>
  ),
};
