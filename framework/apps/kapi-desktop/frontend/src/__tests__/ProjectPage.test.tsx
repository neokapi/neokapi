import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { ProjectPage } from "../components/ProjectPage";

describe("ProjectPage", () => {
  const project = {
    version: "v1",
    name: "Test Project",
    source_language: "en-US",
    target_languages: ["fr-FR", "de-DE"],
    content: [
      { path: "src/locales/*.json", format: "json" },
    ],
    flows: {
      translate: { steps: [{ tool: "ai-translate" }] },
    },
    preset: "nextjs",
    plugins: ["okapi@1.47.0"],
  };

  it("displays project name", () => {
    render(
      <ProjectPage project={project} projectPath="/test.kapi"  />,
    );
    expect(screen.getByText("Test Project")).toBeInTheDocument();
  });

  it("displays file path", () => {
    render(
      <ProjectPage project={project} projectPath="/test.kapi"  />,
    );
    expect(screen.getByText("/test.kapi")).toBeInTheDocument();
  });

  it("shows unsaved message when no path", () => {
    render(
      <ProjectPage project={project} projectPath=""  />,
    );
    expect(screen.getByText(/Unsaved project/)).toBeInTheDocument();
  });

  it("displays languages", () => {
    render(
      <ProjectPage project={project} projectPath=""  />,
    );
    expect(screen.getByText("en-US")).toBeInTheDocument();
    expect(screen.getByText("fr-FR, de-DE")).toBeInTheDocument();
  });

  it("displays content patterns", () => {
    render(
      <ProjectPage project={project} projectPath=""  />,
    );
    expect(screen.getByText("src/locales/*.json")).toBeInTheDocument();
  });

  it("displays flows", () => {
    render(
      <ProjectPage project={project} projectPath=""  />,
    );
    expect(screen.getByText(/translate/)).toBeInTheDocument();
  });

  it("displays preset and plugins", () => {
    render(
      <ProjectPage project={project} projectPath=""  />,
    );
    expect(screen.getByText("nextjs")).toBeInTheDocument();
    expect(screen.getByText("okapi@1.47.0")).toBeInTheDocument();
  });
});
