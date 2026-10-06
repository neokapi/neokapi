import type { KeptConflict } from "../../types/api";

/** An edit made on another machine that did not land on the Dutch title. */
export const editConflict: KeptConflict = {
  kind: "edit",
  doc: "src/en.json",
  locale: "nl",
  edit: "0pcm184gxrtanh4h3f6rs19m",
  blocks: [
    {
      block: "title",
      source: "Tide window",
      held: { text: "Tijvenster", rev: "r:7e19d9b8516eac94" },
      other: { text: "Getijdenvenster", rev: "r:d991eea02eadebf8" },
    },
  ],
};

/** Wording kept in the workspace that the French file, written since, does not hold. */
export const fileConflict: KeptConflict = {
  kind: "file",
  doc: "locales/en.json",
  locale: "fr",
  file: "locales/fr.json",
  blocks: [
    {
      block: "title",
      source: "Tide window",
      held: { text: "Autre chose", rev: "r:1a2b3c4d5e6f7081" },
      other: { text: "Fenêtre de marée", rev: "r:9f8e7d6c5b4a3921" },
    },
    {
      block: "cta",
      source: 'Plan a <x id="1"/>crossing',
      held: { text: "", rev: "absent", absent: true },
      other: { text: 'Planifier une <x id="1"/>traversée', rev: "r:0011223344556677" },
    },
  ],
};
