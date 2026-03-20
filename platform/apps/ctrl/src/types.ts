// ---------------------------------------------------------------------------
// Admin-specific types for the control plane
// ---------------------------------------------------------------------------

export interface AdminWorkspace {
  id: string;
  name: string;
  slug: string;
  owner_email: string;
  plan: string;
  status: string;
  credit_usage_percent: number;
  credits_used: number;
  credits_total: number;
  member_count: number;
  created_at: string;
}

export interface AdminWorkspaceDetail extends AdminWorkspace {
  stripe_customer_id: string | null;
  stripe_subscription_id: string | null;
  current_period_start: string | null;
  current_period_end: string | null;
  cancel_at: string | null;
  seat_count: number;
  members: WorkspaceMember[];
  recent_activity: ActivityEntry[];
}

export interface WorkspaceMember {
  user_id: string;
  email: string;
  name: string;
  role: string;
  joined_at: string;
}

export interface ActivityEntry {
  id: string;
  type: string;
  description: string;
  created_at: string;
}

export interface FeatureOverride {
  id: string;
  workspace_id: string;
  workspace_name?: string;
  feature: string;
  enabled: boolean;
  reason: string | null;
  created_by: string;
  created_at: string;
  expires_at: string | null;
}

export interface WorkspaceNote {
  id: string;
  workspace_id: string;
  author_email: string;
  content: string;
  created_at: string;
}

export interface AdminUser {
  id: string;
  email: string;
  name: string;
  workspace_count: number;
  last_login: string | null;
  created_at: string;
}

export interface AdminUserDetail extends AdminUser {
  workspaces: UserWorkspaceMembership[];
}

export interface UserWorkspaceMembership {
  workspace_id: string;
  workspace_name: string;
  workspace_slug: string;
  role: string;
  plan: string;
  joined_at: string;
}

export interface PlatformMetrics {
  mrr: number;
  active_workspaces: number;
  new_signups_7d: number;
  new_signups_30d: number;
  credit_utilization_percent: number;
  churn_rate_percent: number;
  top_workspaces: TopWorkspaceUsage[];
}

export interface TopWorkspaceUsage {
  workspace_id: string;
  workspace_name: string;
  plan: string;
  credits_used: number;
  credits_total: number;
}

export interface BillingEvent {
  id: string;
  type: BillingEventType;
  workspace_id: string;
  workspace_name: string;
  detail: string;
  created_at: string;
}

export type BillingEventType =
  | "subscription_created"
  | "subscription_upgraded"
  | "subscription_downgraded"
  | "subscription_canceled"
  | "payment_succeeded"
  | "payment_failed"
  | "credits_purchased"
  | "credits_granted";

export interface UpsellOpportunity {
  workspace_id: string;
  workspace_name: string;
  current_plan: string;
  signal: UpsellSignal;
  score: number;
  detail: string;
  suggested_plan: string;
  detected_at: string;
}

export type UpsellSignal =
  | "credit_exhaustion"
  | "seat_pressure"
  | "feature_gate_hits"
  | "high_usage"
  | "trial_expiring"
  | "dormant_paid";

export interface LedgerEntry {
  id: string;
  workspace_id: string;
  allocation_id: string | null;
  amount: number;
  balance_after: number;
  operation: string;
  reference_id: string | null;
  created_at: string;
}
