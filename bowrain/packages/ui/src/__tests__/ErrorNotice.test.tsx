import { describe, it, expect } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ErrorNotice } from "../errors/ErrorNotice";
import { apiErrorFromResponse } from "../errors/ApiError";
import { tokenizeJson } from "../errors/JsonHighlight";

describe("ErrorNotice", () => {
  it("renders the friendly title for an envelope error", () => {
    render(<ErrorNotice error={{ error: "project not found" }} />);
    expect(screen.getByRole("alert")).toHaveTextContent("Project not found");
  });

  it("uses the contextual title and demotes the parsed message to detail", () => {
    render(
      <ErrorNotice
        title="Couldn't create the API token"
        error={new Error('403: {"error":"cannot grant permissions beyond your own"}')}
      />,
    );
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Couldn't create the API token");
    expect(alert).toHaveTextContent("Cannot grant permissions beyond your own");
  });

  it("shows a Details disclosure only for structured payloads", () => {
    const { rerender } = render(<ErrorNotice error={{ error: "flow not found" }} />);
    expect(screen.getByTestId("error-notice-details-toggle")).toBeInTheDocument();

    rerender(<ErrorNotice error="plain text failure" />);
    expect(screen.queryByTestId("error-notice-details-toggle")).not.toBeInTheDocument();
  });

  it("expands the Details disclosure to reveal highlighted raw JSON", async () => {
    const user = userEvent.setup();
    render(<ErrorNotice error={{ error: "block not found", code: "not_found" }} />);

    expect(screen.queryByTestId("error-notice-details")).not.toBeInTheDocument();

    const toggle = screen.getByTestId("error-notice-details-toggle");
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    await user.click(toggle);

    expect(toggle).toHaveAttribute("aria-expanded", "true");
    const details = screen.getByTestId("error-notice-details");
    expect(details).toHaveTextContent('"error"');
    expect(details).toHaveTextContent('"block not found"');
    // Highlighted: keys and values are wrapped in styled spans.
    expect(details.querySelectorAll("span").length).toBeGreaterThan(2);

    await user.click(toggle);
    expect(screen.queryByTestId("error-notice-details")).not.toBeInTheDocument();
  });

  it("renders the recovery hint", () => {
    render(<ErrorNotice error={{ error: "forbidden", code: "forbidden" }} />);
    expect(screen.getByRole("alert")).toHaveTextContent(/workspace admin/i);
  });

  it("shows a refused review action as the server's sentence, with no permission remedy", () => {
    // The review banner: a separation-of-duties 403 is refused for who the
    // reviewer is, so the sentence stands alone.
    const refusal = "separation of duties: you cannot review or approve your own work";
    render(
      <ErrorNotice
        variant="inline"
        title="The review action didn't go through"
        error={apiErrorFromResponse(403, JSON.stringify({ error: refusal }))}
      />,
    );
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("The review action didn't go through");
    expect(alert).toHaveTextContent(
      "Separation of duties: you cannot review or approve your own work",
    );
    expect(alert).not.toHaveTextContent(/workspace admin/i);
  });

  it("keeps the permission remedy for a 403 written for a missing grant", () => {
    render(
      <ErrorNotice
        variant="inline"
        title="The review action didn't go through"
        error={apiErrorFromResponse(403, '{"error":"insufficient project permissions"}')}
      />,
    );
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("You don't have permission to do that");
    expect(alert).toHaveTextContent(/workspace admin/i);
  });

  it("renders the inline variant with a working disclosure", async () => {
    const user = userEvent.setup();
    render(
      <ErrorNotice
        variant="inline"
        error={{ error: "invalid request body", details: "name is required" }}
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Invalid request body");
    expect(screen.getByRole("alert")).toHaveTextContent("name is required");
    await user.click(screen.getByTestId("error-notice-details-toggle"));
    expect(screen.getByTestId("error-notice-details")).toHaveTextContent('"details"');
  });

  it("calls onRetry from the retry action", async () => {
    const user = userEvent.setup();
    let retried = 0;
    render(<ErrorNotice error="boom" onRetry={() => retried++} />);
    await user.click(screen.getByTestId("error-notice-retry"));
    expect(retried).toBe(1);
  });
});

describe("tokenizeJson", () => {
  it("classifies keys, strings, numbers, literals and punctuation", () => {
    const text = JSON.stringify({ a: "s", n: 1.5, b: true, z: null }, null, 2);
    const tokens = tokenizeJson(text);
    const types = new Set(tokens.map((t) => t.type));
    expect(types).toContain("key");
    expect(types).toContain("string");
    expect(types).toContain("number");
    expect(types).toContain("literal");
    expect(types).toContain("punct");
    // Round-trips: concatenated tokens reproduce the input exactly.
    expect(tokens.map((t) => t.text).join("")).toBe(text);
  });

  it("handles escaped quotes inside strings", () => {
    const text = JSON.stringify({ msg: 'say "hi"' }, null, 2);
    const tokens = tokenizeJson(text);
    expect(tokens.map((t) => t.text).join("")).toBe(text);
    expect(tokens.filter((t) => t.type === "string")).toHaveLength(1);
  });
});
