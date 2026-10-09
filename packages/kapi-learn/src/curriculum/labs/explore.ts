// Series 4, "Explore the engine": the engine explorers, played as labs.
//
// Each explorer opens one part of the engine on a file of its own or one the
// reader brings: the flow workspace, the segmentation comparison, format
// conversion, document structure from a PDF, vision, audio and video, the
// bundle format, and a free terminal. Here each is a lab: the poster boots
// what it needs, the chapters set what the explorer shows and say what to
// look at, and the explorer itself is the stage. The host resolves the
// explorer ids and maps each chapter's stage props to the component.

import type { Lab } from "../types.ts";

const DOCS = {
  flows: { label: "Flows", href: "/framework/flows" },
  pipeline: { label: "Pipeline", href: "/framework/pipeline" },
  tools: { label: "Tools", href: "/framework/tools" },
  segmentation: { label: "Segmentation", href: "/framework/segmentation" },
  formats: { label: "Formats", href: "/framework/formats" },
  contentModel: { label: "The content model", href: "/framework/content-model" },
  redaction: { label: "Redaction", href: "/framework/redaction" },
  structure: {
    label: "Document structure tiers",
    href: "/contribute/architecture/engine/e-08-document-structure-tiers",
  },
  multimodal: {
    label: "Multimodal content",
    href: "/contribute/architecture/multilingual/m-03-multimodal-content",
  },
  media: { label: "Translate audio and video", href: "/kapi/recipes/translate-media" },
  bundle: { label: "The content bundle", href: "/reference/serialization/content-bundle" },
  serialization: { label: "Serialization formats", href: "/reference/serialization/overview" },
  commands: { label: "Every command", href: "/commands" },
  quickstart: { label: "Quick Start", href: "/kapi/get-started/quickstart" },
  formatMaturity: { label: "Format maturity", href: "/format-maturity" },
};

export const flowWorkspace: Lab = {
  id: "flow-workspace",
  series: "explore",
  position: 1,
  title: "Flow workspace",
  tagline: "Build a flow on the canvas, run it, and open what each step attached.",
  summary:
    "A flow is a composition of tools, and the engine runs it as a pipeline over every part of a document. The workspace draws the flow as a graph, runs it in the browser with tracing on, and replays the trace on the same nodes, so a step's effect on each block can be opened. Each chapter loads one teaching scenario; its own walkthrough card points into the canvas.",
  sample: "engine",
  kind: "explorer",
  explorer: "flow",
  concepts: ["flow", "tool", "pipeline", "trace", "overlay", "project presets"],
  docs: [DOCS.flows, DOCS.pipeline, DOCS.tools, DOCS.redaction],
  minutes: 8,
  chapters: [
    {
      id: "annotations",
      title: "What a step attaches",
      narration:
        "Segmentation, translation and a check, run as one flow. Each step leaves something on the block: sentence spans as an overlay, a target in another language, findings. Press Run in the workspace and open a node to see what it attached.",
      hint: "scenario: annotations",
      stage: { scenario: "annotations" },
    },
    {
      id: "redaction",
      title: "Protect before you send",
      narration:
        "The redact step replaces sensitive spans with placeholders and vaults the originals; translate works on the protected text; unredact puts the originals back into the translation. The rules live at project level, so the step on the canvas is bare and still knows what to redact.",
      hint: "scenario: redaction",
      stage: { scenario: "redaction" },
    },
    {
      id: "segmentation",
      title: "Segment, then translate",
      narration:
        "Segmentation is an overlay on the content model rather than a structural split: the block stays whole, and the sentence boundaries are stand-off spans the next step reads.",
      hint: "scenario: segmentation",
      stage: { scenario: "segmentation" },
    },
    {
      id: "project",
      title: "The recipe behind the canvas",
      narration:
        "Every flow on the canvas serializes to a kapi.yaml recipe, and the project panel shows the defaults form beside the live source. Bindings choose where content enters and where results go, which is the whole difference between an ad-hoc run and a project.",
      hint: "scenario: project",
      stage: { scenario: "project" },
    },
    {
      id: "scripting",
      title: "A script as a step",
      narration:
        "The script step runs your code over every part in the pipeline, with the same part model the built-in tools see. Edit the code in the panel, run, and read the log.",
      hint: "scenario: scripting",
      stage: { scenario: "scripting" },
    },
    {
      id: "build-your-own",
      title: "Build your own",
      narration:
        "An empty canvas. Drop tools from the palette, connect them, pick a file from the library or upload one, and run. The recorded native traces in the picker show what the browser cannot run live: parallel workers and the Java bridge.",
      hint: "scenario: build your own",
      stage: { scenario: "build-your-own" },
    },
  ],
  tryNext: [
    "Add a second file to the working set and run the flow over both.",
    "Open the project panel's source tab and copy the recipe into your own project.",
  ],
};

