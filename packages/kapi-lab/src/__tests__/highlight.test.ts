import { describe, expect, it } from "vitest";
import { detectLang, tokenize } from "../highlight";

describe("detectLang", () => {
  it("maps extensions to languages", () => {
    expect(detectLang("messages.json")).toBe("json");
    expect(detectLang("app.xliff")).toBe("xml");
    expect(detectLang("page.html")).toBe("xml");
    expect(detectLang("app.properties")).toBe("properties");
    expect(detectLang("strings.po")).toBe("po");
    expect(detectLang("notes.md")).toBe("markdown");
    expect(detectLang("data.csv")).toBe("csv");
    expect(detectLang("README")).toBe("text");
  });
});

describe("tokenize", () => {
  it("distinguishes JSON keys from string values", () => {
    const line = tokenize('{"k": "v"}', "json")[0];
    expect(line.some((t) => t.type === "key" && t.text === '"k"')).toBe(true);
    expect(line.some((t) => t.type === "string" && t.text === '"v"')).toBe(true);
  });

  it("recognises JSON numbers and booleans", () => {
    const line = tokenize('{"n": 42, "b": true}', "json")[0];
    expect(line.some((t) => t.type === "number" && t.text === "42")).toBe(true);
    expect(line.some((t) => t.type === "boolean" && t.text === "true")).toBe(true);
  });

  it("tokenises XML tags and attributes", () => {
    const line = tokenize('<a href="/x">', "xml")[0];
    expect(line.some((t) => t.type === "tag")).toBe(true);
    expect(line.some((t) => t.type === "attr" && t.text === "href")).toBe(true);
  });

  it("marks properties comments and keys", () => {
    const lines = tokenize("# a comment\napp.title = Hello", "properties");
    expect(lines[0][0].type).toBe("comment");
    expect(lines[1].some((t) => t.type === "key")).toBe(true);
  });

  it("covers every character of every line", () => {
    const text = '{"greeting": "Hello, {name}!"}';
    const joined = tokenize(text, "json")[0]
      .map((t) => t.text)
      .join("");
    expect(joined).toBe(text);
  });
});
