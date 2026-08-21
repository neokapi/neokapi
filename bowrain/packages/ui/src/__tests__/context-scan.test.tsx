import { describe, it, expect, vi } from "vite-plus/test";
import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import type { ApiAdapter } from "../api/adapter";
import type { Workspace, ContextScanJob } from "../types/api";
import type { VoiceProfile } from "../brand/types";
import { ContextScanInput } from "../context-scan/ContextScanInput";
import { ContextScanProgress } from "../context-scan/ContextScanProgress";
import { ContextScanReview } from "../context-scan/ContextScanReview";
import { ContextScanLiveTester } from "../context-scan/ContextScanLiveTester";
import { contextScanPollInterval } from "../hooks/useContextScanApi";
import { createMockAdapter, sampleContextScanDraft } from "../stories/mock-adapter";
import type { MockAdapter } from "../stories/mock-adapter";
import { TEST_IDS } from "../test-ids";

const workspace: Workspace = {
  id: "ws-1",
  name: "Demo",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

function renderWithProviders(ui: ReactNode, adapter: ApiAdapter) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={workspace}>{ui}</WorkspaceProvider>
      </ApiProvider>
    </QueryClientProvider>,
  );
}

// ---------------------------------------------------------------------------
// ContextScanInput
// ---------------------------------------------------------------------------

