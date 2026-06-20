import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { ProjectPage } from "../components/ProjectPage";

describe("ProjectPage", () => {
  const project = {
    version: "v1",
    name: "Test Project",
    defaults: {
      source_language: "en-US",
      target_languages: ["fr-FR", "de-DE"],
    },
    content: [{ path: "src/locales/*.json", format: { name: "json" } }],
    flows: {
      translate: { steps: [{ tool: "translate" }] },
    },
    preset: "nextjs",
    plugins: {
      okapi: { framework_version: "^1.47.0" },
    },
  };

  it("displays project name", () => {
    render(<ProjectPage project={project} projectPath="/test.kapi" tabID="t1" />);
    expect(screen.getByText("Test Project")).toBeInTheDocument();
  });

  it("derives display name from folder when name is empty", () => {
    const noName = { ...project, name: "" };
    render(<ProjectPage project={noName} projectPath="/Users/me/MyApp/project.kapi" tabID="t1" />);
    expect(screen.getByText("MyApp")).toBeInTheDocument();
  });

  it("displays file path", () => {
    render(<ProjectPage project={project} projectPath="/test.kapi" tabID="t1" />);
    expect(screen.getByText("/test.kapi")).toBeInTheDocument();
  });

  it("shows unsaved message when no path", () => {
    render(<ProjectPage project={project} projectPath="" tabID="t1" />);
    expect(screen.getByText(/Not yet saved/)).toBeInTheDocument();
    expect(screen.getByText("Save As...")).toBeInTheDocument();
  });

  it("displays languages", () => {
    render(<ProjectPage project={project} projectPath="" tabID="t1" />);
    expect(screen.getByText("en-US")).toBeInTheDocument();
    expect(screen.getByText("fr-FR, de-DE")).toBeInTheDocument();
  });

  it("displays content patterns", () => {
    render(<ProjectPage project={project} projectPath="" tabID="t1" />);
    expect(screen.getByText("src/locales/*.json")).toBeInTheDocument();
  });

  it("displays flows", () => {
    render(<ProjectPage project={project} projectPath="" tabID="t1" />);
    expect(screen.getByText(/translate/)).toBeInTheDocument();
  });

  it("displays preset and plugins", () => {
    render(<ProjectPage project={project} projectPath="" tabID="t1" />);
    expect(screen.getByText("nextjs")).toBeInTheDocument();
    expect(screen.getByText(/okapi/)).toBeInTheDocument();
  });
});
