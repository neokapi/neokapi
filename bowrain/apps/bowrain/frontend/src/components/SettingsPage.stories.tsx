import type { Meta, StoryObj } from "@storybook/react-vite";
import { SettingsPage } from "./SettingsPage";
import { withBackend } from "../storybook/backend";
import { qk } from "../lib/queryKeys";
import type {
  FormatInfo,
  ToolInfo,
  FlowInfo,
  PluginInfo,
  VersionInfo,
  ProviderConfig,
} from "../types/api";

/**
 * SettingsPage is a Wails-`Backend` container: its tabs read the static
 * registries (formats/tools/flows/plugins/version) and the AI-provider configs
 * through react-query, where each `queryFn` is a generated binding. The stories
 * pre-seed those query keys via `withBackend`, so every tab renders its loaded
 * state without a live Wails runtime. The General tab (theme picker) is shown by
 * default and needs no data.
 */

const version: VersionInfo = { version: "1.2.0", commit: "a1b2c3d", build_date: "2026-07-08" };

const formats: FormatInfo[] = [
  { name: "json", has_reader: true, has_writer: true },
  { name: "xliff", has_reader: true, has_writer: true },
  { name: "docx", has_reader: true, has_writer: false },
];

const tools: ToolInfo[] = [
  {
    name: "translate",
    description: "Translate content using an AI/LLM provider.",
    category: "translation",
  },
  { name: "qa", description: "Quality-check translations using AI.", category: "quality" },
  {
    name: "pseudo-translate",
    description: "Generate pseudo-translations for layout testing.",
    category: "text-processing",
  },
];

const flows: FlowInfo[] = [
  { name: "translate-qa", description: "Translate then run a QA check." },
  { name: "pseudo", description: "Pseudo-translate for layout testing." },
];

const plugins: PluginInfo[] = [
  {
    name: "okapi-bridge",
    type: "format",
    source: "okapi-bridge",
    formats: ["idml", "po", "properties"],
  },
];

const providers: ProviderConfig[] = [
  {
    id: "prov-1",
    name: "House Anthropic",
    provider_type: "anthropic",
    model: "claude-sonnet-4-20250514",
    base_url: "",
  },
  {
    id: "prov-2",
    name: "Local Ollama",
    provider_type: "ollama",
    model: "llama3",
    base_url: "http://localhost:11434",
  },
];

const loadedSeed = [
  { queryKey: qk.version(), data: version },
  { queryKey: qk.formats(), data: formats },
  { queryKey: qk.tools(), data: tools },
  { queryKey: qk.flows(""), data: flows },
  { queryKey: qk.plugins(), data: { plugins, pluginDir: "~/.config/kapi/plugins" } },
  { queryKey: qk.providerConfigs(), data: providers },
];

const emptySeed = [
  { queryKey: qk.version(), data: version },
  { queryKey: qk.formats(), data: [] },
  { queryKey: qk.tools(), data: [] },
  { queryKey: qk.flows(""), data: [] },
  { queryKey: qk.plugins(), data: { plugins: [], pluginDir: "" } },
  { queryKey: qk.providerConfigs(), data: [] },
];

const meta: Meta<typeof SettingsPage> = {
  title: "Bowrain/SettingsPage",
  component: SettingsPage,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <div className="h-[640px] w-full max-w-4xl p-6">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof SettingsPage>;

/** Providers, plugins, and registries all populated. */
export const Loaded: Story = {
  decorators: [withBackend(loadedSeed)],
};

/** No AI providers or plugins configured — the empty-state cards show. */
export const Empty: Story = {
  decorators: [withBackend(emptySeed)],
};
