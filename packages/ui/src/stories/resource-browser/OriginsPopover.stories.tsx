import type { Meta, StoryObj } from "@storybook/react-vite";
import { OriginsPopover } from "../../components/resource-browser/OriginsPopover";
import type { OriginDTO, ImportSessionDTO } from "../../components/resource-browser/types";

const now = new Date().toISOString();
const yesterday = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString();
const lastWeek = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString();

const singleFileOrigin: OriginDTO[] = [
  {
    source: "file",
    key: "apps/web/locales/en-US.json:errors.notFound",
    reference: "commit:abc123ef",
    added_at: yesterday,
    added_by: "tmx-import",
  },
];

const multipleOrigins: OriginDTO[] = [
  {
    source: "file",
    key: "apps/web/locales/en-US.json:errors.notFound",
    reference: "commit:abc123ef",
    added_at: lastWeek,
    added_by: "tmx-import",
  },
  {
    source: "file",
    key: "apps/mobile/strings.xml:error_not_found",
    added_at: yesterday,
    added_by: "tmx-import",
  },
  {
    source: "tool",
    key: "ai-translate",
    reference: "job-42",
    added_at: now,
    added_by: "kapi",
  },
  {
    source: "user",
    added_at: now,
    added_by: "alice@example.com",
  },
];

const meta: Meta<typeof OriginsPopover> = {
  title: "Resource Browser/OriginsPopover",
  component: OriginsPopover,
  tags: ["autodocs"],
  decorators: [
    (Story: React.ComponentType) => (
      <div style={{ padding: 40 }}>
        <Story />
      </div>
    ),
  ],
  parameters: {
    docs: {
      description: {
        component:
          "Provenance popover for TM entries. Shows a count badge of origins (file, tool, import, user) and an optional translator note. Click to expand.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof OriginsPopover>;

export const SingleOrigin: Story = {
  args: {
    origins: singleFileOrigin,
  },
};

export const MultipleOrigins: Story = {
  args: {
    origins: multipleOrigins,
  },
};

export const WithNote: Story = {
  args: {
    origins: singleFileOrigin,
    note: "This is shown in the welcome screen — keep the tone friendly and concise.",
  },
};

export const NoteOnly: Story = {
  args: {
    origins: [],
    note: "Translator note without any recorded origin.",
  },
};

export const Empty: Story = {
  args: {
    origins: [],
  },
  parameters: {
    docs: {
      description: {
        story: "When there are no origins and no note, the component renders nothing.",
      },
    },
  },
};

const SAMPLE_SESSION: ImportSessionDTO = {
  id: "sess-1",
  file_key: "acme-glossary.tmx",
  file_hash: "sha256:abc123",
  file_size_bytes: 120_480,
  imported_at: lastWeek,
  imported_by: "tmx-import",
  tool_name: "tmx-import",
  tool_version: "1.4.0",
  seg_type: "sentence",
  admin_lang: "",
  src_lang: "en-US",
  data_type: "plaintext",
  original_format: "TMX 1.4",
  original_encoding: "UTF-8",
  entry_count: 542,
};

const sessionOrigin: OriginDTO[] = [
  {
    source: "import",
    key: "acme-glossary.tmx",
    session_id: "sess-1",
    added_at: lastWeek,
    added_by: "tmx-import",
  },
];

/**
 * Origin carrying a session_id. On open the popover fetches the session
 * metadata via `getImportSession` and displays the tool, version, count
 * and import time.
 */
export const WithSessionInfo: Story = {
  args: {
    origins: sessionOrigin,
    getImportSession: async (id: string) => (id === "sess-1" ? SAMPLE_SESSION : null),
  },
};
