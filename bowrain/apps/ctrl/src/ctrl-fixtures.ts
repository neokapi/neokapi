// ---------------------------------------------------------------------------
// Shared mock data for ctrl (control plane) Storybook stories.
//
// The admin console reads its data through react-query + the /api/admin/*
// client. Stories pre-load these fixtures as props (presentational components)
// or leave the queries disabled (dialogs/pages), so no network is hit. Keep the
// shapes in sync with ../types.
// ---------------------------------------------------------------------------

import type {
  AdminUser,
  AdminWorkspace,
  BillingEvent,
  PlatformMetrics,
  UpsellOpportunity,
} from "./types";

export const sampleMetrics: PlatformMetrics = {
  mrr: 48250,
  active_workspaces: 312,
  new_signups_7d: 24,
  new_signups_30d: 118,
  credit_utilization_pct: 63.4,
  churn_rate: 2.1,
  top_workspaces: [
    {
      workspace_id: "ws_acme",
      workspace_name: "Acme Localization",
      plan: "enterprise",
      credits_used: 1_820_000,
      credits_total: 2_000_000,
    },
    {
      workspace_id: "ws_globex",
      workspace_name: "Globex Docs",
      plan: "team",
      credits_used: 640_000,
      credits_total: 1_000_000,
    },
    {
      workspace_id: "ws_initech",
      workspace_name: "Initech",
      plan: "pro",
      credits_used: 210_000,
      credits_total: 500_000,
    },
    {
      workspace_id: "ws_umbrella",
      workspace_name: "Umbrella Corp",
      plan: "team",
      credits_used: 995_000,
      credits_total: 1_000_000,
    },
    {
      workspace_id: "ws_hooli",
      workspace_name: "Hooli",
      plan: "free",
      credits_used: 42_000,
      credits_total: 50_000,
    },
  ],
};

export const sampleWorkspaces: AdminWorkspace[] = [
  {
    id: "ws_acme",
    name: "Acme Localization",
    slug: "acme",
    owner_email: "ops@acme.example",
    plan: "enterprise",
    status: "active",
    credit_usage_percent: 91,
    credits_used: 1_820_000,
    credits_total: 2_000_000,
    member_count: 42,
    created_at: "2025-01-12T09:00:00Z",
  },
  {
    id: "ws_globex",
    name: "Globex Docs",
    slug: "globex",
    owner_email: "admin@globex.example",
    plan: "team",
    status: "active",
    credit_usage_percent: 64,
    credits_used: 640_000,
    credits_total: 1_000_000,
    member_count: 12,
    created_at: "2025-03-04T14:30:00Z",
  },
  {
    id: "ws_initech",
    name: "Initech",
    slug: "initech",
    owner_email: "peter@initech.example",
    plan: "pro",
    status: "past_due",
    credit_usage_percent: 42,
    credits_used: 210_000,
    credits_total: 500_000,
    member_count: 5,
    created_at: "2025-05-21T11:15:00Z",
  },
  {
    id: "ws_hooli",
    name: "Hooli",
    slug: "hooli",
    owner_email: "gavin@hooli.example",
    plan: "free",
    status: "active",
    credit_usage_percent: 84,
    credits_used: 42_000,
    credits_total: 50_000,
    member_count: 3,
    created_at: "2025-06-30T08:45:00Z",
  },
];

export const sampleUsers: AdminUser[] = [
  {
    id: "usr_1",
    email: "ops@acme.example",
    name: "Alex Operator",
    workspace_count: 3,
    last_login: "2026-07-04T18:22:00Z",
    created_at: "2025-01-12T09:00:00Z",
  },
  {
    id: "usr_2",
    email: "admin@globex.example",
    name: "Blake Admin",
    workspace_count: 1,
    last_login: "2026-07-05T07:10:00Z",
    created_at: "2025-03-04T14:30:00Z",
  },
  {
    id: "usr_3",
    email: "peter@initech.example",
    name: "Peter Gibbons",
    workspace_count: 2,
    last_login: null,
    created_at: "2025-05-21T11:15:00Z",
  },
];

export const sampleEvents: BillingEvent[] = [
  {
    id: "evt_1",
    type: "subscription_created",
    workspace_id: "ws_globex",
    workspace_name: "Globex Docs",
    detail: "Subscribed to Team (annual)",
    created_at: "2026-07-05T09:12:00Z",
  },
  {
    id: "evt_2",
    type: "payment_failed",
    workspace_id: "ws_initech",
    workspace_name: "Initech",
    detail: "Card declined — insufficient funds",
    created_at: "2026-07-05T06:40:00Z",
  },
  {
    id: "evt_3",
    type: "subscription_upgraded",
    workspace_id: "ws_acme",
    workspace_name: "Acme Localization",
    detail: "Team → Enterprise",
    created_at: "2026-07-04T22:05:00Z",
  },
  {
    id: "evt_4",
    type: "credits_granted",
    workspace_id: "ws_hooli",
    workspace_name: "Hooli",
    detail: "200,000 tokens — support compensation",
    created_at: "2026-07-04T15:33:00Z",
  },
  {
    id: "evt_5",
    type: "subscription_canceled",
    workspace_id: "ws_umbrella",
    workspace_name: "Umbrella Corp",
    detail: "Downgraded to Free at period end",
    created_at: "2026-07-03T11:00:00Z",
  },
];

export const sampleUpsells: UpsellOpportunity[] = [
  {
    workspace_id: "ws_umbrella",
    workspace_name: "Umbrella Corp",
    current_plan: "team",
    signal: "credit_exhaustion",
    score: 92,
    detail: "Used 99.5% of monthly credits with 8 days remaining",
    suggested_plan: "enterprise",
    detected_at: "2026-07-05T05:00:00Z",
  },
  {
    workspace_id: "ws_globex",
    workspace_name: "Globex Docs",
    current_plan: "team",
    signal: "seat_pressure",
    score: 74,
    detail: "12 of 12 seats filled; 3 pending invites",
    suggested_plan: "enterprise",
    detected_at: "2026-07-04T12:00:00Z",
  },
  {
    workspace_id: "ws_initech",
    workspace_name: "Initech",
    current_plan: "pro",
    signal: "feature_gate_hits",
    score: 58,
    detail: "Hit the SSO gate 14 times this week",
    suggested_plan: "team",
    detected_at: "2026-07-03T16:20:00Z",
  },
];
