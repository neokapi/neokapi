import type { Meta, StoryObj } from "@storybook/react-vite";
import KeyedTable from "../../components/preview/KeyedTable";
import FormatPreview from "../../components/preview/FormatPreview";
import type { ContentTree, Run } from "../../components/preview/types";

// The structured preview reads a block's inline codes two ways: a presentational
// code (bold, italic, monospace) renders as its real element, and an opaque code
// (a placeholder, a link, a break) renders as a chip. These stories show each
// case, and the overlays that layer over the now-styled text.

const bold = (text: string): Run[] => [
  { pcOpen: { id: "b", type: "fmt:bold", data: "<b>" } },
  { text },
  { pcClose: { id: "b", type: "fmt:bold", data: "</b>" } },
];

const code = (text: string): Run[] => [
  { pcOpen: { id: "c", type: "fmt:code", data: "<code>" } },
  { text },
  { pcClose: { id: "c", type: "fmt:code", data: "</code>" } },
];

function block(name: string, source: Run[]) {
  return { kind: "block", id: `b:${name}`, name, translatable: true, source } as const;
}

function tree(nodes: ReturnType<typeof block>[], format = "json"): ContentTree {
  return {
    format,
    stats: { layers: 0, groups: 0, blocks: nodes.length, data: 0, media: 0, runs: 0 },
    root: nodes,
  } as unknown as ContentTree;
}

const meta: Meta = {
  title: "Preview/Inline codes",
  parameters: {
    docs: {
      description: {
        component:
          "How the structured preview reads a block's inline codes: presentational codes render as their real element, opaque codes as chips, and overlays layer over the styled text.",
      },
    },
  },
};

export default meta;
type Story = StoryObj;

/** A bold pair reads as bold, not as [B]…/B chips around plain text. */
export const BoldPair: Story = {
  render: () => (
    <div className="max-w-xl p-4">
      <KeyedTable
        tree={tree([
          block("promo.headline", [{ text: "Big " }, ...bold("Sale"), { text: " now" }]),
        ])}
      />
    </div>
  ),
};

/** A code pair reads as monospace, the way the faithful document render shows it. */
export const CodePair: Story = {
  render: () => (
    <div className="max-w-xl p-4">
      <KeyedTable
        tree={tree([
          block("intro", [
            { text: "Fired when an " },
            ...code("order.created"),
            { text: " event occurs." },
          ]),
        ])}
      />
    </div>
  ),
};

/** A placeholder stands for a value with no rendered form, so it stays a chip. */
export const Placeholder: Story = {
  render: () => (
    <div className="max-w-xl p-4">
      <KeyedTable
        tree={tree([
          block("greeting", [
            { text: "Welcome back, " },
            { ph: { id: "1", type: "code:variable", data: "{{user}}", equiv: "{{user}}" } },
            { text: "." },
          ]),
        ])}
      />
    </div>
  ),
};

/** A line break ends the line, with a chip so the code stays visible. */
export const Break: Story = {
  render: () => (
    <div className="max-w-xl p-4">
      <KeyedTable
        tree={tree([
          block("address", [
            { text: "First line" },
            { ph: { id: "1", type: "struct:break", data: "<br/>" } },
            { text: "Second line" },
          ]),
        ])}
      />
    </div>
  ),
};

/** An overlay (a term highlight) layers over text inside a bold span. */
export const OverlayOverStyled: Story = {
  render: () => {
    const runs: Run[] = [
      { pcOpen: { id: "1", type: "fmt:bold", data: "<b>" } },
      { text: "Big Sale now" },
      { pcClose: { id: "1", type: "fmt:bold", data: "</b>" } },
    ];
    const t = {
      format: "json",
      stats: { layers: 0, groups: 0, blocks: 1, data: 0, media: 0, runs: 3 },
      root: [
        {
          kind: "block",
          id: "b1",
          name: "promo.headline",
          translatable: true,
          source: runs,
          overlays: [
            {
              type: "term",
              side: "source",
              spans: [
                {
                  id: "t1",
                  range: {
                    kind: "range",
                    start: { run: 1, offset: 4 },
                    end: { run: 1, offset: 8 },
                  },
                  text: "Sale",
                },
              ],
            },
          ],
        },
      ],
    } as unknown as ContentTree;
    return (
      <div className="max-w-xl p-4">
        <KeyedTable tree={t} />
      </div>
    );
  },
};

/** The API-reference shape: an event catalog whose ids read as monospace code,
 *  not as [CODE]…/code chips. */
export const ApiReferenceTable: Story = {
  render: () => (
    <div className="max-w-2xl p-4">
      <KeyedTable
        tree={tree([
          block("events.order_created", [
            { text: "Sent when an " },
            ...code("order.created"),
            { text: " event fires." },
          ]),
          block("events.order_refunded", [
            { text: "Sent when an " },
            ...code("order.refunded"),
            { text: " event fires." },
          ]),
          block("events.subscription_renewed", [
            { text: "Sent when a " },
            ...code("subscription.renewed"),
            { text: " event fires." },
          ]),
        ])}
      />
    </div>
  ),
};

/** A prose block read as a document, with bold and monospace in flow. */
export const InDocument: Story = {
  render: () => (
    <div className="max-w-xl p-4">
      <FormatPreview
        tree={tree(
          [
            block("p1", [
              { text: "Call " },
              ...code("kapi up"),
              { text: " to converge, then review the " },
              ...bold("pending"),
              { text: " changes." },
            ]),
          ],
          "markdown",
        )}
        reducedMotion
      />
    </div>
  ),
};
