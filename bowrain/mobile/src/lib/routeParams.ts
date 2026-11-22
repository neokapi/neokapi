// Expo Router names a layout group in parentheses, such as "(tabs)", and a
// navigation that reaches a dynamic route by a group's name passes that name
// as the route parameter. A group name is never a workspace slug or a project
// id, so a screen reads it as no value and builds no request from it.

/** The route parameter as a value, or undefined when it is missing or a group name. */
export function routeValue(value: string | string[] | undefined): string | undefined {
  if (typeof value !== "string" || value === "") return undefined;
  return /^\(.*\)$/.test(value) ? undefined : value;
}