export const segmentation: Lab = {
  id: "segmentation",
  series: "explore",
  position: 2,
  title: "Segmentation",
  tagline: "Compare the sentence splitters on the cases that trip them up.",
  summary:
    "Before text can be translated well it has to be split into sentences correctly: an abbreviation, a price, a quotation, a script with no spaces. The engine offers rule-based, Unicode, hybrid and learned segmenters. This lab runs them side by side on the tricky cases and shows where they agree, as overlays on the same blocks.",
  sample: "engine",
  kind: "explorer",
  explorer: "segmentation",
  concepts: ["segmentation", "SRX", "UAX-29", "overlay", "agreement"],
  docs: [DOCS.segmentation, DOCS.contentModel],
  minutes: 5,
  chapters: [
    {
      id: "abbrev",
      title: "Abbreviations and decimals",
      narration:
        "Dr. Smith, $3.50, Jan. 5: every full stop that is not a sentence end. The four instant engines run as soon as the chapter opens; the colours mark boundaries two or more engines drew in the same place.",
      hint: "sample: abbreviations",
      stage: { sample: "abbrev", autoRun: true },
      notice: ["SRX rules", "UAX-29", "Hybrid"],
    },
    {
      id: "dialogue",
      title: "Dialogue and quotes",
      narration:
        "A question mark inside quotation marks, then a comma: whether the quote closes the sentence is exactly where rule sets differ.",
      hint: "sample: dialogue",
      stage: { sample: "dialogue", autoRun: true },
    },
    {
      id: "cjk",
      title: "No spaces at all",
      narration:
        "Japanese marks sentence ends with its own punctuation and no spaces. The locale is set with the sample, so each engine applies its rules for the language rather than for English.",
      hint: "sample: Japanese",
      stage: { sample: "cjk", autoRun: true },
    },
    {
      id: "paragraphs",
      title: "Paragraphs are blocks",
      narration:
        "Two paragraphs become two blocks before any engine runs: the content model's structure comes first, and segmentation works inside each block.",
      hint: "sample: paragraphs",
      stage: { sample: "paragraphs", autoRun: true },
    },
    {
      id: "learned",
      title: "A learned model, on request",
      narration:
        "SaT and the local language models are opt-in: select one in the engine list and run again. Each downloads its weights the first time, and the download shows in the navbar as the plugin loads.",
      hint: "sample: abbreviations · opt in to SaT",
      stage: { sample: "abbrev" },
    },
  ],
  tryNext: [
    "Paste a paragraph of your own and run the comparison.",
    "Upload a JSON or HTML file: its blocks are segmented rather than its markup.",
  ],
};

