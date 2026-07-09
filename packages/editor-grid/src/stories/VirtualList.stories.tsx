import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { VirtualList } from "../VirtualList";

interface Row {
  id: string;
  source: string;
  target: string;
  status: "not-started" | "draft" | "translated" | "reviewed";
}

const STATUS_ACCENT: Record<Row["status"], string> = {
  "not-started": "transparent",
  draft: "#d97706",
  translated: "#2563eb",
  reviewed: "#16a34a",
};

function makeRows(n: number): Row[] {
  const statuses: Row["status"][] = ["not-started", "draft", "translated", "reviewed"];
  return Array.from({ length: n }, (_, i) => ({
    id: `b${i}`,
    source: `Source string number ${i + 1} — the quick brown fox jumps over the lazy dog.`,
    target: i % 3 === 0 ? "" : `Cadena de destino número ${i + 1}.`,
    status: statuses[i % statuses.length],
  }));
}

/** A bilingual source/target grid, the shape bowrain's TableView renders. */
function BilingualGrid({ count }: { count: number }) {
  const [items] = useState(() => makeRows(count));
  const [selected, setSelected] = useState(0);

  return (
    <div style={{ display: "flex", flexDirection: "column", height: 480, fontSize: 13 }}>
      <VirtualList<Row>
        items={items}
        estimateSize={56}
        overscan={12}
        getItemKey={(_, it) => it.id}
        initialRect={{ width: 800, height: 480 }}
        scrollToIndex={selected}
        dataTestId="story-grid"
        className="grid-viewport"
        header={
          <div
            style={{
              display: "flex",
              gap: 12,
              padding: "8px 12px",
              position: "sticky",
              top: 0,
              background: "var(--sb-bg, #fff)",
              borderBottom: "1px solid #e5e7eb",
              fontWeight: 600,
              textTransform: "uppercase",
              letterSpacing: "0.05em",
              fontSize: 11,
              color: "#6b7280",
            }}
          >
            <span style={{ width: 32 }}>#</span>
            <span style={{ flex: 1 }}>Source</span>
            <span style={{ flex: 1 }}>Target</span>
          </div>
        }
        renderRow={(row, { index, rowProps }) => (
          <div
            key={row.id}
            {...rowProps}
            onClick={() => setSelected(index)}
            style={{
              ...rowProps.style,
              display: "flex",
              gap: 12,
              padding: "8px 12px",
              cursor: "pointer",
              borderBottom: "1px solid #f1f5f9",
              borderLeft: `3px solid ${index === selected ? "#6366f1" : STATUS_ACCENT[row.status]}`,
              background: index === selected ? "#eef2ff" : "transparent",
            }}
          >
            <span style={{ width: 32, color: "#94a3b8" }}>{index + 1}</span>
            <span style={{ flex: 1 }}>{row.source}</span>
            <span style={{ flex: 1, color: row.target ? "#0f172a" : "#94a3b8" }}>
              {row.target || "Click to translate…"}
            </span>
          </div>
        )}
      />
    </div>
  );
}

const meta: Meta<typeof BilingualGrid> = {
  title: "EditorGrid/VirtualList",
  component: BilingualGrid,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof BilingualGrid>;

/** Small list: below the virtualization threshold, every row is mounted. */
export const SmallList: Story = { args: { count: 12 } };

/** Large list: virtualized — only the rows near the viewport are mounted, and
 *  selecting a row scrolls it into view. */
export const Virtualized: Story = { args: { count: 2000 } };
