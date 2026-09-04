import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ApiProvider } from "@neokapi/ui";
import { ProjectFlowsEditor } from "./ProjectFlowsEditor";
import { ProjectFlowPane } from "./ProjectFlowPane";
import { definitionToSpec } from "./flowGraph";
import {
  builtInTranslate,
  createFlowsApi,
  projectReview,
  sampleTools,
  type FlowsApiOptions,
} from "./fixtures";

// The project's Flows tab over an in-memory flow API: the list of built-in and
// project flows, and one flow open in the shared linear editor.

function FlowsOver(options: FlowsApiOptions) {
  const { api } = createFlowsApi(options);
  return (
    <ApiProvider adapter={api}>
      <div className="mx-auto w-full max-w-7xl p-6">
        <ProjectFlowsEditor workspaceSlug="acme" projectId="p-1" />
      </div>
    </ApiProvider>
  );
}

const meta: Meta<typeof FlowsOver> = {
  title: "Pages/ProjectFlows",
  component: FlowsOver,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "A project's flows: built-in flows merged with the project's own, each an outcome-first card, and one flow open in the shared linear step editor. Edits save on their own after a short pause.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof FlowsOver>;

/** The list: two built-in flows and a project flow with a parallel group. */
export const List: Story = { args: {} };

export const ListDark: Story = { args: {}, globals: { theme: "dark" } };

/** A project with no flows at all. */
export const Empty: Story = { args: { flows: [] } };

/** The list while the definitions load. */
export const Loading: Story = { args: { delay: 60 * 60 * 1000 } };

const paneArgs = {
  tools: sampleTools,
  onBack: fn(),
  onChange: fn(),
  onRename: fn(),
  onCopy: fn(),
  onDelete: fn(),
};

function Pane(props: React.ComponentProps<typeof ProjectFlowPane>) {
  return (
    <div className="mx-auto w-full max-w-7xl p-6">
      <ProjectFlowPane {...props} />
    </div>
  );
}

/** A project flow open for editing, with a parallel group and an edit saved. */
export const Editor: StoryObj<typeof Pane> = {
  render: (args) => <Pane {...args} />,
  args: {
    ...paneArgs,
    flow: projectReview,
    spec: definitionToSpec(projectReview),
    readOnly: false,
    saveState: "saved",
  },
};

export const EditorDark: StoryObj<typeof Pane> = {
  ...Editor,
  globals: { theme: "dark" },
};

/** The same flow with an edit resting before its save. */
export const EditorUnsaved: StoryObj<typeof Pane> = {
  ...Editor,
  args: { ...Editor.args, saveState: "unsaved" },
};

/** A save the server refused. */
export const EditorSaveFailed: StoryObj<typeof Pane> = {
  ...Editor,
  args: {
    ...Editor.args,
    saveState: "error",
    saveError: new Error("403 Forbidden: manage_automation is required to edit flows"),
  },
};

/** A built-in flow: read-only, with a copy offered instead of edits. */
export const EditorReadOnly: StoryObj<typeof Pane> = {
  ...Editor,
  args: {
    ...paneArgs,
    flow: builtInTranslate,
    spec: definitionToSpec(builtInTranslate),
    readOnly: true,
    saveState: "saved",
  },
};

/** A new flow with no steps yet: the template library and the add controls. */
export const EditorEmpty: StoryObj<typeof Pane> = {
  ...Editor,
  args: {
    ...paneArgs,
    flow: {
      id: "flow-new",
      name: "Publish check",
      description: "",
      source: "project",
      nodes: [],
      edges: [],
    },
    spec: { steps: [] },
    readOnly: false,
    saveState: "saved",
  },
};