describe("ContextScanInput", () => {
  it("requires at least one source before the scan can start", async () => {
    const user = userEvent.setup();
    const adapter = createMockAdapter();
    const onStarted = vi.fn();
    renderWithProviders(<ContextScanInput onStarted={onStarted} />, adapter);

    const start = screen.getByTestId(TEST_IDS.contextScan.start);
    expect(start).toBeDisabled();
    expect(screen.getByText("Add at least one source to start.")).toBeInTheDocument();

    await user.type(screen.getByTestId(TEST_IDS.contextScan.input), "We write plainly.");
    expect(start).toBeEnabled();

    await user.click(start);
    await waitFor(() => expect(onStarted).toHaveBeenCalledTimes(1));
    expect(adapter.startContextScanCalls).toEqual([{ paste_text: "We write plainly." }]);
    // No files were attached, so no upload round-trip happened.
    expect(adapter.uploadContextScanSourcesCalls).toEqual([]);
  });

  it("validates https-only links and caps the list", async () => {
    const user = userEvent.setup();
    const adapter = createMockAdapter();
    renderWithProviders(<ContextScanInput onStarted={vi.fn()} />, adapter);

    const urlInput = screen.getByTestId(TEST_IDS.contextScan.urlInput);
    await user.type(urlInput, "http://insecure.example{Enter}");
    expect(screen.getByText("Only https:// links can be fetched.")).toBeInTheDocument();

    await user.clear(urlInput);
    await user.type(urlInput, "https://acme.example/about{Enter}");
    expect(screen.getByText("https://acme.example/about")).toBeInTheDocument();
  });

  it("uploads files first and shows per-file skipped reasons", async () => {
    const user = userEvent.setup();
    const adapter = createMockAdapter();
    const onStarted = vi.fn();
    const { container } = renderWithProviders(<ContextScanInput onStarted={onStarted} />, adapter);

    const fileInput = container.querySelector<HTMLInputElement>('input[type="file"]');
    expect(fileInput).not.toBeNull();
    const guide = new File(["voice guide"], "brand-guide.docx", {
      type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    });
    const deck = new File(["deck"], "pitch-deck.pdf", { type: "application/pdf" });
    await user.upload(fileInput!, [guide, deck]);

    expect(screen.getByText("brand-guide.docx")).toBeInTheDocument();
    expect(screen.getByText("pitch-deck.pdf")).toBeInTheDocument();

    await user.click(screen.getByTestId(TEST_IDS.contextScan.start));

    // The pdf comes back skipped with the deferred-extractor reason, and the
    // start PAUSES: proceeding now would navigate to the job page and unmount
    // this panel before the reasons can be read.
    await waitFor(() => expect(screen.getByText("Some files were skipped")).toBeInTheDocument());
    expect(
      screen.getByText(/unsupported \(deferred: needs pdf\/pptx text extractor\)/),
    ).toBeInTheDocument();
    expect(onStarted).not.toHaveBeenCalled();
    expect(adapter.startContextScanCalls).toEqual([]);

    // A second, explicit click confirms and starts without re-uploading; only
    // the accepted file's blob key goes into the scan request.
    const start = screen.getByTestId(TEST_IDS.contextScan.start);
    expect(start).toHaveTextContent("Start anyway");
    await user.click(start);
    await waitFor(() => expect(onStarted).toHaveBeenCalledTimes(1));
    expect(adapter.uploadContextScanSourcesCalls).toEqual([["brand-guide.docx", "pitch-deck.pdf"]]);
    expect(adapter.startContextScanCalls).toEqual([{ upload_keys: ["blob-brand-guide.docx"] }]);
    expect(onStarted).toHaveBeenCalledWith("scan-1", {
      upload_keys: ["blob-brand-guide.docx"],
    });
  });

  it("does not start when every file is skipped and no other source exists", async () => {
    const user = userEvent.setup();
    const adapter = createMockAdapter();
    const onStarted = vi.fn();
    const { container } = renderWithProviders(<ContextScanInput onStarted={onStarted} />, adapter);

    const fileInput = container.querySelector<HTMLInputElement>('input[type="file"]');
    await user.upload(fileInput!, [new File(["x"], "deck.pdf", { type: "application/pdf" })]);
    await user.click(screen.getByTestId(TEST_IDS.contextScan.start));

    await waitFor(() =>
      expect(
        screen.getByText("None of the files could be read. Add another source to start the scan."),
      ).toBeInTheDocument(),
    );
    expect(onStarted).not.toHaveBeenCalled();
    expect(adapter.startContextScanCalls).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// ContextScanProgress
// ---------------------------------------------------------------------------

describe("ContextScanProgress", () => {
  const baseJob: ContextScanJob = {
    id: "scan-1",
    status: "processing",
    progress: 60,
    message: "drafting-voice",
    tokens_used: 0,
  };

  it("renders the progress percentage and milestone steps", () => {
    render(<ContextScanProgress job={baseJob} />);
    expect(screen.getByTestId(TEST_IDS.contextScan.progress)).toBeInTheDocument();
    expect(screen.getByText("60%")).toBeInTheDocument();
    expect(screen.getByText("Drafting your voice profile")).toBeInTheDocument();
  });

  it("renders the error and retry action on failure", async () => {
    const user = userEvent.setup();
    const onRetry = vi.fn();
    render(
      <ContextScanProgress
        job={{ ...baseJob, status: "failed", error: "corpus was empty" }}
        onRetry={onRetry}
      />,
    );
    expect(screen.getByText("corpus was empty")).toBeInTheDocument();
    await user.click(screen.getByText("Try again"));
    expect(onRetry).toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// ContextScanReview
// ---------------------------------------------------------------------------

describe("ContextScanReview", () => {
  function reviewAdapter(): {
    adapter: MockAdapter;
    createBrandProfile: ReturnType<typeof vi.fn>;
    createConcept: ReturnType<typeof vi.fn>;
  } {
    const adapter = createMockAdapter();
    const created: VoiceProfile = {
      ...sampleContextScanDraft.profile,
      id: "prof-created",
      name: "Edited Voice",
    };
    const createBrandProfile = vi.fn().mockResolvedValue(created);
    const createConcept = vi.fn().mockImplementation(async (_ws: string, req: unknown) => ({
      id: "c-new",
      ...(req as object),
      created_at: "",
      updated_at: "",
    }));
    adapter.createBrandProfile = createBrandProfile;
    (adapter as unknown as Record<string, unknown>).createConcept = createConcept;
    return { adapter, createBrandProfile, createConcept };
  }

  it("renders per-field confidence badges with source attribution", () => {
    const { adapter } = reviewAdapter();
    renderWithProviders(
      <ContextScanReview draft={sampleContextScanDraft} onApproved={vi.fn()} />,
      adapter,
    );

    const badges = screen.getAllByTestId(TEST_IDS.contextScan.fieldConfidence);
    // tone, style, vocabulary, examples
    expect(badges).toHaveLength(4);
    expect(screen.getByText("82% confidence")).toBeInTheDocument();
    expect(screen.getByText("Consistent register across brand-guide.docx")).toBeInTheDocument();
    // Provenance lists every corpus source.
    expect(screen.getByText("https://acme.example/about")).toBeInTheDocument();
  });

  it("approves with the edited draft and only the selected terms", async () => {
    const user = userEvent.setup();
    const { adapter, createBrandProfile, createConcept } = reviewAdapter();
    const onApproved = vi.fn();
    renderWithProviders(
      <ContextScanReview draft={sampleContextScanDraft} onApproved={onApproved} />,
      adapter,
    );

    // Refine: rename the profile before approving.
    const nameInput = screen.getByLabelText("Name");
    await user.clear(nameInput);
    await user.type(nameInput, "Edited Voice");

    // Deselect the second candidate term ("stream").
    const rows = screen.getAllByTestId(TEST_IDS.contextScan.termRow);
    expect(rows).toHaveLength(3);
    await user.click(screen.getByLabelText("Include stream"));

    await user.click(screen.getByTestId(TEST_IDS.contextScan.approve));

    await waitFor(() => expect(onApproved).toHaveBeenCalledTimes(1));
    expect(createBrandProfile).toHaveBeenCalledTimes(1);
    const [ws, request] = createBrandProfile.mock.calls[0];
    expect(ws).toBe("demo");
    expect(request.name).toBe("Edited Voice");
    expect(request.vocabulary).toEqual(sampleContextScanDraft.profile.vocabulary);

    // Only the still-selected terms become concepts.
    expect(createConcept).toHaveBeenCalledTimes(2);
    const conceptTerms = createConcept.mock.calls.map(
      (c) => (c[1] as { terms: { text: string }[] }).terms[0].text,
    );
    expect(conceptTerms).toEqual(["workspace", "convergence"]);

    // Navigation target receives the created profile.
    expect(onApproved).toHaveBeenCalledWith(expect.objectContaining({ id: "prof-created" }));
  });

  it("disables approve when the name is cleared", async () => {
    const user = userEvent.setup();
    const { adapter } = reviewAdapter();
    renderWithProviders(
      <ContextScanReview draft={sampleContextScanDraft} onApproved={vi.fn()} />,
      adapter,
    );
    await user.clear(screen.getByLabelText("Name"));
    expect(screen.getByTestId(TEST_IDS.contextScan.approve)).toBeDisabled();
  });
});

// ---------------------------------------------------------------------------
// ContextScanLiveTester
// ---------------------------------------------------------------------------

describe("ContextScanLiveTester", () => {
  it("debounces checkBrandDraft and renders score + findings", async () => {
    const user = userEvent.setup();
    const adapter = createMockAdapter();
    renderWithProviders(
      <ContextScanLiveTester profile={sampleContextScanDraft.profile} debounceMs={50} />,
      adapter,
    );

    const tester = screen.getByTestId(TEST_IDS.contextScan.liveTester);
    const textarea = tester.querySelector("textarea");
    expect(textarea).not.toBeNull();

    // Many keystrokes in quick succession must collapse into one check.
    await user.type(textarea!, "our synergy story");
    await waitFor(() => expect(adapter.checkBrandDraftCalls).toHaveLength(1));
    // Give the debounce window time to prove no further calls fire.
    await new Promise((r) => setTimeout(r, 120));
    expect(adapter.checkBrandDraftCalls).toHaveLength(1);
    expect(adapter.checkBrandDraftCalls[0].text).toBe("our synergy story");

    // The deterministic mock flags the forbidden term.
    expect(await screen.findByText("62")).toBeInTheDocument();
    expect(screen.getByText('Uses the term "synergy"')).toBeInTheDocument();
    // The mock delivers the SERVER finding shape (core/check.Finding's
    // `category`); the tester must normalize it to the UI's `dimension` —
    // regression lock for the review-page crash on the first live check.
    expect(screen.getByText("vocabulary")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Poll cadence
// ---------------------------------------------------------------------------

describe("contextScanPollInterval", () => {
  it("polls while queued or processing and stops on terminal states", () => {
    expect(contextScanPollInterval(undefined)).toBe(1500);
    expect(contextScanPollInterval("queued")).toBe(1500);
    expect(contextScanPollInterval("processing")).toBe(1500);
    expect(contextScanPollInterval("completed")).toBe(false);
    expect(contextScanPollInterval("failed")).toBe(false);
  });
});
