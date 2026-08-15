import { describe, it, expect } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import { StarterPromptCard, starterPromptText } from "../components/StarterPromptCard";

describe("starterPromptText", () => {
  it("names the workspace and the server it should connect to", () => {
    expect(starterPromptText({ workspaceName: "Acme", serverUrl: "https://bw.example" })).toBe(
      "Install the kapi skill, then: set up a brand starter pack for the Acme workspace and connect this project to Bowrain at https://bw.example.",
    );
  });

  it("falls back to an unnamed workspace and omits an absent server", () => {
    expect(starterPromptText({})).toBe(
      "Install the kapi skill, then: set up a brand starter pack for this workspace and connect this project to Bowrain.",
    );
  });

  it("folds a known repository in, so the assistant is told what to read", () => {
    expect(
      starterPromptText({
        workspaceName: "Acme",
        serverUrl: "https://bw.example",
        repoUrl: "https://github.com/acme/website",
      }),
    ).toBe(
      "Install the kapi skill, then: read https://github.com/acme/website, set up a brand starter pack for the Acme workspace, and connect this repository to Bowrain at https://bw.example.",
    );
  });
});

describe("StarterPromptCard", () => {
  it("renders the prompt and the docs link", () => {
    render(<StarterPromptCard workspaceName="Acme" serverUrl="https://bw.example" />);

    expect(screen.getByText("Build your brand starter pack with your AI")).toBeInTheDocument();
    expect(screen.getByTestId("starter-prompt").textContent).toContain("the Acme workspace");
    expect(screen.getByRole("link", { name: /How the starter pack works/ })).toHaveAttribute(
      "href",
      "https://bowrain.cloud/docs/quickstart",
    );
  });

  it("names the repository in the body when one is known", () => {
    render(<StarterPromptCard repoUrl="https://github.com/acme/website" />);

    expect(screen.getAllByText("https://github.com/acme/website").length).toBeGreaterThan(0);
    expect(screen.getByTestId("starter-prompt").textContent).toContain(
      "read https://github.com/acme/website",
    );
  });

  it("renders the caller's footer under the docs link", () => {
    render(<StarterPromptCard footer={<p>No assistant at hand?</p>} />);

    expect(screen.getByText("No assistant at hand?")).toBeInTheDocument();
  });
});
