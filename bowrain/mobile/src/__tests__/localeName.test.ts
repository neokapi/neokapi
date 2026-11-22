import { localeName } from "../lib/localeName";

describe("localeName", () => {
  it("names a language in the UI language", () => {
    expect(localeName("fr", "en")).toBe("French");
    expect(localeName("de", "fr")).toBe("allemand");
    expect(localeName("pt_BR", "en")).toBe("Brazilian Portuguese");
  });

  it("gives null for a code it cannot name", () => {
    expect(localeName("qqq", "en")).toBeNull();
    expect(localeName("not a tag", "en")).toBeNull();
  });
});
