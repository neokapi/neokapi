import type { Profile } from "@neokapi/docs-shared/capability";

export { operationChanges } from "@neokapi/docs-shared/capability";
export type { Profile };
export type History = {
  date: string;
  revision?: string;
  formats: Partial<Record<string, string>>;
  outputs?: Profile;
};
