import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@neokapi/ui-primitives";
import type { Meta, StoryObj } from "@storybook/react-vite";

const meta: Meta<typeof Table> = {
  title: "Foundations/Table",
  component: Table,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 700, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Table>;

const files = [
  { file: "messages.json", locale: "fr-FR", words: 1243, status: "Translated" },
  { file: "messages.json", locale: "de-DE", words: 1243, status: "In Review" },
  { file: "messages.json", locale: "ja-JP", words: 1243, status: "Draft" },
  { file: "errors.json", locale: "fr-FR", words: 312, status: "Translated" },
  { file: "errors.json", locale: "de-DE", words: 312, status: "Not Started" },
  { file: "ui-labels.xliff", locale: "es-ES", words: 876, status: "In Review" },
];

export const Default: Story = {
  render: () => (
    <Table>
      <TableCaption>Translation file status overview</TableCaption>
      <TableHeader>
        <TableRow>
          <TableHead>File</TableHead>
          <TableHead>Locale</TableHead>
          <TableHead className="text-right">Words</TableHead>
          <TableHead>Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {files.map((row, i) => (
          <TableRow key={i}>
            <TableCell className="font-medium">{row.file}</TableCell>
            <TableCell>{row.locale}</TableCell>
            <TableCell className="text-right">{row.words.toLocaleString()}</TableCell>
            <TableCell>{row.status}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  ),
};
