import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { SubtitleTimeline, MediaCanvas, DocumentViewer } from "@neokapi/ui-primitives/preview";
import type { ContentNode, ContentTree } from "@neokapi/ui-primitives/preview";

// Multimodal previews (AD-030): a timed source (subtitles / audio / video) yields
// cue blocks carrying a `timing` anchor, and a raster source (an image OCR'd by
// kapi-vision) yields a media node plus geometry-anchored text blocks. These
// fixtures mirror the `kapi inspect` ContentTree for those shapes so the timeline
// and the raster OCR overlay can be exercised without booting WASM.

const meta: Meta = {
  title: "Lab/PreviewKit/Multimodal",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

function tree(format: string, root: ContentNode[]): ContentTree {
  return { format, root, stats: { layers: 0, groups: 0, blocks: 0, data: 0, media: 0, runs: 0 } };
}

function cue(id: string, startMs: number, endMs: number, src: string, tgt?: string): ContentNode {
  return {
    kind: "block",
    id,
    type: "subtitle",
    source: [{ text: src }],
    targets: tgt ? { "fr-FR": [{ text: tgt }] } : undefined,
    timing: { startMs, endMs },
  };
}

const subtitleTree = tree("vtt", [
  {
    kind: "layer",
    id: "doc",
    name: "talk.vtt",
    children: [
      cue("c1", 0, 2500, "Welcome to the show.", "Bienvenue à l'émission."),
      cue(
        "c2",
        2500,
        5000,
        "Today we explore localization.",
        "Aujourd'hui, nous explorons la localisation.",
      ),
      cue("c3", 5000, 8000, "Let's begin.", "Commençons."),
    ],
  },
]);

function ocrLine(
  id: string,
  x: number,
  y: number,
  w: number,
  h: number,
  src: string,
  role: string,
): ContentNode {
  return {
    kind: "block",
    id,
    type: "line",
    source: [{ text: src }],
    geometry: { x, y, w, h, resolution: 100 },
    structure: { role },
  };
}

// A tiny inline SVG raster stands in for an OCR'd document image. Base64 (not
// the `;utf8,` form, which some browsers refuse) so it always renders.
const OCR_SVG =
  "<svg xmlns='http://www.w3.org/2000/svg' width='400' height='300'>" +
  "<rect width='400' height='300' fill='#0f172a'/>" +
  "<text x='20' y='64' fill='#f8fafc' font-family='sans-serif' font-size='30'>Invoice</text>" +
  "<text x='20' y='150' fill='#cbd5e1' font-family='sans-serif' font-size='20'>Total: $42.00</text>" +
  "</svg>";
const OCR_IMAGE = "data:image/svg+xml;base64," + btoa(OCR_SVG);

const imageTree = tree("image", [
  {
    kind: "media",
    id: "m1",
    media: { mimeType: "image/svg+xml", filename: "invoice.svg", uri: OCR_IMAGE },
  },
  ocrLine("o1", 5, 14, 22, 12, "Invoice", "heading"),
  ocrLine("o2", 5, 42, 36, 9, "Total: $42.00", "paragraph"),
]);

export const Subtitles: Story = {
  name: "Subtitle timeline",
  render: () => <SubtitleTimeline tree={subtitleTree} currentTimeMs={3000} className="max-w-2xl" />,
};

const MediaCanvasDemo = () => {
  const [selected, setSelected] = useState<string | null>(null);
  return (
    <MediaCanvas
      src={OCR_IMAGE}
      tree={imageTree}
      selectedBlockId={selected}
      onSelectBlock={setSelected}
      alt="Invoice"
      className="max-w-md"
    />
  );
};

export const ImageOCR: Story = {
  name: "Image OCR overlay",
  render: () => <MediaCanvasDemo />,
};

export const ImageInViewer: Story = {
  name: "Image in DocumentViewer",
  render: () => (
    <DocumentViewer
      tree={imageTree}
      filename="invoice.svg"
      resolveMediaUrl={(n) => n.media?.uri}
      className="max-w-2xl"
    />
  ),
};

export const SubtitlesInViewer: Story = {
  name: "Subtitles in DocumentViewer",
  render: () => <DocumentViewer tree={subtitleTree} filename="talk.vtt" className="max-w-2xl" />,
};