export const conversion: Lab = {
  id: "conversion",
  series: "explore",
  position: 3,
  title: "File conversion",
  tagline: "One document, every serialization, and what each one keeps.",
  summary:
    "The engine reads a document into the content model and a writer puts it out again in any generative format. This lab opens one document every way at once: the model-level tabs stay the same while the output format changes, which is the point. Each chapter picks an input and a target; the Rendered pane reads the converted output back through the engine.",
  sample: "engine",
  kind: "explorer",
  explorer: "conversion",
  concepts: [
    "reader and writer",
    "generative format",
    "round trip",
    "structure",
    "inline formatting",
  ],
  docs: [DOCS.formats, DOCS.formatMaturity, DOCS.contentModel],
  minutes: 5,
  chapters: [
    {
      id: "md-doclang",
      title: "Markdown to DocLang",
      narration:
        "An article with headings, a list and inline emphasis, re-expressed as DocLang, the engine's own structural XML. Every heading level and inline code survives, because both formats carry the same model.",
      hint: "article.md → DocLang",
      stage: { sample: "article-md", target: "doclang" },
    },
    {
      id: "md-html",
      title: "Markdown to HTML",
      narration:
        "The same article as HTML. Compare the Rendered pane with the Preview tab: the writer reconstructed the document from the model, and the two read alike.",
      hint: "article.md → HTML",
      stage: { sample: "article-md", target: "html" },
    },
    {
      id: "md-asciidoc",
      title: "Markdown to AsciiDoc",
      narration:
        "A table survives the crossing into AsciiDoc because the model holds it as a table, not as pipes and dashes.",
      hint: "article.md → AsciiDoc",
      stage: { sample: "article-md", target: "asciidoc" },
    },
    {
      id: "html-md",
      title: "HTML to Markdown",
      narration:
        "The other direction. A page's markup becomes Markdown's lighter syntax; what Markdown cannot say, such as an attribute, is the limit of the target rather than a loss in the model.",
      hint: "page.html → Markdown",
      stage: { sample: "page-html", target: "markdown" },
    },
    {
      id: "json-yaml",
      title: "A catalog to YAML",
      narration:
        "Keyed formats convert too: a JSON catalog as YAML keeps every key path, so the blocks are the same blocks with the same names.",
      hint: "messages.json → YAML",
      stage: { sample: "messages-json", target: "yaml" },
    },
    {
      id: "plaintext",
      title: "Down to plain text",
      narration:
        "Plain text is the floor: structure and inline formatting have nowhere to go, and the Rendered pane shows what is left. Skeleton-driven formats such as Word and InDesign are deliberately absent as targets: they inject into an original file rather than generating one.",
      hint: "article.md → plain text",
      stage: { sample: "article-md", target: "plaintext" },
    },
  ],
  tryNext: [
    "Upload a Markdown or HTML file of your own and try each target.",
    "Switch to the Blocks tab and watch it stay the same across targets.",
  ],
};

export const structure: Lab = {
  id: "structure",
  series: "explore",
  position: 4,
  title: "Structure and layout",
  tagline: "Reading order, outline and geometry, recovered from a PDF.",
  summary:
    "A PDF carries positioned text and little else. The engine's PDF reader, bridged to PDFium in WebAssembly, recovers the text with its geometry, and the structure tier infers the outline and the reading order. This lab opens three PDFs in the shared viewer: Layout places each block on the page, Structure shows the outline, Blocks lists the content.",
  sample: "engine",
  kind: "explorer",
  explorer: "pdf",
  plugins: ["pdfium"],
  concepts: ["geometry", "reading order", "structure tier", "PDFium bridge"],
  docs: [DOCS.structure, DOCS.formats],
  minutes: 4,
  chapters: [
    {
      id: "report",
      title: "A report with headings and a table",
      narration:
        "Open the Layout tab and compare the block positions with the page: the headings, the body text and the table cells each sit where the PDF drew them. Then open Structure for the outline the engine inferred.",
      hint: "report.pdf",
      stage: { samples: ["report.pdf", "invoice.pdf", "anatomy.pdf"] },
    },
    {
      id: "invoice",
      title: "An invoice: columns",
      narration:
        "Columns are where reading order goes wrong. Switch the file to the invoice and check the Blocks order against the page, especially the line items and the totals.",
      hint: "invoice.pdf",
      stage: { samples: ["invoice.pdf", "report.pdf", "anatomy.pdf"] },
    },
    {
      id: "anatomy",
      title: "The minimal case",
      narration:
        "A one-page document with a title and two paragraphs: the smallest PDF the reader turns into blocks, useful for seeing the geometry fields themselves.",
      hint: "anatomy.pdf",
      stage: { samples: ["anatomy.pdf", "report.pdf", "invoice.pdf"] },
    },
  ],
  tryNext: [
    "Upload a PDF of your own and read its Layout tab.",
    "Compare the Structure outline with the document's own headings.",
  ],
};

