import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { DataPreview } from "@neokapi/ui-primitives/preview";
import type { ContentNode, ContentTree, Run } from "@neokapi/ui-primitives/preview";

// DataPreview is the reading a catalog file gets: units addressed by a key
// path, one row each, grouped the way the file nests, with the source beside
// the target. The document reading renders the same file as a column of
// strings, which for `messages.json` leaves a reviewer looking at French with
// nothing to say which string is the button and which is the error.
//
// The code view is a capability the host supplies. The desktop has the file on
// disk and the core writer, so it passes one; the platform stores blocks rather
// than files and passes none, and the toggle does not appear.

const meta: Meta<typeof DataPreview> = {
  title: "Lab/PreviewKit/DataPreview",
  component: DataPreview,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof DataPreview>;

function txt(s: string): Run[] {
  return [{ text: s }];
}

function unit(key: string, source: Run[], target: Run[]): ContentNode {
  return {
    kind: "block",
    id: `b:${key}`,
    name: key,
    translatable: true,
    source,
    targets: { "fr-FR": target },
  };
}

function tree(format: string, name: string, units: ContentNode[]): ContentTree {
  return {
    format,
    root: [{ kind: "layer", id: name, name, children: units }],
    stats: { layers: 1, groups: 0, blocks: units.length, data: 0, media: 0, runs: 0 },
  };
}

/** A line break as the engine emits it: a placeholder in the break vocabulary. */
const BR: Run = { ph: { id: "br", type: "struct:break", data: "<br/>" } };

// ── A nested JSON message catalog ────────────────────────────────────────────

const messagesJson = tree("json", "messages.json", [
  unit("title", txt("Kapimart"), txt("Kapimart")),
  unit(
    "intro",
    [{ text: "First line" }, BR, { text: "Second line" }],
    [{ text: "Première ligne" }, BR, { text: "Deuxième ligne" }],
  ),
  unit(
    "cart.summary",
    [
      { text: "Your basket holds " },
      { ph: { id: "1", type: "code:variable", data: "{count}", equiv: "count" } },
      { text: " items." },
    ],
    [
      { text: "Votre panier contient " },
      { ph: { id: "1", type: "code:variable", data: "{count}", equiv: "count" } },
      { text: " articles." },
    ],
  ),
  unit("cart.empty", txt("Your basket is empty"), txt("Votre panier est vide")),
  unit("errors.network.timeout", txt("The request timed out"), txt("La requête a expiré")),
  unit("errors.network.refused", txt("The server refused the connection"), []),
  unit("errors.auth.expired", txt("Your session has expired"), txt("Votre session a expiré")),
]);

const messagesJsonFile = `{
  "title": "Kapimart",
  "intro": "Première ligne<br/>Deuxième ligne",
  "cart": {
    "summary": "Votre panier contient {count} articles.",
    "empty": "Votre panier est vide"
  },
  "errors": {
    "network": {
      "timeout": "La requête a expiré",
      "refused": "The server refused the connection"
    },
    "auth": {
      "expired": "Votre session a expiré"
    }
  }
}
`;

// ── A YAML catalog with an array ─────────────────────────────────────────────

const contentYaml = tree("yaml", "content.yaml", [
  unit("site.name", txt("Kapimart"), txt("Kapimart")),
  unit("site.tagline", txt("Everything, faithfully"), txt("Tout, fidèlement")),
  unit("nav[0].label", txt("Home"), txt("Accueil")),
  unit("nav[1].label", txt("Catalogue"), txt("Catalogue")),
  unit("nav[2].label", txt("Basket"), txt("Panier")),
  unit("footer.legal", txt("All rights reserved"), txt("Tous droits réservés")),
]);

const contentYamlFile = `site:
  name: Kapimart
  tagline: Tout, fidèlement
nav:
  - label: Accueil
  - label: Catalogue
  - label: Panier
footer:
  legal: Tous droits réservés
`;

// ── Stories ──────────────────────────────────────────────────────────────────

export const Json: Story = {
  name: "JSON catalog",
  render: () => <DataPreview tree={messagesJson} locale="fr-FR" sourceLocale="en" />,
};

export const Yaml: Story = {
  name: "YAML catalog",
  render: () => <DataPreview tree={contentYaml} locale="fr-FR" sourceLocale="en" />,
};

export const SourceOnly: Story = {
  name: "Source only (no locale in view)",
  render: () => <DataPreview tree={messagesJson} sourceLocale="en" />,
};

export const WithCodeView: Story = {
  name: "With the written-back file",
  render: function WithCode() {
    const [selected, setSelected] = useState<string | undefined>("b:errors.network.timeout");
    return (
      <DataPreview
        tree={messagesJson}
        locale="fr-FR"
        sourceLocale="en"
        selectedBlockId={selected}
        onSelectBlock={setSelected}
        code={{ text: messagesJsonFile, filename: "messages.fr-FR.json" }}
      />
    );
  },
};

export const YamlCodeView: Story = {
  name: "YAML, written back",
  render: () => (
    <DataPreview
      tree={contentYaml}
      locale="fr-FR"
      sourceLocale="en"
      view="code"
      code={{ text: contentYamlFile, filename: "content.fr-FR.yaml" }}
    />
  ),
};

export const Selection: Story = {
  name: "Selecting a unit",
  render: function Selecting() {
    const [selected, setSelected] = useState<string | undefined>();
    return (
      <div className="space-y-2">
        <p className="text-xs text-muted-foreground">Selected: {selected ?? "none"}</p>
        <DataPreview
          tree={messagesJson}
          locale="fr-FR"
          sourceLocale="en"
          selectedBlockId={selected}
          onSelectBlock={setSelected}
          blockAttrs={(id) => ({ "data-testid": `unit-${id}` })}
        />
      </div>
    );
  },
};
