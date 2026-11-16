import Storage from "expo-sqlite/kv-store";

import { profileOf } from "../lib/profile";
import { loadRecentProject, parseRecentProject, setRecentProject } from "../lib/recentProject";

describe("profileOf", () => {
  it("prefers what the server says, and keys the colour on the user id", () => {
    expect(
      profileOf(
        { id: "u1", email: "ada@example.com", name: "Ada Lindqvist" },
        { email: "old@example.com" },
      ),
    ).toEqual({
      key: "u1",
      name: "Ada Lindqvist",
      ownName: "Ada Lindqvist",
      email: "ada@example.com",
    });
  });

  it("shows the server's shown name while the person has set none", () => {
    expect(profileOf({ id: "u1", email: "ada@example.com", shown_name: "ada" }, null)).toEqual({
      key: "u1",
      name: "ada",
      email: "ada@example.com",
    });
  });

  it("falls back to the session, then to the email before the @", () => {
    expect(profileOf(undefined, { userId: "u2", email: "bo@example.com" })).toEqual({
      key: "u2",
      name: "bo",
      email: "bo@example.com",
    });
    expect(profileOf(undefined, { email: "cy@example.com" }).key).toBe("cy@example.com");
    expect(profileOf(undefined, null)).toEqual({ key: "", name: "", email: undefined });
  });
});

describe("recent project", () => {
  it("reads only a well-formed stored value", () => {
    expect(parseRecentProject(null)).toBeNull();
    expect(parseRecentProject("not json")).toBeNull();
    expect(parseRecentProject('{"ws":"acme"}')).toBeNull();
    expect(parseRecentProject('{"ws":"acme","project":"p1","name":"Site"}')).toEqual({
      ws: "acme",
      project: "p1",
      name: "Site",
    });
  });

  it("keeps the choice across a start, and forgets it", async () => {
    setRecentProject({ ws: "acme", project: "p1", name: "Site" });
    await new Promise((r) => setTimeout(r, 0));
    expect(await Storage.getItem("bowrain.recent-project")).toContain('"project":"p1"');
    await loadRecentProject();
    setRecentProject(null);
    await new Promise((r) => setTimeout(r, 0));
    expect(await Storage.getItem("bowrain.recent-project")).toBeNull();
  });
});
