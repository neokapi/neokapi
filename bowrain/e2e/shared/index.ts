/**
 * bowrain-e2e-shared — unified API client and layered seed data for e2e tests.
 *
 * Usage:
 *   import { BowrainAPI, deviceAuth, waitForReady, fullSeed } from "bowrain-e2e-shared";
 */

// API client + static helpers
export { BowrainAPI, deviceAuth, waitForReady } from "./api-client.js";

// Types from API client
export type {
  Workspace,
  Project,
  Concept,
  AutomationRule,
  Invite,
  BrandProfile,
  Stream,
  Task,
  Notification,
  NotificationPreferences,
  Activity,
  TMEntry,
  ReadinessComponentStatus,
  ReadinessInfo,
} from "./api-client.js";

// Keycloak admin
export { KeycloakAdmin } from "./keycloak-admin.js";
export type { KeycloakAdminConfig, KeycloakUser } from "./keycloak-admin.js";

// Seed data constants
export {
  WORKSPACE,
  PROJECTS,
  TM_ENTRIES,
  CONCEPTS,
  BRAND_PROFILE,
  TASKS,
  AUTOMATION_RULES,
  STREAM,
} from "./seed-data.js";
export type { ProjectDef, TaskDef, AutomationRuleDef } from "./seed-data.js";

// Seeder functions
export {
  seedFoundation,
  seedLanguageAssets,
  seedBrandVoice,
  seedCollaboration,
  seedAutomation,
  fullSeed,
} from "./seeder.js";
export type { StoryContext } from "./seeder.js";
