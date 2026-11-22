// Where a card's text is read, in words: the product and the surface its
// collection's recipe binds, rather than the file path the text was read from.
import type { DeckCollection } from "@neokapi/bowrain-client";

/** A recipe slug as a reader names it: `kapi-desktop` reads `Kapi desktop`. */
export function humanize(slug: string): string {
  const words = slug.replace(/[-_]+/g, " ").trim();
  return words ? words[0].toUpperCase() + words.slice(1) : "";
}

/**
 * The product and surface a collection's content ships on. Coordinates win
 * over the channel binding; a binding written `product/channel` gives both.
 * Falls back to the collection's name, and is empty for no collection.
 */
export function whereParts(
  c: Pick<DeckCollection, "name" | "channel" | "coordinates"> | undefined,
): string[] {
  if (!c) return [];
  const coords = c.coordinates ?? {};
  let product = coords.product ?? "";
  let channel = coords.channel ?? "";
  if (c.channel) {
    const [head, tail] = c.channel.includes("/") ? c.channel.split("/", 2) : ["", c.channel];
    product ||= head;
    channel ||= tail;
  }
  const parts = [product, channel].filter(Boolean).map(humanize);
  if (parts.length === 0 && c.name) parts.push(humanize(c.name));
  return parts;
}

/** The file name of an item path: `apps/x/build/config.yml` reads `config.yml`. */
export function fileName(path: string): string {
  const i = path.lastIndexOf("/");
  return i < 0 ? path : path.slice(i + 1);
}
