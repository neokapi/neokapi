// The language picker is a function of ship state, not of which catalogs exist.
//
// ship.json is written by the build, verbatim:
//
//     kapi status --ship --emit site/ship.json
//
// and is a map of locale → {shippable, verified}. The two flags are independent
// gates, and each one means something a reader can act on:
//
//   shippable && verified   the locale is offered, unmarked. A person reviewed it.
//   shippable && !verified  the locale is offered, marked AI. It clears the ship
//                           bar, so it is safe to read, but nobody has signed
//                           off every string in it yet.
//   !shippable              the locale is NOT offered. It exists in the
//                           repository, it has translations, and it is not ready
//                           to be read by an operator at 04:00.
//
// The last line is the point. A catalog file being present is not a decision;
// the ship gate is, and the picker only ever offers what the gate cleared.

const SOURCE_LOCALE = "en-GB";

const LANGUAGE_NAMES = {
  "en-GB": "English",
  nb: "Norsk bokmål",
  de: "Deutsch",
  nl: "Nederlands",
};

const picker = document.getElementById("language");
const marker = document.getElementById("marker");
const note = document.getElementById("note");

async function loadJSON(path) {
  const res = await fetch(path, { cache: "no-store" });
  if (!res.ok) throw new Error(`${path}: ${res.status}`);
  return res.json();
}

function lookup(catalog, key) {
  return key.split(".").reduce((node, part) => (node == null ? node : node[part]), catalog);
}

function render(catalog) {
  for (const el of document.querySelectorAll("[data-key]")) {
    const value = lookup(catalog, el.dataset.key);
    el.textContent = typeof value === "string" ? value : "";
  }
}

// The source locale is always offered: it is the content, not a translation of
// it, so no gate stands between it and a reader.
function offered(ship) {
  const entries = [[SOURCE_LOCALE, { shippable: true, verified: true }]];
  for (const [locale, state] of Object.entries(ship)) {
    if (locale !== SOURCE_LOCALE && state.shippable) entries.push([locale, state]);
  }
  return entries;
}

async function select(locale, state) {
  document.documentElement.lang = locale;
  marker.hidden = state.verified;
  render(await loadJSON(`./locales/${locale}.json`));
}

async function main() {
  const ship = await loadJSON("./ship.json");
  const entries = offered(ship);
  const withheld = Object.entries(ship).filter(([, s]) => !s.shippable).length;

  for (const [locale, state] of entries) {
    const option = document.createElement("option");
    option.value = locale;
    option.textContent = LANGUAGE_NAMES[locale] ?? locale;
    option.dataset.verified = String(state.verified);
    picker.append(option);
  }

  picker.addEventListener("change", () => {
    const chosen = entries.find(([locale]) => locale === picker.value);
    void select(chosen[0], chosen[1]);
  });

  note.textContent =
    `${entries.length} language${entries.length === 1 ? "" : "s"} offered` +
    (withheld ? `, ${withheld} withheld by the ship gate` : "") +
    ". The picker is built from ship.json, which kapi writes.";

  await select(entries[0][0], entries[0][1]);
}

void main();