export const vision: Lab = {
  id: "vision",
  series: "explore",
  position: 5,
  title: "Vision",
  tagline: "Text and layout recognised in an image, on your device.",
  summary:
    "The native kapi-vision plugin reads text and page layout from images with two ONNX models. This lab runs the same models in the browser: recognition first, layout on request. Each chapter picks an image; select a box on the image or a line in the list and the other highlights.",
  sample: "engine",
  kind: "explorer",
  explorer: "vision",
  engine: false,
  plugins: ["vision"],
  concepts: ["OCR", "layout regions", "geometry", "handwriting fallback"],
  docs: [DOCS.multimodal],
  minutes: 4,
  chapters: [
    {
      id: "document",
      title: "A printed page",
      narration:
        "Recognition runs when the image opens. Each line comes back with its text and its box. Press Layout to run the second model, which labels regions: title, paragraph, table, figure.",
      hint: "document.png",
      stage: { samples: ["document", "hello", "handwriting", "report.docx"] },
    },
    {
      id: "hello",
      title: "A short sample",
      narration:
        "A few words at a large size: the fast case, and a good one for reading the geometry values.",
      hint: "hello.png",
      stage: { samples: ["hello", "document", "handwriting", "report.docx"] },
    },
    {
      id: "handwriting",
      title: "Handwriting",
      narration:
        "Printed-text recognition struggles here. Turn on the handwriting fallback: low-confidence lines are read again by a second model, loaded on first use.",
      hint: "handwriting.png",
      stage: { samples: ["handwriting", "document", "hello", "report.docx"] },
    },
    {
      id: "embedded",
      title: "An image inside a document",
      narration:
        "A Word file with an embedded image. The engine extracts the image through the document's reader, and recognition runs on that, which is how a figure's text reaches a translation.",
      hint: "report.docx",
      stage: { samples: ["report.docx", "document", "hello", "handwriting"] },
    },
  ],
  tryNext: ["Drop a photo of a page or a sign onto the explorer."],
};

export const media: Lab = {
  id: "media",
  series: "explore",
  position: 6,
  title: "Audio and video",
  tagline: "Speech and on-screen text as timed content.",
  summary:
    "Subtitles are timed blocks: a cue is a block with a start and an end, and on-screen text is a block with a box and a time. The first three chapters show the finished shape with recorded results, animated over a playhead. The last two run the real models on a sample clip: speech recognition, and the full demux, transcribe and frame-OCR pipeline.",
  sample: "engine",
  kind: "explorer",
  explorer: "multimodal",
  engine: false,
  concepts: ["timed blocks", "subtitle cues", "frame OCR", "speech recognition"],
  docs: [DOCS.media, DOCS.multimodal],
  minutes: 6,
  chapters: [
    {
      id: "image",
      title: "An image, translated",
      narration:
        "Recorded results, no model: an invoice's lines recognised and translated, drawn over the image at their boxes. Toggle the language to see the translation sit where the source was.",
      hint: "recorded: image",
      stage: { chapter: 0 },
    },
    {
      id: "audio-cues",
      title: "Audio as cues",
      narration:
        "Speech becomes subtitle cues with a start and an end. The playhead advances and the active cue highlights, exactly as it would over real playback.",
      hint: "recorded: audio",
      stage: { chapter: 1 },
    },
    {
      id: "video-cues",
      title: "Video: speech and frames",
      narration:
        "A video carries both: the speech track as cues, and the on-screen text as boxes with a time. Both are translated, and both are drawn at their moment.",
      hint: "recorded: video",
      stage: { chapter: 2 },
    },
    {
      id: "transcribe",
      title: "Transcribe for real",
      narration:
        "Whisper runs in the browser on the sample clip's audio. Press Transcribe: the model downloads once, and the cues appear in the player as they would from the native plugin.",
      hint: "live: audio",
      stage: { explorer: "audio" },
    },
    {
      id: "process",
      title: "The whole pipeline",
      narration:
        "Press Process: ffmpeg demuxes the clip into audio and frames, Whisper transcribes the speech, and recognition reads the frame text. The result is one timed document with both tracks.",
      hint: "live: video",
      stage: { explorer: "video" },
    },
  ],
  tryNext: ["Drop an audio file or a short clip of your own onto the explorer."],
};

