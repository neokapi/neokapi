// The review plan's query key, shared by the plan screen and the project home
// so both read one cached answer and refresh together.
export function reviewPlanKey(ws: string, project: string) {
  return ["review-plan", ws, project] as const;
}

// The coordinates map's query key. Confirming refreshes it and the plan.
export function reviewCoordinatesKey(ws: string, project: string) {
  return ["review-coordinates", ws, project] as const;
}
