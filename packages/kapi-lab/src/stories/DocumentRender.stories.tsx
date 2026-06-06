import type { Meta, StoryObj } from "@storybook/react-vite";
import DocumentRender from "../DocumentRender";
import type { RenderDoc } from "../renderDoc";

// DocumentRender paints the normalized extraction model (treeToRenderDoc) as a
// recognizable document. These stories show all three structured kinds plus the
// before/after change highlight that the hero carousel and modal rely on.

const meta: Meta<typeof DocumentRender> = {
  title: "Lab/Document Render",
  component: DocumentRender,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof DocumentRender>;

const slideEN: RenderDoc = {
  kind: "slides",
  format: "openxml",
  slides: [
    {
      name: "ppt/slides/slide1.xml",
      title: { id: "p1", text: "Welcome to Acme", role: "title" },
      bullets: [
        { id: "p2", text: "Acme makes every quarter count.", role: "bullet" },
        { id: "p3", text: "Sign up for Acme today", role: "bullet" },
        { id: "p4", text: "Talk to the Acme team soon", role: "bullet" },
      ],
    },
  ],
};

const slideFR: RenderDoc = {
  kind: "slides",
  format: "openxml",
  slides: [
    {
      name: "ppt/slides/slide1.xml",
      title: { id: "p1", text: "Bienvenue chez Acme", role: "title" },
      bullets: [
        { id: "p2", text: "Acme fait compter chaque trimestre.", role: "bullet" },
        { id: "p3", text: "Inscrivez-vous chez Acme dès aujourd'hui", role: "bullet" },
        { id: "p4", text: "Parlez vite à l'équipe Acme", role: "bullet" },
      ],
    },
  ],
};

const sheetEN: RenderDoc = {
  kind: "sheet",
  format: "openxml",
  sheet: {
    name: "xl/worksheets/sheet1.xml",
    cols: 2,
    rows: 3,
    cells: [
      { id: "x1", col: 0, row: 0, ref: "A1", text: "Acme quarterly revenue" },
      { id: "x2", col: 1, row: 0, ref: "B1", text: "Total revenue" },
      { id: "x3", col: 0, row: 1, ref: "A2", text: "Acme net profit" },
      { id: "x4", col: 1, row: 1, ref: "B2", text: "Net profit" },
      { id: "x5", col: 0, row: 2, ref: "A3", text: "Acme customer count" },
      { id: "x6", col: 1, row: 2, ref: "B3", text: "Active accounts" },
    ],
  },
};

const sheetFR: RenderDoc = {
  kind: "sheet",
  format: "openxml",
  sheet: {
    name: "xl/worksheets/sheet1.xml",
    cols: 2,
    rows: 3,
    cells: [
      { id: "x1", col: 0, row: 0, ref: "A1", text: "Chiffre d'affaires trimestriel Acme" },
      { id: "x2", col: 1, row: 0, ref: "B1", text: "Chiffre d'affaires total" },
      { id: "x3", col: 0, row: 1, ref: "A2", text: "Bénéfice net Acme" },
      { id: "x4", col: 1, row: 1, ref: "B2", text: "Bénéfice net" },
      { id: "x5", col: 0, row: 2, ref: "A3", text: "Nombre de clients Acme" },
      { id: "x6", col: 1, row: 2, ref: "B3", text: "Comptes actifs" },
    ],
  },
};

const docEN: RenderDoc = {
  kind: "doc",
  format: "markdown",
  paragraphs: [
    { id: "m1", text: "Welcome to Acme", role: "heading" },
    { id: "m2", text: "Acme helps teams ship faster.", role: "body" },
    { id: "m3", text: "Sign up for Acme today", role: "bullet" },
  ],
};

const docFR: RenderDoc = {
  kind: "doc",
  format: "markdown",
  paragraphs: [
    { id: "m1", text: "Bienvenue chez Acme", role: "heading" },
    { id: "m2", text: "Acme aide les équipes à livrer plus vite.", role: "body" },
    { id: "m3", text: "Inscrivez-vous chez Acme dès aujourd'hui", role: "bullet" },
  ],
};

export const Slide: Story = {
  render: () => <DocumentRender doc={slideEN} className="max-w-md" />,
};

export const Sheet: Story = {
  render: () => <DocumentRender doc={sheetEN} className="max-w-lg" />,
};

export const DocPage: Story = {
  name: "Document page",
  render: () => <DocumentRender doc={docEN} className="max-w-lg" />,
};

export const BeforeAfter: Story = {
  name: "Before / after (EN → FR)",
  render: () => (
    <div className="flex flex-wrap gap-6">
      <div className="max-w-md flex-1">
        <p className="mb-2 text-sm font-semibold text-muted-foreground">Source (EN)</p>
        <DocumentRender doc={slideEN} />
      </div>
      <div className="max-w-md flex-1">
        <p className="mb-2 text-sm font-semibold text-muted-foreground">Result (FR)</p>
        <DocumentRender doc={slideFR} before={slideEN} />
      </div>
    </div>
  ),
};

export const SheetBeforeAfter: Story = {
  name: "Sheet before / after",
  render: () => (
    <div className="flex flex-wrap gap-6">
      <DocumentRender doc={sheetEN} className="max-w-md flex-1" />
      <DocumentRender doc={sheetFR} before={sheetEN} className="max-w-md flex-1" />
    </div>
  ),
};

export const DocBeforeAfter: Story = {
  name: "Doc before / after",
  render: () => (
    <div className="flex flex-wrap gap-6">
      <DocumentRender doc={docEN} className="max-w-sm flex-1" />
      <DocumentRender doc={docFR} before={docEN} className="max-w-sm flex-1" />
    </div>
  ),
};
