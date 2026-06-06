// Workspace demo fixtures for the .klz WorkspaceExplorer. Office samples are
// tiny, hand-built OOXML packages base64-embedded so the explorer is
// self-contained (no network). The text sample is plain JSON.

function bytesFromBase64(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

export interface WorkspaceSample {
  id: string;
  label: string;
  filename: string;
  /** Human note about the format. */
  kind: string;
  /** Bytes seeded into the in-memory filesystem. */
  bytes: () => Uint8Array;
  /** True for binary formats (don't render the raw bytes as text). */
  binary: boolean;
  /**
   * A project TMX (en→fr) whose source segments match this sample's
   * translatable text, so `kapi tm import` + the `translate` (tm-leverage) flow
   * fill real `fr` targets offline — no LLM. The ProjectExplorer seeds this
   * before extract; the merged output is a genuine localized file.
   */
  tmx: string;
}

const enc = new TextEncoder();

const JSON_SAMPLE = `{
  "greeting": "Welcome to Acme",
  "cta": "Sign up today",
  "farewell": "Talk soon"
}
`;

// A minimal en→fr TMX. Each <tu> pairs a source segment with its French
// translation; the ProjectExplorer imports this into the project TM so the
// `translate` (tm-leverage) flow fills real targets offline.
function tmx(pairs: [string, string][]): string {
  const tus = pairs
    .map(
      ([en, fr]) =>
        `    <tu>\n      <tuv xml:lang="en"><seg>${en}</seg></tuv>\n      <tuv xml:lang="fr"><seg>${fr}</seg></tuv>\n    </tu>`,
    )
    .join("\n");
  return `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="neokapi" creationtoolversion="1.0"
          segtype="sentence" o-tmf="unknown" adminlang="en"
          srclang="en" datatype="plaintext"/>
  <body>
${tus}
  </body>
</tmx>
`;
}

const JSON_TMX = tmx([
  ["Welcome to Acme", "Bienvenue chez Acme"],
  ["Sign up today", "Inscrivez-vous aujourd'hui"],
  ["Talk soon", "À bientôt"],
]);

// Matches the translatable text baked into DOCX_B64's word/document.xml.
const DOCX_TMX = tmx([
  ["Welcome to Acme", "Bienvenue chez Acme"],
  ["Your account is ready.", "Votre compte est prêt."],
  ["Sign in to continue", "Connectez-vous pour continuer"],
]);

// Matches the shared strings baked into XLSX_B64's xl/sharedStrings.xml.
const XLSX_TMX = tmx([
  ["Total revenue", "Chiffre d'affaires total"],
  ["Net profit", "Bénéfice net"],
]);

const DOCX_B64 =
  "UEsDBBQAAAAIAOm6xFzmdcR+0gAAAIsBAAATAAAAW0NvbnRlbnRfVHlwZXNdLnhtbH2QvVLDMAzHX8XnlasVGBh6SToAKzD0BXSOkvjw11luad++Sls6cIVR+n/8ZLebQ/BqT4Vdip1+NI3e9O32mImVKJE7Pdea1wBsZwrIJmWKooypBKwylgky2i+cCJ6a5hlsipViXdWlQ/ftK42481W9HWR9oRTyrNXLxbiwOo05e2exig77OPyirK4EI8mzh2eX+UEMGu4SFuVvwDX3Ic8ubiD1iaW+YxAXfKcywJDsLkjS/F9z5840js7SLb+05ZIsMbs4BW9uSkAXf+6H83f3J1BLAwQUAAAACADpusRcXzOVUpUAAAAHAQAACwAAAF9yZWxzLy5yZWxzjc87DsIwDAbgq0Q+QJ0yMKCmXVi6Ii4QJW5T0TzkhNftycBAEQOjf//6LHfDw6/iRpyXGBS0jYSh70606lKD7JaURW2ErMCVkg6I2TjyOjcxUaibKbLXpY48Y9LmomfCnZR75E8DtqYYrQIebQvi/Ez0jx2naTF0jObqKZQfJ74aVdY8U1Fwj2zRvuOmsoB9h5sX+xdQSwMEFAAAAAgA6brEXO2a4OirAAAAIQEAABEAAAB3b3JkL2RvY3VtZW50LnhtbIWPSwrCMBCGrxJygKa6cFH6wDO4EJcxHdtAMxMmibW3NxFEEMHNN8zz/6cdHm4Rd+BgCTu5q2o59O3ajGSSA4witzE0ayfnGH2jVDAzOB0q8oC5dyN2OuaUJ7USj57JQAgWJ7eofV0flNMWZTl5pXEr0RdwQezPsBhyICKJo3HQqlIs5Bf99/yFEgttDKVszQbBoMet+rt2shMKi0XGEEaL6ZeUentUn//7J1BLAQIUAxQAAAAIAOm6xFzmdcR+0gAAAIsBAAATAAAAAAAAAAAAAACAAQAAAABbQ29udGVudF9UeXBlc10ueG1sUEsBAhQDFAAAAAgA6brEXF8zlVKVAAAABwEAAAsAAAAAAAAAAAAAAIABAwEAAF9yZWxzLy5yZWxzUEsBAhQDFAAAAAgA6brEXO2a4OirAAAAIQEAABEAAAAAAAAAAAAAAIABwQEAAHdvcmQvZG9jdW1lbnQueG1sUEsFBgAAAAADAAMAuQAAAJsCAAAAAA==";
const XLSX_B64 =
  "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";

export const WORKSPACE_SAMPLES: WorkspaceSample[] = [
  {
    id: "json",
    label: "JSON catalog",
    filename: "messages.json",
    kind: "text · JSON",
    bytes: () => enc.encode(JSON_SAMPLE),
    binary: false,
    tmx: JSON_TMX,
  },
  {
    id: "docx",
    label: "Word document",
    filename: "welcome.docx",
    kind: "binary · OOXML (.docx)",
    bytes: () => bytesFromBase64(DOCX_B64),
    binary: true,
    tmx: DOCX_TMX,
  },
  {
    id: "xlsx",
    label: "Excel sheet",
    filename: "report.xlsx",
    kind: "binary · OOXML (.xlsx)",
    bytes: () => bytesFromBase64(XLSX_B64),
    binary: true,
    tmx: XLSX_TMX,
  },
];

export function workspaceSampleById(id: string): WorkspaceSample {
  return WORKSPACE_SAMPLES.find((s) => s.id === id) ?? WORKSPACE_SAMPLES[0];
}

export { JSON_SAMPLE };