export const kbfAnatomy: Lab = {
  id: "kbf-anatomy",
  series: "explore",
  position: 7,
  title: "Bundle anatomy",
  tagline: "One content bundle, read part by part, then round-tripped through the engine.",
  summary:
    "The content bundle is the interchange document of the toolchain: one deterministic JSON file that carries a document's blocks as runs, its editions per language, and the provenance of every string, so extraction, translation, checking and write-back can be separate steps. The first chapters read one realistic bundle part by part; the last ones edit a bundle and watch the engine canonicalize, preview and validate it.",
  sample: "engine",
  kind: "explorer",
  explorer: "kbf-anatomy",
  engine: true,
  concepts: ["content bundle", "envelope", "block", "run", "edition", "provenance", "overlay"],
  docs: [DOCS.bundle, DOCS.serialization],
  minutes: 6,
  chapters: [
    {
      id: "envelope",
      title: "The envelope",
      narration:
        "A bundle starts with what produced it and for which project. Select a line on the left or a term on the right to see what that part of the document is for.",
      hint: "part: envelope",
      stage: { term: "envelope" },
    },
    {
      id: "document",
      title: "The document",
      narration: "One source file, with its format and its path. A bundle may carry several.",
      hint: "part: document",
      stage: { term: "document" },
    },
    {
      id: "block",
      title: "A block and its key",
      narration: "The unit everything else hangs on: a stable key, and the content as editions.",
      hint: "part: block",
      stage: { term: "block" },
    },
    {
      id: "editions",
      title: "Editions",
      narration:
        "The source language and each translation are peers on the block, keyed by locale. One is already Norwegian; the plural still awaits a translation.",
      hint: "part: editions",
      stage: { term: "editions" },
    },
    {
      id: "runs",
      title: "Runs: text and codes",
      narration:
        "An edition's content is a sequence of runs: text, paired codes for inline markup, placeholders for variables. The markup never sits inside the text.",
      hint: "part: run-text, run-pc, run-ph",
      stage: { term: "run-pc" },
    },
    {
      id: "plural",
      title: "A plural",
      narration:
        "A plural is one run whose branches each hold their own runs, so a translation cannot drop a category unnoticed.",
      hint: "part: run-plural",
      stage: { term: "run-plural" },
    },
    {
      id: "provenance",
      title: "Provenance",
      narration: "Where each string came from and when: the fields a reviewer and a gate read.",
      hint: "part: provenance",
      stage: { term: "provenance" },
    },
    {
      id: "roundtrip",
      title: "Round-trip through the engine",
      narration:
        "Now the real engine: edit the bundle on the left and the canonical Go implementation, compiled to WebAssembly, parses it, canonicalizes the bytes, renders each block's preview, validates the run structure and resolves the companion overlay anchor by anchor.",
      hint: "live: edit a bundle",
      stage: { explorer: "kbf-explorer", sample: "full" },
    },
    {
      id: "invalid",
      title: "What validation refuses",
      narration:
        "A bundle with an unbalanced pair: the engine names the block, the run and the rule it broke. Fix it in the editor and the preview returns.",
      hint: "live: an invalid bundle",
      stage: { explorer: "kbf-explorer", sample: "invalid" },
    },
  ],
  tryNext: [
    "Add a target edition by hand and watch the preview switch.",
    "Change an overlay anchor's offsets and see how the resolution reports it.",
  ],
};

