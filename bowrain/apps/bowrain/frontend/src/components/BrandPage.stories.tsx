import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ApiAdapter, VoiceProfile, CandidateRule, ProjectInfo } from "@neokapi/ui";
import { BrandPage } from "./BrandPage";
import { withWorkspaceApi, mockApiAdapter, mockPersonalWorkspace } from "../storybook/backend";

/**
 * BrandPage is a `useApi()`/`useWorkspace()` container — the desktop's
 * brand-governance review surface (the correction-learning loop, AD-019). Its
 * react-query `queryFn`s are `ApiAdapter` methods, so the stories back it with
 * a mock adapter through `withWorkspaceApi`; no Wails runtime is involved.
 */

const profiles = [{ id: "prof-1", name: "House Voice" } as unknown as VoiceProfile];

const candidates: CandidateRule[] = [
  {
    term: "utilize",
    replacement: "use",
    correction_count: 6,
    dimension: "vocabulary",
    status: "pending",
  },
  {
    term: "leverage",
    replacement: "use",
    correction_count: 4,
    dimension: "vocabulary",
    status: "pending",
  },
  {
    term: "synergy",
    replacement: "teamwork",
    correction_count: 3,
    dimension: "vocabulary",
    status: "pending",
  },
];

const projects = [
  { id: "proj-1", name: "Marketing Site" } as unknown as ProjectInfo,
  { id: "proj-2", name: "Mobile App" } as unknown as ProjectInfo,
];

function brandAdapter(): ApiAdapter {
  return mockApiAdapter({
    listBrandProfiles: () => Promise.resolve(profiles),
    listBrandCandidates: () => Promise.resolve(candidates),
    getBrandDrift: () =>
      Promise.resolve({
        drifted: true,
        recent_avg: 82,
        baseline_avg: 91,
        drop: 9,
        recent_days: 30,
        recent_count: 24,
      }),
    promoteBrandRule: () => Promise.resolve({ promoted: true }),
    rejectBrandRule: () => Promise.resolve(undefined),
    evaluateBrandRule: () =>
      Promise.resolve({
        total_blocks: 120,
        affected_blocks: 14,
        improved_blocks: 11,
        degraded_blocks: 0,
        new_violations: 0,
        resolved_violations: 14,
        critical_count: 0,
        collections: [],
      }),
  } as unknown as Partial<ApiAdapter>);
}

const meta: Meta<typeof BrandPage> = {
  title: "Bowrain/BrandPage",
  component: BrandPage,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  args: { projects },
};

export default meta;
type Story = StoryObj<typeof BrandPage>;

/** Candidate rules loaded for a connected team workspace. */
export const WithCandidates: Story = {
  decorators: [withWorkspaceApi({ adapter: brandAdapter() })],
};

/** The disconnected personal workspace shows the connect prompt. */
export const PersonalWorkspace: Story = {
  decorators: [withWorkspaceApi({ adapter: brandAdapter(), workspace: mockPersonalWorkspace })],
};