export const freeTerminal: Lab = {
  id: "free-terminal",
  series: "explore",
  position: 8,
  title: "Free terminal",
  tagline: "A terminal of your own, with sample files to start from.",
  summary:
    "The same terminal the labs type into, with nothing scripted beyond a few openers. The KapiMart files are in the sandbox, and each chapter drops in one more sample in another format and runs a first command on it. Type anything the browser engine supports; help lists what the terminal itself knows.",
  sample: "mart",
  concepts: ["ad-hoc commands", "formats", "the browser engine"],
  docs: [DOCS.commands, DOCS.quickstart],
  minutes: 5,
  chapters: [
    {
      id: "help",
      title: "What runs here",
      narration:
        "The browser build mirrors the native command set verb for verb. The verbs it cannot run, such as plugins and credentials, say why.",
      command: "kapi --help",
      expect: ["Usage:"],
    },
    {
      id: "json",
      title: "A JSON catalog",
      narration: "KapiMart's catalog, measured.",
      command: "kapi stats src/en.json",
      expect: ["Words:"],
      look: { file: "src/en.json", view: "preview" },
    },
    {
      id: "html",
      title: "An HTML page",
      narration: "A page is dropped into the sandbox and inspected: the text, free of its markup.",
      files: [
        {
          path: "page.html",
          content:
            '<!doctype html>\n<html lang="en">\n  <head>\n    <meta charset="utf-8" />\n    <title>Welcome</title>\n  </head>\n  <body>\n    <h1>Welcome aboard</h1>\n    <p>Thanks for trying <strong>kapi</strong>. Edit this file and run a command.</p>\n    <a href="/docs">Read the documentation</a>\n  </body>\n</html>\n',
        },
      ],
      command: "kapi inspect page.html",
      expect: ["Welcome aboard"],
      look: { file: "page.html", view: "blocks" },
    },
    {
      id: "xliff",
      title: "A bilingual file",
      narration:
        "An XLIFF holds source and target together. Pseudo-translation fills the targets with accented text, which a layout test needs and a reviewer never mistakes for a translation.",
      files: [
        {
          path: "app.xliff",
          content:
            '<?xml version="1.0" encoding="UTF-8"?>\n<xliff xmlns="urn:oasis:names:tc:xliff:document:2.2" version="2.2" srcLang="en" trgLang="fr">\n  <file id="app.json" original="app.json">\n    <unit id="greeting">\n      <segment>\n        <source>Hello, World!</source>\n      </segment>\n    </unit>\n    <unit id="farewell">\n      <segment>\n        <source>See you tomorrow</source>\n      </segment>\n    </unit>\n  </file>\n</xliff>\n',
        },
      ],
      command: "kapi pseudo-translate app.xliff -o app.qps.xliff",
      expect: ["completed"],
      look: { file: "app.qps.xliff", view: "raw" },
    },
    {
      id: "properties",
      title: "A Java properties file",
      narration: "One more format, one more reader, the same blocks.",
      files: [
        {
          path: "app.properties",
          content:
            "# Application strings\napp.title = Welcome aboard\napp.greeting = Hello, World!\ncart.empty = Your cart is empty\n",
        },
      ],
      command: "kapi inspect app.properties | head -20",
      expect: ["app.title"],
      look: { file: "app.properties", view: "blocks" },
    },
    {
      id: "yours",
      title: "Your own files",
      narration:
        "Add a file with the files pane's upload button, then run a command on it. Everything stays in this tab; download what you want to keep.",
      command: "ls",
      expect: ["src/"],
    },
  ],
  tryNext: [
    "kapi formats",
    "kapi tools schema translate",
    "kconv src/about.md -o about.adoc",
    "kapi check src/about.md --forbid 'seamless'",
  ],
};

export const EXPLORE_LABS: Lab[] = [
  flowWorkspace,
  segmentation,
  conversion,
  structure,
  vision,
  media,
  kbfAnatomy,
  freeTerminal,
];
